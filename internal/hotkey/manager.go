//go:build windows

package hotkey

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/TIANLI0/THRM/internal/logger"
	"github.com/TIANLI0/THRM/internal/types"
)

// Action 定义快捷键触发的动作。
type Action string

const (
	ActionToggleManualGear   Action = "toggle-manual-gear"
	ActionToggleAutoMode     Action = "toggle-auto-control"
	ActionToggleCurveProfile Action = "toggle-curve-profile"
)

// Modifier 是 RegisterHotKey 的修饰键位（MOD_*）。
type Modifier uint32

// Key 是 Windows 虚拟键码（VK_*）。
type Key uint32

const (
	ModAlt   Modifier = 0x0001
	ModCtrl  Modifier = 0x0002
	ModShift Modifier = 0x0004
	ModWin   Modifier = 0x0008
)

// Manager 负责注册/更新/注销全局快捷键。
//
// 所有快捷键共用一条消息循环线程（见 msgloop_windows.go），没有按键时线程阻塞在
// GetMessage 里。此前用的 golang.design/x/hotkey 给每个快捷键开一条锁定线程，并以
// 10ms 间隔轮询 PeekMessage/GetAsyncKeyState：默认 3 个快捷键就是每秒 300 次唤醒，
// 占了核心服务约九成的空闲 CPU，还让笔记本 CPU 进不了深度休眠。
type Manager struct {
	logger   types.Logger
	onAction func(action Action, shortcut string)

	mutex    sync.Mutex
	bindings map[Action]*binding
	closed   bool
	nextID   int32

	// loop 在第一次注册快捷键时才启动，没配快捷键就不占线程。
	loop *messageLoop
	// fired 把 WM_HOTKEY 从消息循环线程交给 dispatch 协程：动作回调可能反过来调用
	// UpdateBindings，若直接在消息循环线程上执行，等待自己处理注册请求会死锁。
	fired chan int32
}

type binding struct {
	shortcut string
	id       int32
}

// NewManager 创建快捷键管理器。
func NewManager(logger types.Logger, onAction func(action Action, shortcut string)) *Manager {
	return &Manager{
		logger:   logger,
		onAction: onAction,
		bindings: make(map[Action]*binding),
	}
}

// UpdateBindings 根据配置更新快捷键绑定。
func (m *Manager) UpdateBindings(manualGearShortcut, autoControlShortcut, curveProfileShortcut string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return fmt.Errorf("hotkey manager already stopped")
	}

	var errs []string
	if err := m.setBinding(ActionToggleManualGear, manualGearShortcut); err != nil {
		errs = append(errs, err.Error())
	}
	if err := m.setBinding(ActionToggleAutoMode, autoControlShortcut); err != nil {
		errs = append(errs, err.Error())
	}
	if err := m.setBinding(ActionToggleCurveProfile, curveProfileShortcut); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// Stop 释放所有快捷键资源。
func (m *Manager) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return
	}
	m.closed = true

	for action := range m.bindings {
		m.unregisterBinding(action)
	}
	if m.loop != nil {
		m.loop.stop()
		close(m.fired)
		m.loop = nil
	}
}

func (m *Manager) setBinding(action Action, shortcut string) error {
	shortcut = normalizeShortcut(shortcut)

	if existing, ok := m.bindings[action]; ok {
		if existing.shortcut == shortcut {
			return nil
		}
		m.unregisterBinding(action)
	}

	if shortcut == "" {
		return nil
	}

	mods, key, err := ParseShortcut(shortcut)
	if err != nil {
		return fmt.Errorf("%s 快捷键无效: %w", action, err)
	}
	var modMask Modifier
	for _, mod := range mods {
		modMask |= mod
	}

	if m.loop == nil {
		m.fired = make(chan int32, 8)
		m.loop = startMessageLoop(m.onHotkey)
		go m.dispatch(m.fired)
	}

	m.nextID++
	id := m.nextID
	if err := m.loop.call(func() error { return registerHotKey(id, modMask, key) }); err != nil {
		return fmt.Errorf("%s 注册失败(%s): %w", action, shortcut, err)
	}

	m.bindings[action] = &binding{shortcut: shortcut, id: id}
	m.logInfo("快捷键已注册: %s -> %s", action, shortcut)
	return nil
}

