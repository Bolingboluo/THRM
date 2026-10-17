package temperature

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/TIANLI0/THRM/internal/types"
)

func historyTestPoint(i int, base int64) types.TemperatureHistoryPoint {
	return types.TemperatureHistoryPoint{
		Timestamp: base + int64(i)*5_000,
		CPUTemp:   50 + i%30,
		GPUTemp:   40 + i%20,
		CPUPower:  float64(10+i%50) + 0.5,
		GPUPower:  float64(20+i%80) + 0.3,
		FanRPM:    1200 + i,
		CPUFanRPM: 2000 + i,
		GPUFanRPM: 2100 + i,
	}
}

// encodeV4HistoryForTest 按升级前的 v4 整文件快照格式编码，用来验证迁移。
func encodeV4HistoryForTest(enabled bool, points []types.TemperatureHistoryPoint, events []types.TimelineEvent) []byte {
	var flags uint8
	if enabled {
		flags = historyEnabledFlag
	}
	buf := append([]byte{}, historyBinaryMagic...)
	buf = binary.LittleEndian.AppendUint16(buf, historyBinaryVersionEvents)
	buf = append(buf, flags, 0)
	buf = binary.LittleEndian.AppendUint32(buf, 5)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(points)))
	buf = binary.LittleEndian.AppendUint64(buf, uint64(time.Now().UnixMilli()))
	for _, p := range points {
		buf = appendHistoryPointPayload(buf, p)
	}
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(events)))
	for _, e := range events {
		buf = appendHistoryEventPayload(buf, e)
	}
	return buf
}

func readHistoryFileForTest(t *testing.T, path string) (version uint16, points []types.TemperatureHistoryPoint, events []types.TimelineEvent, records int, torn bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	if len(data) < historyHeaderSize || !bytes.HasPrefix(data, []byte(historyBinaryMagic)) {
		t.Fatalf("history file has no valid header (%d bytes)", len(data))
	}
	version = binary.LittleEndian.Uint16(data[4:6])
	if version != historyBinaryVersion {
		return version, nil, nil, 0, false
	}
	points, events, records, torn = decodeHistoryRecords(data[historyHeaderSize:])
	return version, points, events, records, torn
}

// 升级后第一次启动要能读出 v4 文件里的全部历史与事件；之后第一次落盘把它整体重写成 v5，
// 再接着往后追加，重启后内容与内存一致。
func TestHistoryRecorderMigratesV4File(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "history.bin")
	base := time.Now().Add(-time.Hour).UnixMilli()
	var oldPoints []types.TemperatureHistoryPoint
	for i := range 10 {
		oldPoints = append(oldPoints, historyTestPoint(i, base))
	}
	oldEvents := []types.TimelineEvent{
		{Timestamp: base + 1_000, Type: types.TimelineEventTypeMode, LabelKey: types.TimelineKeyCoreStarted},
		{Timestamp: base + 20_000, Type: types.TimelineEventTypeDisconnect, LabelKey: types.TimelineKeyDeviceDisconnected},
	}
	if err := os.WriteFile(filePath, encodeV4HistoryForTest(true, oldPoints, oldEvents), 0o644); err != nil {
		t.Fatal(err)
	}

	recorder := newTestHistoryRecorder(t, filePath, 32, 5*time.Second)
	snapshot := recorder.Snapshot()
	if !snapshot.Enabled || !slices.Equal(snapshot.Points, oldPoints) || !slices.Equal(snapshot.Events, oldEvents) {
		t.Fatalf("v4 history not loaded intact: enabled=%v points=%d events=%+v", snapshot.Enabled, len(snapshot.Points), snapshot.Events)
	}

	if _, ok := recorder.Add(types.TemperatureData{CPUTemp: 77, GPUTemp: 66, UpdateTime: base + 100_000}, &types.FanData{CurrentRPM: 1800}); !ok {
		t.Fatal("expected new point to be recorded")
	}
	if err := recorder.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	version, _, _, _, torn := readHistoryFileForTest(t, filePath)
	if version != historyBinaryVersion || torn {
		t.Fatalf("expected file rewritten as clean v%d, got v%d torn=%v", historyBinaryVersion, version, torn)
	}
	want := recorder.Snapshot()
	got := NewHistoryRecorder(filePath, 32, 5*time.Second, nil).Snapshot()
	if !slices.Equal(got.Points, want.Points) || !slices.Equal(got.Events, want.Events) || len(got.Points) != 11 {
		t.Fatalf("reloaded history differs from memory: got %d points / %d events, want %d / %d",
			len(got.Points), len(got.Events), len(want.Points), len(want.Events))
	}
}

