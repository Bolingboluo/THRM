package temperature

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TIANLI0/THRM/internal/types"
)

// SampleDue 必须与 Add 的记录判定一致，否则无 GUI 时跳过的笔记本风扇读数会让历史点缺值。
func TestHistoryRecorderSampleDueMatchesAdd(t *testing.T) {
	recorder := NewHistoryRecorder(filepath.Join(t.TempDir(), "history.bin"), 64, 5*time.Second, nil)
	enableRecorderForTest(t, recorder)

	base := time.Now().Add(-time.Hour).UnixMilli()
	offsetsMs := []int64{0, 1000, 4999, 5000, 6000, 9999, 10000, 16000, 16500, 21000}
	for _, offset := range offsetsMs {
		temp := types.TemperatureData{CPUTemp: 50, UpdateTime: base + offset}
		due := recorder.SampleDue(temp)
		_, recorded := recorder.Add(temp, nil)
		if due != recorded {
			t.Fatalf("offset %dms: SampleDue=%v but Add recorded=%v", offset, due, recorded)
		}
	}

	if recorder.SampleDue(types.TemperatureData{UpdateTime: base + 60_000}) {
		t.Fatal("samples without temperatures are never recorded")
	}

	if err := recorder.SetEnabled(false); err != nil {
		t.Fatalf("disable recorder: %v", err)
	}
	if recorder.SampleDue(types.TemperatureData{CPUTemp: 50, UpdateTime: base + 120_000}) {
		t.Fatal("a disabled recorder never records")
	}

	var nilRecorder *HistoryRecorder
	if nilRecorder.SampleDue(types.TemperatureData{CPUTemp: 50}) {
		t.Fatal("nil recorder must report false")
	}
}
