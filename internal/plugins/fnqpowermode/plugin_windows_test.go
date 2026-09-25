//go:build windows

package fnqpowermode

import (
	"context"
	"testing"
)

// 快速 Stop→Start 时，旧 run 协程的退出清理晚于新实例启动。它曾无条件清掉
// running，新实例随即被当成已停止，Stop 直接返回，新的 PowerShell 监听进程就泄漏了。
func TestStaleRunExitDoesNotClobberNewInstance(t *testing.T) {
	p := New(Options{})

	// 第一代实例：ctx 已被 Stop 取消，run 会在 cmd.Start 处立即失败，不会真的拉起进程。
	staleCtx, staleCancel := context.WithCancel(context.Background())
	p.mutex.Lock()
	p.generation = 1
	p.cancel = staleCancel
	p.running = true
	p.mutex.Unlock()
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// 紧接着的 Start 已让第二代实例接管（这里只摆出 Start 设置的状态，不启动 PowerShell）。
	newCtx, newCancel := context.WithCancel(context.Background())
	defer newCancel()
	p.mutex.Lock()
	p.generation = 2
	p.cancel = newCancel
	p.running = true
	p.mutex.Unlock()

	// 旧 run 这时才退出。
	p.run(staleCtx, 1)

	status := p.Status()
	if !status.Running {
		t.Fatal("旧实例退出把新实例标成了未运行")
	}
	if status.LastError != "" {
		t.Fatalf("旧实例被取消的启动失败写进了新实例的错误: %q", status.LastError)
	}
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if newCtx.Err() == nil {
		t.Fatal("Stop 没有取消新实例，监听进程会泄漏")
	}
}