func (m *Manager) unregisterBinding(action Action) {
	b, ok := m.bindings[action]
	if !ok || b == nil {
		return
	}
	if err := m.loop.call(func() error { return unregisterHotKey(b.id) }); err != nil {
		m.logDebug("快捷键注销失败: %s (%v)", action, err)
	}
	delete(m.bindings, action)
}

// onHotkey 运行在消息循环线程上，只做转交。通道满了说明动作还在处理，丢掉这次按键即可。
func (m *Manager) onHotkey(id int32) {
	select {
	case m.fired <- id:
	default:
	}
}

func (m *Manager) dispatch(fired <-chan int32) {
	for id := range fired {
		action, shortcut, ok := m.lookup(id)
		if ok && m.onAction != nil {
			m.onAction(action, shortcut)
		}
	}
}

// lookup 找出 id 当前对应的动作。按键与注销可能交错：已注销的 id 找不到，直接忽略。
func (m *Manager) lookup(id int32) (Action, string, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for action, b := range m.bindings {
		if b.id == id {
			return action, b.shortcut, true
		}
	}
	return "", "", false
}

// ParseShortcut 解析快捷键字符串，支持 Ctrl/Alt/Shift/Win + A-Z/0-9/F1-F12。
func ParseShortcut(input string) ([]Modifier, Key, error) {
	normalized := normalizeShortcut(input)
	if normalized == "" {
		return nil, 0, fmt.Errorf("empty shortcut")
	}

	parts := strings.Split(normalized, "+")
	mods := make([]Modifier, 0, 3)
	modSet := map[Modifier]bool{}
	var key Key
	keySet := false

	for _, part := range parts {
		if part == "" {
			continue
		}

		if mod, ok := parseModifier(part); ok {
			if !modSet[mod] {
				mods = append(mods, mod)
				modSet[mod] = true
			}
			continue
		}

		if keySet {
			return nil, 0, fmt.Errorf("multiple main keys")
		}

		parsedKey, err := parseKey(part)
		if err != nil {
			return nil, 0, err
		}
		key = parsedKey
		keySet = true
	}

	if !keySet {
		return nil, 0, fmt.Errorf("missing main key")
	}
	if len(mods) == 0 {
		return nil, 0, fmt.Errorf("missing modifier")
	}

	return mods, key, nil
}

func parseModifier(part string) (Modifier, bool) {
	switch strings.ToUpper(strings.TrimSpace(part)) {
	case "CTRL", "CONTROL":
		return ModCtrl, true
	case "ALT":
		return ModAlt, true
	case "SHIFT":
		return ModShift, true
	case "WIN", "WINDOWS", "SUPER":
		return ModWin, true
	default:
		return 0, false
	}
}

// parseKey 把主键换成虚拟键码：A-Z、0-9 的虚拟键码就是对应的 ASCII 码，F1-F12 是 0x70-0x7B。
func parseKey(part string) (Key, error) {
	token := strings.ToUpper(strings.TrimSpace(part))

	if len(token) == 1 {
		if c := token[0]; (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return Key(c), nil
		}
	}

	if after, ok := strings.CutPrefix(token, "F"); ok {
		if n, err := strconv.Atoi(after); err == nil && n >= 1 && n <= 12 {
			return Key(0x70 + n - 1), nil
		}
	}

	return 0, fmt.Errorf("unsupported key: %s", part)
}

func normalizeShortcut(input string) string {
	parts := strings.Split(strings.TrimSpace(input), "+")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return strings.Join(out, "+")
}

func (m *Manager) logInfo(format string, v ...any) {
	if m.logger != nil {
		logger.Wrapped(m.logger).Info(format, v...)
	}
}

func (m *Manager) logDebug(format string, v ...any) {
	if m.logger != nil {
		logger.Wrapped(m.logger).Debug(format, v...)
	}
}
