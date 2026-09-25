package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TIANLI0/THRM/internal/types"
)

type testLogger struct{}

func (l testLogger) Info(format string, v ...any)  {}
func (l testLogger) Error(format string, v ...any) {}
func (l testLogger) Warn(format string, v ...any)  {}
func (l testLogger) Debug(format string, v ...any) {}
func (l testLogger) Close()                        {}
func (l testLogger) CleanOldLogs()                 {}
func (l testLogger) SetDebugMode(enabled bool)     {}
func (l testLogger) GetLogDir() string             { return "" }

func isolateUserDirs(t *testing.T, homeDir string) {
	t.Helper()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(homeDir, ".local", "state"))
}

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp", testLogger{})
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.GetDefaultConfigDir() == "" {
		t.Error("GetDefaultConfigDir returned empty")
	}
}

func TestGetDefaultConfigDir(t *testing.T) {
	m := NewManager("/tmp", testLogger{})
	dir := m.GetDefaultConfigDir()
	if dir == "" {
		t.Error("GetDefaultConfigDir should not be empty")
	}
}

func TestSetAndGet(t *testing.T) {
	m := NewManager("/tmp", testLogger{})
	cfg := types.GetDefaultConfig(false)
	m.Set(cfg)
	got := m.Get()
	if got.AutoControl != cfg.AutoControl {
		t.Error("Get() should return the set config")
	}
}

func TestGetWithRevision(t *testing.T) {
	m := NewManager("/tmp", testLogger{})
	cfg := types.GetDefaultConfig(false)
	m.Set(cfg)
	_, rev1 := m.GetWithRevision()
	m.Set(cfg)
	_, rev2 := m.GetWithRevision()
	if rev2 <= rev1 {
		t.Errorf("revision should increment: %d <= %d", rev2, rev1)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	isolateUserDirs(t, tmpDir)

	m := NewManager("/tmp", testLogger{})
	cfg := types.GetDefaultConfig(false)
	cfg.AutoControl = true
	m.Set(cfg)

	if err := m.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	m2 := NewManager("/tmp", testLogger{})
	loaded := m2.Load(false)
	if loaded.AutoControl != true {
		t.Error("Loaded config should have AutoControl = true")
	}
}

func TestUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	isolateUserDirs(t, tmpDir)
	m := NewManager("/tmp", testLogger{})
	cfg := types.GetDefaultConfig(false)
	m.Set(cfg)

	cfg.AutoControl = true
	if err := m.Update(cfg); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	got := m.Get()
	if got.AutoControl != true {
		t.Error("Update should persist changes")
	}
}

func TestGetInstallDir(t *testing.T) {
	dir := GetInstallDir()
	if dir == "" {
		t.Error("GetInstallDir should not be empty")
	}
}

func TestGetCurrentWorkingDir(t *testing.T) {
	dir := GetCurrentWorkingDir()
	if dir == "" {
		t.Error("GetCurrentWorkingDir should not be empty")
	}
}

func TestValidateFanCurve_Valid(t *testing.T) {
	curve := types.GetDefaultFanCurve()
	if err := ValidateFanCurve(curve); err != nil {
		t.Errorf("ValidateFanCurve should pass for default curve: %v", err)
	}
}

func TestValidateFanCurve_TooFewPoints(t *testing.T) {
	curve := []types.FanCurvePoint{{Temperature: 30, RPM: 800}}
	if err := ValidateFanCurve(curve); err == nil {
		t.Error("ValidateFanCurve should fail for < 2 points")
	}
}

func TestValidateFanCurve_NonIncreasingTemp(t *testing.T) {
	curve := []types.FanCurvePoint{
		{Temperature: 50, RPM: 1500},
		{Temperature: 30, RPM: 2000},
	}
	if err := ValidateFanCurve(curve); err == nil {
		t.Error("ValidateFanCurve should fail for decreasing temperature")
	}
}

func TestValidateFanCurve_DecreasingRPM(t *testing.T) {
	curve := []types.FanCurvePoint{
		{Temperature: 30, RPM: 2000},
		{Temperature: 50, RPM: 1000},
	}
	if err := ValidateFanCurve(curve); err == nil {
		t.Error("ValidateFanCurve should fail for decreasing RPM")
	}
}

