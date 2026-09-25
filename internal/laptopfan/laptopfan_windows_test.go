//go:build windows

package laptopfan

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// withStubSession 把会话建立替换为不触碰 WMI 的假实现，并返回建立次数计数器。
func withStubSession(t *testing.T) *int {
	t.Helper()

	opened := 0
	original := openSession
	openSession = func() (*wmiSession, error) {
		opened++
		// service 保持 nil：close() 对 nil service 是安全的，而测试里的任务
		// 不会真的去调用它。
		return &wmiSession{callers: make(map[string]*wmiMethodCaller)}, nil
	}
	t.Cleanup(func() { openSession = original })
	return &opened
}

// 会话在整个进程生命周期内复用，是这次优化的全部意义所在：
// 每次采样重新 ConnectServer 正是要消除的开销。
func TestCOMWorkerReusesSessionAcrossCalls(t *testing.T) {
	opened := withStubSession(t)

	worker := newCOMWorker()
	defer worker.close()

	for i := range 5 {
		if err := worker.do(func(*wmiSession) error { return nil }); err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i+1, err)
		}
	}

	if *opened != 1 {
		t.Fatalf("会话应只建立一次，实际建立 %d 次", *opened)
	}
}

// 类/方法不存在是机型探测的正常结果，不能因此丢掉一条可用连接——
// 否则探测阶段每撞一个不存在的类就要重连一次。
func TestCOMWorkerKeepsSessionWhenClassUnavailable(t *testing.T) {
	opened := withStubSession(t)

	worker := newCOMWorker()
	defer worker.close()

	probeErr := fmt.Errorf("查询 LENOVO_FAN_METHOD 失败: %w", errWMIClassUnavailable)
	for i := range 3 {
		err := worker.do(func(*wmiSession) error { return probeErr })
		if !errors.Is(err, errWMIClassUnavailable) {
			t.Fatalf("第 %d 次调用应原样返回探测错误，实际: %v", i+1, err)
		}
	}
	if err := worker.do(func(*wmiSession) error { return nil }); err != nil {
		t.Fatalf("探测失败后仍应能正常调用: %v", err)
	}

	if *opened != 1 {
		t.Fatalf("探测失败不应重建会话，实际建立 %d 次", *opened)
	}
}

// 真正的调用失败（连接失效、休眠唤醒后句柄作废）必须丢弃会话并重建，
// 这是旧实现"每次都重连"自带的自愈能力，缓存之后要显式保留。
func TestCOMWorkerRebuildsSessionAfterCallFailure(t *testing.T) {
	opened := withStubSession(t)

	worker := newCOMWorker()
	defer worker.close()

	if err := worker.do(func(*wmiSession) error { return nil }); err != nil {
		t.Fatalf("首次调用失败: %v", err)
	}
	if err := worker.do(func(*wmiSession) error { return errors.New("ExecMethod 调用失败") }); err == nil {
		t.Fatal("应返回调用错误")
	}
	if err := worker.do(func(*wmiSession) error { return nil }); err != nil {
		t.Fatalf("失败后应重建会话并恢复: %v", err)
	}

	if *opened != 2 {
		t.Fatalf("调用失败后应重建一次会话，实际建立 %d 次", *opened)
	}
}

// close() 之后不得再向已关闭的通道投递任务（那会 panic），
// 而要返回一个普通错误。
func TestCOMWorkerRejectsCallsAfterClose(t *testing.T) {
	withStubSession(t)

	worker := newCOMWorker()
	if err := worker.do(func(*wmiSession) error { return nil }); err != nil {
		t.Fatalf("关闭前调用失败: %v", err)
	}

	worker.close()
	worker.close() // 幂等

	if err := worker.do(func(*wmiSession) error { return nil }); err == nil {
		t.Fatal("关闭后的调用应返回错误")
	}
}

// 标记为不支持时必须释放常驻的 COM 线程与 WMI 连接：不支持的机型是多数情况，
// 白白留着一个线程和一条连接正是这次优化要避免的。
func TestReaderReleasesWorkerWhenUnsupported(t *testing.T) {
	withStubSession(t)

	reader := &windowsReader{backendIdx: -1}
	reader.worker = newCOMWorker()

	reader.mutex.Lock()
	reader.disableLocked("test: unsupported")
	reader.mutex.Unlock()

	if !reader.unsupported {
		t.Fatal("应标记为不支持")
	}
	if reader.worker != nil {
		t.Fatal("应释放 COM 工作线程")
	}
	if _, ok := reader.read(); ok {
		t.Fatal("标记不支持后不应再返回读数")
	}
	if reader.worker != nil {
		t.Fatal("标记不支持后不应重新创建 COM 工作线程")
	}
}

// 一次卡死的 WMI 调用不能把调用方（温控循环）无限期挂住。
func TestCOMWorkerCallTimesOut(t *testing.T) {
	withStubSession(t)
	previous := comCallTimeout
	comCallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { comCallTimeout = previous })

	worker := newCOMWorker()
	release := make(chan struct{})
	finished := make(chan struct{})
	defer func() {
		// 等卡住的调用真正返回再结束测试，清理函数才能安全地恢复 openSession。
		close(release)
		<-finished
		worker.close()
	}()

	start := time.Now()
	err := worker.do(func(*wmiSession) error {
		<-release
		close(finished)
		return nil
	})
	if !errors.Is(err, errCOMTimeout) {
		t.Fatalf("卡死的调用应返回 errCOMTimeout，实际: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("超时返回耗时 %v", elapsed)
	}
}

func TestLockedBackendRetryDelayGrowsAndCaps(t *testing.T) {
	if got := lockedBackendRetryDelay(maxConsecutiveFailures); got != lockedBackendRetryBase {
		t.Fatalf("首次退避 = %v，期望 %v", got, lockedBackendRetryBase)
	}
	if got := lockedBackendRetryDelay(maxConsecutiveFailures + 1); got != 2*lockedBackendRetryBase {
		t.Fatalf("第二次退避 = %v，期望 %v", got, 2*lockedBackendRetryBase)
	}
	if got := lockedBackendRetryDelay(100); got != lockedBackendRetryMax {
		t.Fatalf("退避上限 = %v，期望 %v", got, lockedBackendRetryMax)
	}
}