// 断电或进程被杀时最后一次追加可能只写了一半（或被文件系统填成零）。启动不能失败，
// 之前的记录照常读出；下一次落盘整体重写，把残缺部分清掉。
func TestHistoryRecorderToleratesTornTail(t *testing.T) {
	t.Parallel()

	base := time.Now().Add(-time.Hour).UnixMilli()
	goodRecord := appendHistoryRecord(nil, historyRecordPoint, func(b []byte) []byte {
		return appendHistoryPointPayload(b, historyTestPoint(99, base))
	})
	badCRC := slices.Clone(goodRecord)
	badCRC[len(badCRC)-1] ^= 0xFF
	hugeLength := slices.Clone(goodRecord)
	binary.LittleEndian.PutUint32(hugeLength[1:5], 0xFFFFFFF0)

	tails := map[string][]byte{
		"half-record":  goodRecord[:len(goodRecord)/2],
		"zero-filled":  make([]byte, 64),
		"bad-crc":      badCRC,
		"huge-length":  hugeLength,
		"tiny-garbage": {0x01, 0x02},
	}
	for name, tail := range tails {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			filePath := filepath.Join(t.TempDir(), "history.bin")
			writer := newTestHistoryRecorder(t, filePath, 16, 5*time.Second)
			for i := range 3 {
				p := historyTestPoint(i, base)
				_, _ = writer.Add(types.TemperatureData{CPUTemp: p.CPUTemp, GPUTemp: p.GPUTemp, UpdateTime: p.Timestamp}, nil)
			}
			if err := writer.Flush(); err != nil {
				t.Fatalf("flush: %v", err)
			}
			want := writer.Snapshot().Points

			file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Write(tail); err != nil {
				t.Fatal(err)
			}
			_ = file.Close()

			logger := &historyErrorLogger{}
			reloaded := NewHistoryRecorder(filePath, 16, 5*time.Second, logger)
			t.Cleanup(func() { _ = reloaded.Flush() })
			if got := reloaded.Snapshot().Points; !slices.Equal(got, want) {
				t.Fatalf("records before the torn tail must survive: got %d points, want %d", len(got), len(want))
			}
			if logger.errors != 0 {
				t.Fatalf("a torn tail is expected after power loss and must not be reported as a load failure")
			}

			if _, ok := reloaded.Add(types.TemperatureData{CPUTemp: 70, UpdateTime: base + 60_000}, nil); !ok {
				t.Fatal("expected recording to continue after a torn tail")
			}
			if err := reloaded.Flush(); err != nil {
				t.Fatalf("flush after torn tail: %v", err)
			}
			_, points, _, _, torn := readHistoryFileForTest(t, filePath)
			if torn || len(points) != 4 {
				t.Fatalf("expected the torn tail to be compacted away: torn=%v points=%d", torn, len(points))
			}
		})
	}
}

// 常态下落盘只在文件尾追加新记录，已有内容一个字节都不动。
func TestHistoryRecorderAppendsInsteadOfRewriting(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "history.bin")
	recorder := newTestHistoryRecorder(t, filePath, 64, 5*time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	add := func(i int) {
		p := historyTestPoint(i, base)
		if _, ok := recorder.Add(types.TemperatureData{CPUTemp: p.CPUTemp, UpdateTime: p.Timestamp}, nil); !ok {
			t.Fatalf("point %d not recorded", i)
		}
	}
	add(0)
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 20; i++ {
		add(i)
		if err := recorder.Flush(); err != nil {
			t.Fatal(err)
		}
		current, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(current, previous) {
			t.Fatalf("flush %d rewrote existing content instead of appending", i)
		}
		if grown := len(current) - len(previous); grown != historyRecordOverhead+historyPointPayloadSize {
			t.Fatalf("flush %d grew the file by %d bytes, want one point record", i, grown)
		}
		previous = current
	}
}

