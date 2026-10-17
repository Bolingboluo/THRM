package device

import (
	"testing"
	"time"

	"github.com/TIANLI0/THRM/internal/types"
)

// TestIdlePollingInterval 覆盖 #10：空闲时放慢 HID 空转轮询。
//
// HID 句柄设为非阻塞后，读循环靠"空转即休眠"避免占满 CPU，这意味着常驻
// 10 次/秒的唤醒。空闲（无 GUI 且未开智能控温）时没人消费实时转速，
// 托盘 5 秒才刷新一次，因此可以降到 2 次/秒。
func TestIdlePollingInterval(t *testing.T) {
	m := NewManager(testLogger{})
	// 空闲轮询只在本次连接已读到过状态帧后生效，见 TestIdlePollingWaitsForFirstStatusFrame。
	m.currentFanData.Store(&types.FanData{})

	if got := m.readPollInterval(); got != hidReadPollInterval {
		t.Fatalf("默认轮询间隔 = %s, want %s", got, hidReadPollInterval)
	}

	m.SetIdlePolling(true)
	if got := m.readPollInterval(); got != hidIdleReadPollInterval {
		t.Fatalf("空闲轮询间隔 = %s, want %s", got, hidIdleReadPollInterval)
	}

	m.SetIdlePolling(false)
	if got := m.readPollInterval(); got != hidReadPollInterval {
		t.Fatalf("恢复后轮询间隔 = %s, want %s", got, hidReadPollInterval)
	}

	wakeupsPerSecond := float64(time.Second) / float64(hidIdleReadPollInterval)
	t.Logf("空闲唤醒频率 %.0f 次/秒（修复前恒为 %.0f 次/秒）",
		wakeupsPerSecond, float64(time.Second)/float64(hidReadPollInterval))
}

// TestWaitPollIntervalRespondsToStop 验证拉长间隔后仍能立即响应停止信号。
// 用 select 而非 time.Sleep 是必要的：否则空闲模式下断连/重连要多等半秒。
func TestWaitPollIntervalRespondsToStop(t *testing.T) {
	m := NewManager(testLogger{})
	m.SetIdlePolling(true)

	stop := make(chan struct{})
	close(stop)

	start := time.Now()
	if m.waitPollInterval(stop) {
		t.Fatal("收到停止信号时 waitPollInterval 应返回 false")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("停止信号响应耗时 %v，未能立即返回", elapsed)
	}
}

// TestWaitPollIntervalElapses 验证无停止信号时正常等满间隔。
func TestWaitPollIntervalElapses(t *testing.T) {
	m := NewManager(testLogger{})
	stop := make(chan struct{})

	start := time.Now()
	if !m.waitPollInterval(stop) {
		t.Fatal("无停止信号时 waitPollInterval 应返回 true")
	}
	if elapsed := time.Since(start); elapsed < hidReadPollInterval {
		t.Fatalf("等待时间 %v 短于轮询间隔 %s", elapsed, hidReadPollInterval)
	}
}

// TestIdlePollingWaitsForFirstStatusFrame 验证新连接在读到首个状态帧之前不放慢轮询：
// 连接就绪判定在等这一帧，放慢会让它错过宽限期而改走整套设置查询。
func TestIdlePollingWaitsForFirstStatusFrame(t *testing.T) {
	m := NewManager(testLogger{})
	m.SetIdlePolling(true)
	if got := m.readPollInterval(); got != hidReadPollInterval {
		t.Fatalf("尚无状态帧时轮询间隔 = %s, want %s", got, hidReadPollInterval)
	}
	m.currentFanData.Store(&types.FanData{})
	if got := m.readPollInterval(); got != hidIdleReadPollInterval {
		t.Fatalf("已有状态帧后轮询间隔 = %s, want %s", got, hidIdleReadPollInterval)
	}
}

// TestSingleCommandAckWaitWakesIdlePoller 验证单条命令等 ACK 时同样标记事务：
// 处于空闲休眠的读循环被立即唤醒，而不是等空闲间隔自然到期才读到 ACK。
// 智能控温每次下发转速都走这条路径，空闲轮询因此可以覆盖开着智能控温的后台场景。
func TestSingleCommandAckWaitWakesIdlePoller(t *testing.T) {
	m := NewManager(testLogger{})
	m.SetIdlePolling(true)
	m.currentFanData.Store(&types.FanData{})

	stop := make(chan struct{})
	defer close(stop)
	woke := make(chan time.Duration, 1)
	sleeping := make(chan struct{})
	go func() {
		close(sleeping)
		start := time.Now()
		m.waitPollInterval(stop)
		woke <- time.Since(start)
	}()
	<-sleeping
	time.Sleep(20 * time.Millisecond)

	func() {
		// 测试里没有真实 HID 句柄，写入会在事务标记之后 panic；这里只关心标记本身。
		defer func() { _ = recover() }()
		_, _ = m.sendHIDCommandAndWaitLocked(0x21, nil, hidControlReportLen, 10*time.Millisecond)
	}()

	select {
	case elapsed := <-woke:
		if elapsed >= hidIdleReadPollInterval/2 {
			t.Fatalf("等 ACK 时读循环 %v 后才醒来，未被立即唤醒", elapsed)
		}
	case <-time.After(hidIdleReadPollInterval * 2):
		t.Fatal("等 ACK 时读循环未被唤醒")
	}
	if got := m.activeTransactions.Load(); got != 0 {
		t.Fatalf("命令结束后事务计数 = %d, want 0", got)
	}
}