func TestValidateFanCurve_OutOfRange(t *testing.T) {
	curve := []types.FanCurvePoint{
		{Temperature: 30, RPM: 5001},
		{Temperature: 50, RPM: 5001},
	}
	if err := ValidateFanCurve(curve); err == nil {
		t.Error("ValidateFanCurve should fail for RPM out of range")
	}
}

func TestValidateFanCurve_Accepts5000RPM(t *testing.T) {
	curve := []types.FanCurvePoint{
		{Temperature: 30, RPM: 0},
		{Temperature: 50, RPM: 1000},
		{Temperature: 90, RPM: 5000},
	}
	if err := ValidateFanCurve(curve); err != nil {
		t.Fatalf("ValidateFanCurve rejected 0..5000 RPM curve: %v", err)
	}
}

func TestConfigDirConsistency(t *testing.T) {
	m := NewManager("/tmp", testLogger{})
	if m.GetConfigDir() == "" {
		t.Error("GetConfigDir should not be empty")
	}
	if m.GetConfigDir() != m.GetDefaultConfigDir() {
		t.Error("GetConfigDir should equal GetDefaultConfigDir")
	}
}

func TestLegacyConfigDir(t *testing.T) {
	m := NewManager("/tmp", testLogger{})
	legacy := m.GetLegacyConfigDir()
	if legacy == "" {
		t.Error("GetLegacyConfigDir should not be empty")
	}
	if legacy == m.GetDefaultConfigDir() {
		t.Error("Legacy and default config dirs should differ")
	}
}

func TestLoad_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	isolateUserDirs(t, tmpDir)

	cfgDir := NewManager("/tmp", testLogger{}).GetDefaultConfigDir()
	os.RemoveAll(cfgDir)
	defer os.RemoveAll(cfgDir)

	m := NewManager("/tmp", testLogger{})
	cfg := m.Load(false)
	if len(cfg.FanCurve) == 0 {
		t.Error("Load with no config should return default config with FanCurve")
	}
}

func TestMutateOnlyBumpsRevisionWhenChanged(t *testing.T) {
	m := NewManager("/tmp", testLogger{})
	m.Set(types.GetDefaultConfig(false))
	_, rev := m.GetWithRevision()

	if _, changed := m.Mutate(func(cfg *types.AppConfig) bool { return false }); changed {
		t.Fatal("fn 返回 false 时不应报告改动")
	}
	if _, got := m.GetWithRevision(); got != rev {
		t.Fatalf("未改动时修订号不应变化: %d -> %d", rev, got)
	}

	after, changed := m.Mutate(func(cfg *types.AppConfig) bool {
		cfg.AutoControl = !cfg.AutoControl
		return true
	})
	if !changed || after.AutoControl == types.GetDefaultConfig(false).AutoControl {
		t.Fatal("Mutate 应应用改动并返回新配置")
	}
	if _, got := m.GetWithRevision(); got == rev {
		t.Fatal("改动后修订号应递增")
	}
}

// TestSaveLeavesNoTempFiles 验证原子保存不会在配置目录留下临时文件。
func TestSaveLeavesNoTempFiles(t *testing.T) {
	tmpDir := t.TempDir()
	isolateUserDirs(t, tmpDir)
	m := NewManager("/tmp", testLogger{})
	m.Set(types.GetDefaultConfig(false))
	for range 3 {
		if err := m.Save(); err != nil {
			t.Fatalf("Save error: %v", err)
		}
	}

	entries, err := os.ReadDir(m.GetDefaultConfigDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "config.json" {
			t.Fatalf("配置目录残留了多余文件: %s", entry.Name())
		}
	}
}

// TestLoadPreservesCorruptConfig 验证损坏的配置文件被改名保留，而不是被默认配置直接覆盖。
func TestLoadPreservesCorruptConfig(t *testing.T) {
	tmpDir := t.TempDir()
	isolateUserDirs(t, tmpDir)
	m := NewManager("", testLogger{})
	configDir := m.GetDefaultConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.json")
	corrupt := []byte(`{"autoControl": tr`)
	if err := os.WriteFile(configPath, corrupt, 0644); err != nil {
		t.Fatal(err)
	}

	m.Load(false)

	backup, err := os.ReadFile(configPath + ".corrupt")
	if err != nil {
		t.Fatalf("损坏的配置文件应被保留: %v", err)
	}
	if string(backup) != string(corrupt) {
		t.Fatalf("保留的内容不一致: %q", backup)
	}
}