// 记录数超过上限后压缩：被环形缓冲挤掉的旧点从文件里清出去，文件大小有界，
// 重新加载的内容始终与内存一致。
func TestHistoryRecorderCompactsWhenFileGrows(t *testing.T) {
	t.Parallel()

	const capacity = 4
	filePath := filepath.Join(t.TempDir(), "history.bin")
	recorder := newTestHistoryRecorder(t, filePath, capacity, time.Second)
	limit := 2 * (capacity + timelineEventCapacity)
	base := time.Now().Add(-2 * time.Hour).UnixMilli()

	compactions := 0
	lastRecords := 0
	for i := range 3 * limit {
		if _, ok := recorder.Add(types.TemperatureData{CPUTemp: 50 + i%40, UpdateTime: base + int64(i)*1_000}, nil); !ok {
			t.Fatalf("point %d not recorded", i)
		}
		if i%7 != 6 {
			continue
		}
		if err := recorder.Flush(); err != nil {
			t.Fatal(err)
		}
		_, _, _, records, torn := readHistoryFileForTest(t, filePath)
		if torn {
			t.Fatal("file must never be torn after a successful flush")
		}
		if records > limit+7 {
			t.Fatalf("file holds %d records, compaction limit is %d", records, limit)
		}
		if records < lastRecords {
			compactions++
		}
		lastRecords = records
	}
	if compactions == 0 {
		t.Fatal("expected the file to be compacted at least once")
	}
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	want := recorder.Snapshot().Points
	if got := NewHistoryRecorder(filePath, capacity, time.Second, nil).Snapshot().Points; !slices.Equal(got, want) {
		t.Fatalf("reloaded points differ from memory: got %v want %v", got, want)
	}
}

// 缩短保留时长会丢掉最旧的点，这在只追加的文件里表达不了，必须立即整体重写：
// 否则下次启动（甚至改回更长的保留时长后）旧点会从文件里复活。
func TestHistoryRecorderRetentionChangePersists(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "history.bin")
	perHour := int(time.Hour / DefaultHistorySampleInterval)
	recorder := newTestHistoryRecorder(t, filePath, 2*perHour, DefaultHistorySampleInterval)
	base := time.Now().Add(-3 * time.Hour).UnixMilli()
	total := perHour + perHour/2
	for i := range total {
		p := historyTestPoint(i, base)
		if _, ok := recorder.Add(types.TemperatureData{CPUTemp: p.CPUTemp, GPUTemp: p.GPUTemp, UpdateTime: p.Timestamp}, nil); !ok {
			t.Fatalf("point %d not recorded", i)
		}
	}
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}

	if err := recorder.SetRetentionHours(1); err != nil {
		t.Fatalf("shrink retention: %v", err)
	}
	if got := recorder.RetentionHours(); got != 1 {
		t.Fatalf("RetentionHours() = %d, want 1", got)
	}
	shrunk := recorder.Snapshot().Points
	if len(shrunk) != perHour || shrunk[len(shrunk)-1].Timestamp != base+int64(total-1)*5_000 {
		t.Fatalf("shrinking must keep the newest %d points, got %d", perHour, len(shrunk))
	}
	// 用更长的容量重新加载：文件里不能再有被裁掉的旧点。
	if got := NewHistoryRecorder(filePath, 2*perHour, DefaultHistorySampleInterval, nil).Snapshot().Points; !slices.Equal(got, shrunk) {
		t.Fatalf("trimmed points resurrected from disk: got %d points, want %d", len(got), len(shrunk))
	}

	if err := recorder.SetRetentionHours(2); err != nil {
		t.Fatalf("grow retention: %v", err)
	}
	if got := recorder.Snapshot().Points; !slices.Equal(got, shrunk) {
		t.Fatal("growing retention must keep every existing point")
	}
	p := historyTestPoint(total, base)
	if _, ok := recorder.Add(types.TemperatureData{CPUTemp: p.CPUTemp, UpdateTime: p.Timestamp}, nil); !ok {
		t.Fatal("expected recording to continue after a retention change")
	}
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	want := recorder.Snapshot().Points
	if got := NewHistoryRecorder(filePath, 2*perHour, DefaultHistorySampleInterval, nil).Snapshot().Points; !slices.Equal(got, want) {
		t.Fatalf("reloaded points differ after growing retention: got %d want %d", len(got), len(want))
	}
}

