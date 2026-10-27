//go:build windows

package hotkey

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey    = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey  = user32.NewProc("UnregisterHotKey")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procPeekMessageW      = user32.NewProc("PeekMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")
)

const (
	wmQuit   = 0x0012
	wmHotkey = 0x0312
	// wmRunRequests 唤醒消息循环去执行排队的注册/注销请求。
	wmRunRequests = 0x8000 + 1 // WM_APP + 1
	pmNoRemove    = 0x0000
	modNoRepeat   = 0x4000
)

type winMsg struct {
	HWnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       struct{ X, Y int32 }
	LPrivate uint32
}

var errMessageLoopStopped = errors.New("hotkey message loop stopped")

// messageLoop 是所有快捷键共用的一条系统线程。RegisterHotKey(hwnd=0) 把 WM_HOTKEY 投到
// 调用线程的消息队列，所以注册、注销与 GetMessage 必须在同一条锁定线程上；没有按键时
// 线程阻塞在 GetMessage 里，完全不被唤醒。
type messageLoop struct {
	threadID uint32
	onHotkey func(id int32)

	mu       sync.Mutex
	requests []func()
	stopped  bool
	done     chan struct{}
}

func startMessageLoop(onHotkey func(id int32)) *messageLoop {
	l := &messageLoop{onHotkey: onHotkey, done: make(chan struct{})}
	started := make(chan struct{})
	go l.run(started)
	<-started
	return l
}

func (l *messageLoop) run(started chan<- struct{}) {
	runtime.LockOSThread()
	defer close(l.done)

	// 线程第一次调用 PeekMessage 时系统才为它建消息队列，之后 PostThreadMessage 才投得进来。
	var msg winMsg
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmNoRemove)
	l.threadID = windows.GetCurrentThreadId()
	close(started)

	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		// 0 是 WM_QUIT，-1 是出错；两种情况都结束循环。
		if int32(r) <= 0 {
			break
		}
		switch msg.Message {
		case wmHotkey:
			l.onHotkey(int32(msg.WParam))
		case wmRunRequests:
			l.runRequests()
		}
	}

	l.mu.Lock()
	l.stopped = true
	pending := l.requests
	l.requests = nil
	l.mu.Unlock()
	// 退出前收到的请求照样执行，等待它们的调用方才不会永远阻塞。
	for _, f := range pending {
		f()
	}
}

func (l *messageLoop) runRequests() {
	l.mu.Lock()
	pending := l.requests
	l.requests = nil
	l.mu.Unlock()
	for _, f := range pending {
		f()
	}
}

// call 在消息循环线程上同步执行 f 并返回其结果。
func (l *messageLoop) call(f func() error) error {
	errc := make(chan error, 1)
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return errMessageLoopStopped
	}
	l.requests = append(l.requests, func() { errc <- f() })
	l.mu.Unlock()
	if r, _, err := procPostThreadMessage.Call(uintptr(l.threadID), wmRunRequests, 0, 0); r == 0 {
		return err
	}
	return <-errc
}

func (l *messageLoop) stop() {
	procPostThreadMessage.Call(uintptr(l.threadID), wmQuit, 0, 0)
	<-l.done
}

// registerHotKey 只能在消息循环线程上调用。
func registerHotKey(id int32, mods Modifier, key Key) error {
	r, _, err := procRegisterHotKey.Call(0, uintptr(id), uintptr(mods)|modNoRepeat, uintptr(key))
	if r == 0 {
		return err
	}
	return nil
}

// unregisterHotKey 只能在消息循环线程上调用。
func unregisterHotKey(id int32) error {
	r, _, err := procUnregisterHotKey.Call(0, uintptr(id))
	if r == 0 {
		return err
	}
	return nil
}
