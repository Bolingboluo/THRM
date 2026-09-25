package coreapp

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/TIANLI0/THRM/internal/config"
	"github.com/TIANLI0/THRM/internal/temperature"
	"github.com/TIANLI0/THRM/internal/types"
)

// UpdateConfig 不得回退保留时长：GUI 手上的配置副本在通过专用接口改过保留时长之后
// 仍然是旧值，若整份配置提交时采信该字段，用户的选择会在下次核心启动时静默丢失。
func TestUpdateConfigKeepsHistoryRetentionSelection(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	const selectedHours = 6
	pointsPerHour := int(60 * 60 / temperature.DefaultHistorySampleInterval.Seconds())
	app := &CoreApp{
		configManager: config.NewManager(tmpDir, nil),
		tempHistory: temperature.NewHistoryRecorder(
			filepath.Join(tmpDir, "history.bin"),
			pointsPerHour*selectedHours,
			temperature.DefaultHistorySampleInterval,
			nil,
		),
	}

	staleCfg := types.GetDefaultConfig(false)
	staleCfg.TemperatureHistoryRetentionHours = types.DefaultTemperatureHistoryRetentionHours
	if err := app.UpdateConfig(staleCfg); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	if got := app.configManager.Get().TemperatureHistoryRetentionHours; got != selectedHours {
		t.Fatalf("retention hours should stay at the recorder's %d, got %d", selectedHours, got)
	}
}

func TestGetDefaultConfigHasValidHistoryRetention(t *testing.T) {
	got := types.GetDefaultConfig(false).TemperatureHistoryRetentionHours
	if got != types.DefaultTemperatureHistoryRetentionHours {
		t.Fatalf("default config retention hours = %d, want %d", got, types.DefaultTemperatureHistoryRetentionHours)
	}
}

func TestNormalizeLightStripConfig_FillsDefaults(t *testing.T) {
	emptyCfg := types.LightStripConfig{}
	normalized, changed := normalizeLightStripConfig(emptyCfg)
	if !changed {
		t.Fatal("should report changed when filling empty light strip config")
	}
	if normalized.Mode == "" {
		t.Fatal("mode should be filled with default")
	}
	if normalized.Speed == "" {
		t.Fatal("speed should be filled with default")
	}
	if len(normalized.Colors) == 0 {
		t.Fatal("colors should be filled with default")
	}
}

func TestNormalizeLightStripConfig_InvalidBrightness(t *testing.T) {
	cfg := types.LightStripConfig{Brightness: 150}
	normalized, changed := normalizeLightStripConfig(cfg)
	if !changed {
		t.Fatal("should report changed when brightness is out of range")
	}
	if normalized.Brightness == 150 {
		t.Fatal("brightness should be clamped/replaced with default")
	}
}

func TestNormalizeLightStripConfig_ValidNoChange(t *testing.T) {
	defaults := types.GetDefaultLightStripConfig()
	_, changed := normalizeLightStripConfig(defaults)
	if changed {
		t.Fatal("should not report changed for already-valid defaults")
	}
}

func TestCloneManualGearLevels_FromNil(t *testing.T) {
	result := cloneManualGearLevels(nil)
	if len(result) != 4 {
		t.Fatalf("expected 4 gears, got %d", len(result))
	}
	for _, gear := range []string{"静音", "标准", "强劲", "超频"} {
		if result[gear] != "中" {
			t.Fatalf("gear %q should default to '中', got %q", gear, result[gear])
		}
	}
}

func TestCloneManualGearLevels_PreservesValid(t *testing.T) {
	source := map[string]string{"静音": "低", "标准": "高", "强劲": "低", "超频": "高"}
	result := cloneManualGearLevels(source)
	if result["静音"] != "低" || result["超频"] != "高" {
		t.Fatalf("should preserve valid levels: %v", result)
	}
}

func TestCloneManualGearLevels_NormalizesInvalid(t *testing.T) {
	source := map[string]string{"静音": "invalid", "标准": "超高"}
	result := cloneManualGearLevels(source)
	if result["静音"] != "中" {
		t.Fatalf("invalid level should be normalized to '中', got %q", result["静音"])
	}
}

func TestRuntimeDebugInfo_ContainsKeys(t *testing.T) {
	info := runtimeDebugInfo()
	requiredKeys := []string{"goroutines", "allocMB", "heapAllocMB", "heapInUseMB", "numGC"}
	for _, key := range requiredKeys {
		if _, ok := info[key]; !ok {
			t.Fatalf("runtimeDebugInfo should contain key %q", key)
		}
	}
}

// UpdateConfig 不得采信 GUI 副本里的学习偏移：GUI 最多每 25 秒才收到一次学习结果，
// 期间任意一次设置改动都会把核心刚学到的偏移静默回退。
func TestUpdateConfigKeepsCoreLearnedOffsets(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	app := &CoreApp{configManager: config.NewManager(tmpDir, nil)}
	coreCfg := types.GetDefaultConfig(false)
	learned := make([]int, len(coreCfg.FanCurve))
	// 只在低转速段设偏移：高转速点会被 NormalizeConfig 按转速上限裁掉。
	for i := range len(learned) / 2 {
		learned[i] = 50
	}
	coreCfg.SmartControl.LearnedOffsets = learned
	if err := app.UpdateConfig(coreCfg); err != nil {
		t.Fatalf("seed UpdateConfig failed: %v", err)
	}
	// UpdateConfig 本身就会保留核心侧的值，这里直接写入核心学到的结果。
	app.configManager.Mutate(func(cfg *types.AppConfig) bool {
		cfg.SmartControl.LearnedOffsets = slices.Clone(learned)
		storeSmartControlOffsetsForActiveProfile(cfg)
		return true
	})

	staleGUI := types.GetDefaultConfig(false)
	staleGUI.SmartControl.LearnedOffsets = make([]int, len(staleGUI.FanCurve))
	staleGUI.SmartControl.LearnedOffsetsByProfile = map[string][]int{}
	staleGUI.AutoControl = !coreCfg.AutoControl
	if err := app.UpdateConfig(staleGUI); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	got := app.configManager.Get()
	if got.AutoControl != staleGUI.AutoControl {
		t.Fatal("普通设置应正常生效")
	}
	if !slices.Equal(got.SmartControl.LearnedOffsets, learned) {
		t.Fatalf("学习偏移被 GUI 副本回退: %v", got.SmartControl.LearnedOffsets)
	}
}
