package coreapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/TIANLI0/THRM/internal/config"
	"github.com/TIANLI0/THRM/internal/types"
)

func TestHealthReconnectDue(t *testing.T) {
	long := healthReconnectRelaxedInterval
	short := 30 * time.Second
	tests := []struct {
		name          string
		streak        int32
		sinceLast     time.Duration
		arrivalNotify bool
		relaxable     bool
		want          bool
	}{
		{name: "no arrival notification (Linux) keeps 30s cadence", streak: 100, sinceLast: short, relaxable: true, want: true},
		{name: "device present or BS1 keeps 30s cadence", streak: 100, sinceLast: short, arrivalNotify: true, want: true},
		{name: "first retries after disconnect stay fast", streak: healthReconnectFastAttempts - 1, sinceLast: short, arrivalNotify: true, relaxable: true, want: true},
		{name: "relaxed after fast retries", streak: healthReconnectFastAttempts, sinceLast: short, arrivalNotify: true, relaxable: true, want: false},
		{name: "relaxed interval elapsed", streak: healthReconnectFastAttempts, sinceLast: long, arrivalNotify: true, relaxable: true, want: true},
		{name: "tick jitter tolerated", streak: 50, sinceLast: long - time.Second, arrivalNotify: true, relaxable: true, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := healthReconnectDue(tc.streak, tc.sinceLast, tc.arrivalNotify, tc.relaxable); got != tc.want {
				t.Fatalf("healthReconnectDue(%d, %v, %v, %v) = %v, want %v",
					tc.streak, tc.sinceLast, tc.arrivalNotify, tc.relaxable, got, tc.want)
			}
		})
	}
}

func TestLaptopFanReadDue(t *testing.T) {
	if !laptopFanReadDue(true, false, 0) {
		t.Fatal("a connected GUI must get a fresh reading every cycle")
	}
	if !laptopFanReadDue(false, true, 0) {
		t.Fatal("a history sample must carry a fresh reading")
	}
	if laptopFanReadDue(false, false, time.Second) {
		t.Fatal("without consumers the WMI read should be skipped")
	}
	if !laptopFanReadDue(false, false, laptopFanMaxAge) {
		t.Fatal("a reading older than laptopFanMaxAge must be refreshed")
	}
	// 从未读取过（零值时间）时必须立即读取。
	if !laptopFanReadDue(false, false, time.Since(time.Time{})) {
		t.Fatal("the first cycle must read")
	}
}

func newConfigTestApp(t *testing.T) *CoreApp {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	app := &CoreApp{configManager: config.NewManager(tmpDir, nil)}
	app.configManager.Set(types.GetDefaultConfig(false))
	return app
}

func configFilePathForTest(app *CoreApp) string {
	return filepath.Join(app.configManager.GetDefaultConfigDir(), "config.json")
}

func savedConfig(t *testing.T, app *CoreApp) types.AppConfig {
	t.Helper()
	data, err := os.ReadFile(configFilePathForTest(app))
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	var cfg types.AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse saved config: %v", err)
	}
	return cfg
}

// 学习偏移的周期落盘放慢后，挂起/退出路径必须把内存里尚未落盘的结果写盘。
func TestSaveLearnedOffsetsIfDirty(t *testing.T) {
	app := newConfigTestApp(t)

	// 没有待保存内容时不写盘。
	app.saveLearnedOffsetsIfDirty("test")
	if _, err := os.Stat(configFilePathForTest(app)); err == nil {
		t.Fatal("clean state must not write the config")
	}

	learned := make([]int, len(app.configManager.Get().FanCurve))
	learned[0] = 40
	app.configManager.Mutate(func(cfg *types.AppConfig) bool {
		cfg.SmartControl.LearnedOffsets = slices.Clone(learned)
		storeSmartControlOffsetsForActiveProfile(cfg)
		return true
	})
	app.learnedOffsetsUnsaved.Store(true)

	app.saveLearnedOffsetsIfDirty("suspend")
	if app.learnedOffsetsUnsaved.Load() {
		t.Fatal("successful save must clear the pending flag")
	}
	if got := savedConfig(t, app).SmartControl.LearnedOffsets; !slices.Equal(got, learned) {
		t.Fatalf("saved learned offsets = %v, want %v", got, learned)
	}
}

func TestTrayConfigViewCopiesOnlyNeededFields(t *testing.T) {
	app := newConfigTestApp(t)
	app.configManager.Mutate(func(cfg *types.AppConfig) bool {
		cfg.AutoControl = true
		cfg.DisableGpuMonitoring = true
		cfg.FanCurveProfiles = []types.FanCurveProfile{
			{ID: "a", Name: "Quiet", Curve: cfg.FanCurve},
			{ID: "b", Name: " ", Curve: cfg.FanCurve},
		}
		cfg.ActiveFanCurveProfileID = "b"
		return true
	})

	view := app.trayConfigView()
	if !view.AutoControl || !view.DisableGpuMonitoring || view.ActiveFanCurveProfileID != "b" {
		t.Fatalf("unexpected tray view: %+v", view)
	}
	want := []trayProfileName{{ID: "a", Name: "Quiet"}, {ID: "b", Name: " "}}
	if !slices.Equal(view.FanCurveProfiles, want) {
		t.Fatalf("profiles = %v, want %v", view.FanCurveProfiles, want)
	}

	// 视图是独立副本，之后的配置改动不会反映到已取出的视图里。
	app.configManager.Mutate(func(cfg *types.AppConfig) bool {
		cfg.FanCurveProfiles[0].Name = "Changed"
		return true
	})
	if view.FanCurveProfiles[0].Name != "Quiet" {
		t.Fatal("tray view must not alias the manager's config")
	}
}
