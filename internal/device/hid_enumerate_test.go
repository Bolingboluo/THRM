package device

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TIANLI0/THRM/internal/types"
)

// 一次枚举后的选取结果必须与逐个 hid.OpenFirst(VendorID, pid) 一致：
// 按优先级顺序，每个产品 ID 取枚举顺序中的第一个接口。
func TestPickHIDCandidatesMatchesOpenFirstOrder(t *testing.T) {
	found := []hidCandidate{
		{productID: 0x9999, path: "unrelated"},
		{productID: ProductIDBS2, path: "bs2-a"},
		{productID: ProductIDBS3PRO, path: "bs3pro-a"},
		{productID: ProductIDBS2PRO, path: "bs2pro-a"},
		{productID: ProductIDBS2PRO, path: "bs2pro-b"},
		{productID: ProductIDBS2, path: "bs2-b"},
	}
	got := pickHIDCandidates(found, supportedHIDProductIDs)
	want := []hidCandidate{
		{productID: ProductIDBS2PRO, path: "bs2pro-a"},
		{productID: ProductIDBS3PRO, path: "bs3pro-a"},
		{productID: ProductIDBS2, path: "bs2-a"},
	}
	if len(got) != len(want) {
		t.Fatalf("pickHIDCandidates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pickHIDCandidates[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	if got := pickHIDCandidates(nil, supportedHIDProductIDs); len(got) != 0 {
		t.Fatalf("empty enumeration should yield no candidates, got %v", got)
	}
}

type levelRecorder struct {
	mu      sync.Mutex
	entries []string
}

func (l *levelRecorder) add(level, format string, v ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, level+" "+fmt.Sprintf(format, v...))
}
func (l *levelRecorder) Info(format string, v ...any)  { l.add("INFO", format, v...) }
func (l *levelRecorder) Error(format string, v ...any) { l.add("ERROR", format, v...) }
func (l *levelRecorder) Warn(format string, v ...any)  { l.add("WARN", format, v...) }
func (l *levelRecorder) Debug(format string, v ...any) { l.add("DEBUG", format, v...) }
func (l *levelRecorder) Close()                        {}
func (l *levelRecorder) CleanOldLogs()                 {}
func (l *levelRecorder) SetDebugMode(bool)             {}
func (l *levelRecorder) GetLogDir() string             { return "" }

func (l *levelRecorder) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.entries
	l.entries = nil
	return out
}

func countLevel(entries []string, level string) int {
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e, level+" ") {
			n++
		}
	}
	return n
}

// 设备不在时自动重连只枚举一次，连续失败的第一次照常记录错误，之后降到 Debug，
// 并按 hidAbsentSummaryEvery 输出 Info 汇总；同时报告可以放慢周期性重连。
func TestReconnectWithoutDeviceEnumeratesOnceAndQuietsRepeats(t *testing.T) {
	enumerations := 0
	original := enumerateVendorHID
	enumerateVendorHID = func(vendorID uint16) []hidCandidate {
		if vendorID != VendorID {
			t.Fatalf("unexpected vendor 0x%04X", vendorID)
		}
		enumerations++
		return nil
	}
	t.Cleanup(func() { enumerateVendorHID = original })

	// BS1 走独立的枚举函数，必须一起置空：否则本机真的插着 BS1 时这里会连上，
	// 测试结果就取决于跑测试的机器有没有接设备。
	originalBS1 := enumerateBS1HID
	enumerateBS1HID = func() (string, string, bool) { return "", "", false }
	t.Cleanup(func() { enumerateBS1HID = originalBS1 })

	logs := &levelRecorder{}
	m := NewManager(logs)
	m.SeedLastTransport(types.DeviceTypeHID) // 上次是 HID：不会走 BS1 的 BLE 扫描

	if m.ReconnectPollingRelaxable() {
		t.Fatal("before any attempt polling must not be relaxed")
	}

	if ok, _ := m.Reconnect(); ok {
		t.Fatal("Reconnect unexpectedly succeeded")
	}
	if enumerations != 1 {
		t.Fatalf("expected a single HID enumeration per attempt, got %d", enumerations)
	}
	first := logs.take()
	if countLevel(first, "ERROR") != 1 || countLevel(first, "INFO") != 1 {
		t.Fatalf("first failure should be logged at the normal level, got %v", first)
	}
	if !m.ReconnectPollingRelaxable() {
		t.Fatal("absent HID device with HID last transport should allow relaxed polling")
	}

	for i := 2; i <= hidAbsentSummaryEvery; i++ {
		m.Reconnect()
		entries := logs.take()
		if countLevel(entries, "ERROR") != 0 {
			t.Fatalf("attempt %d: repeated failure must not log errors: %v", i, entries)
		}
		wantInfo := 0
		if i%hidAbsentSummaryEvery == 0 {
			wantInfo = 1
		}
		if got := countLevel(entries, "INFO"); got != wantInfo {
			t.Fatalf("attempt %d: info lines = %d, want %d: %v", i, got, wantInfo, entries)
		}
	}
	if enumerations != hidAbsentSummaryEvery {
		t.Fatalf("expected %d enumerations, got %d", hidAbsentSummaryEvery, enumerations)
	}

	// 设备重新出现后连续计数清零（打开失败也说明设备在，不能放慢）。
	enumerateVendorHID = func(uint16) []hidCandidate {
		return []hidCandidate{{productID: ProductIDBS2PRO, path: "thrm-invalid-test-path"}}
	}
	m.Reconnect()
	if m.ReconnectPollingRelaxable() {
		t.Fatal("a present-but-unopenable device must keep the normal reconnect cadence")
	}
}

func TestReconnectPollingRelaxable(t *testing.T) {
	tests := []struct {
		name        string
		connected   bool
		absent      int
		lastType    string
		lastBLEScan time.Duration // <0 表示从未扫描
		want        bool
	}{
		{name: "device present but open failed", absent: 0, lastType: types.DeviceTypeHID, lastBLEScan: -1, want: false},
		{name: "connected", connected: true, absent: 3, lastType: types.DeviceTypeHID, lastBLEScan: -1, want: false},
		{name: "absent after HID session", absent: 1, lastType: types.DeviceTypeHID, lastBLEScan: -1, want: true},
		{name: "BS1 session keeps cadence", absent: 5, lastType: types.DeviceTypeBLE, lastBLEScan: time.Second, want: false},
		{name: "never connected, BLE scan due", absent: 2, lastBLEScan: -1, want: false},
		{name: "never connected, BLE scan cooling down", absent: 2, lastBLEScan: time.Minute, want: true},
		{name: "never connected, BLE cooldown elapsed", absent: 2, lastBLEScan: idleBLEScanCooldown + time.Second, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(testLogger{})
			m.isConnected = tc.connected
			m.hidAbsentStreak = tc.absent
			m.lastDeviceType = tc.lastType
			if tc.lastBLEScan >= 0 {
				m.lastBLEScanAt = time.Now().Add(-tc.lastBLEScan)
			}
			if got := m.ReconnectPollingRelaxable(); got != tc.want {
				t.Fatalf("ReconnectPollingRelaxable() = %v, want %v", got, tc.want)
			}
		})
	}
}