// 时钟回拨丢弃的"未来"点与事件也要从文件里消失，否则重启后又把缓冲区搅乱。
func TestHistoryRecorderClockRollbackPersists(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "history.bin")
	recorder := newTestHistoryRecorder(t, filePath, 16, 5*time.Second)

	now := time.Now().UnixMilli()
	future := now + 2*time.Hour.Milliseconds()
	for i := range 3 {
		if _, ok := recorder.Add(types.TemperatureData{CPUTemp: 60 + i, UpdateTime: future + int64(i)*5_000}, nil); !ok {
			t.Fatalf("pre-rollback sample %d not recorded", i)
		}
	}
	if _, ok := recorder.AddEvent(types.TimelineEventTypeMode, types.TimelineKeyCoreStarted); !ok {
		t.Fatal("expected event to be recorded")
	}
	recorder.mutex.Lock()
	recorder.events[len(recorder.events)-1].Timestamp = future // 回拨前记下的事件
	recorder.mutex.Unlock()
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}

	if _, ok := recorder.Add(types.TemperatureData{CPUTemp: 70, UpdateTime: now}, nil); !ok {
		t.Fatal("expected post-rollback sample to be recorded")
	}
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}

	want := recorder.Snapshot()
	if len(want.Points) != 1 || len(want.Events) != 0 {
		t.Fatalf("rollback should leave only the new point, got %d points / %d events", len(want.Points), len(want.Events))
	}
	got := NewHistoryRecorder(filePath, 16, 5*time.Second, nil).Snapshot()
	if !slices.Equal(got.Points, want.Points) || len(got.Events) != 0 {
		t.Fatalf("discarded future records came back from disk: %+v / %+v", got.Points, got.Events)
	}
}

// 攒够一批后由后台协程追加，不需要显式 Flush；调用方（控温循环）不在磁盘 IO 上等待。
func TestHistoryRecorderBackgroundWrite(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "history.bin")
	recorder := newTestHistoryRecorder(t, filePath, 64, 5*time.Second)
	base := time.Now().Add(-time.Hour).UnixMilli()
	waitForPoints := func(want int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			if data, err := os.ReadFile(filePath); err == nil && len(data) >= historyHeaderSize {
				if points, _, _, torn := decodeHistoryRecords(data[historyHeaderSize:]); !torn && len(points) == want {
					return
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("background writer did not persist %d points", want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	// 第一个点就会触发写盘（距上次落盘已超过 30 秒）。
	_, _ = recorder.Add(types.TemperatureData{CPUTemp: 60, UpdateTime: base}, nil)
	waitForPoints(1)
	// 之后每攒够一批触发一次。
	for i := 1; i <= dirtyFlushThreshold; i++ {
		_, _ = recorder.Add(types.TemperatureData{CPUTemp: 60, UpdateTime: base + int64(i)*5_000}, nil)
	}
	waitForPoints(1 + dirtyFlushThreshold)
}

// v5 文件头后面没有记录是合法的空历史。
func TestHistoryRecorderLoadsEmptyV5File(t *testing.T) {
	recorder := NewHistoryRecorder("", 8, 5*time.Second, nil)
	data := append([]byte{}, historyBinaryMagic...)
	data = binary.LittleEndian.AppendUint16(data, historyBinaryVersion)
	data = append(data, 0, 0)
	data = binary.LittleEndian.AppendUint32(data, 5)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint64(data, uint64(time.Now().UnixMilli()))
	if err := recorder.loadBinaryData(data); err != nil {
		t.Fatalf("load empty v5 file: %v", err)
	}
	if recorder.IsEnabled() || len(recorder.Snapshot().Points) != 0 {
		t.Fatal("expected a disabled, empty history")
	}
	if recorder.needCompact {
		t.Fatal("a clean v5 file must be appendable without a rewrite")
	}
}
