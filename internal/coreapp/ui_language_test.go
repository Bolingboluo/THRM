package coreapp

import (
	"errors"
	"fmt"
	"testing"

	"github.com/TIANLI0/THRM/internal/config"
	"github.com/TIANLI0/THRM/internal/coremsg"
	"github.com/TIANLI0/THRM/internal/types"
)

func newUILanguageTestApp(t *testing.T) *CoreApp {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Cleanup(func() { coremsg.SetLanguage(coremsg.Default) })

	app := &CoreApp{configManager: config.NewManager(tmpDir, nil)}
	cfg := types.GetDefaultConfig(false)
	cfg.UILanguage = coremsg.ZhCN
	app.configManager.Set(cfg)
	return app
}

func TestSetUILanguageAppliesAndPersistsOnlyOnChange(t *testing.T) {
	app := newUILanguageTestApp(t)

	lang, err := app.SetUILanguage("en-US")
	if err != nil || lang != coremsg.EnUS {
		t.Fatalf("SetUILanguage(en-US) = (%q, %v)", lang, err)
	}
	if got := app.configManager.Get().UILanguage; got != coremsg.EnUS {
		t.Fatalf("配置中的界面语言 = %q", got)
	}
	if got := coremsg.Language(); got != coremsg.EnUS {
		t.Fatalf("核心文案语言 = %q", got)
	}

	// GUI 每次启动/重连都会上报：语言没变时不能产生配置写入。
	_, rev := app.configManager.GetWithRevision()
	if _, err := app.SetUILanguage("en-US"); err != nil {
		t.Fatal(err)
	}
	if _, got := app.configManager.GetWithRevision(); got != rev {
		t.Fatalf("重复上报相同语言不应改动配置: 修订号 %d -> %d", rev, got)
	}

	if _, err := app.SetUILanguage("fr-FR"); err == nil {
		t.Fatal("不支持的语言应报错")
	}
	if got := coremsg.Language(); got != coremsg.EnUS {
		t.Fatalf("报错后语言不应改变，得到 %q", got)
	}
}

// GUI 手上的配置副本可能早于最近一次语言切换，整份提交时不得把语言回退。
func TestUpdateConfigKeepsUILanguage(t *testing.T) {
	app := newUILanguageTestApp(t)
	if _, err := app.SetUILanguage("ja-JP"); err != nil {
		t.Fatal(err)
	}

	stale := types.GetDefaultConfig(false)
	stale.UILanguage = coremsg.ZhCN
	if err := app.UpdateConfig(stale); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}
	if got := app.configManager.Get().UILanguage; got != coremsg.JaJP {
		t.Fatalf("UpdateConfig 不应改写界面语言，得到 %q", got)
	}
}

func TestHotkeyErrorMessage(t *testing.T) {
	t.Cleanup(func() { coremsg.SetLanguage(coremsg.Default) })
	saveErr := errors.New("disk full")
	wrapped := fmt.Errorf("context: %w", errNoCurveProfiles)

	// 中文界面下必须与此前直接使用 err.Error() 的输出逐字一致。
	coremsg.SetLanguage(coremsg.ZhCN)
	for _, err := range []error{errNoCurveProfiles, errAutoControlBlockedByCustomSpeed, saveErr} {
		if got := hotkeyErrorMessage(err); got != err.Error() {
			t.Errorf("zh: hotkeyErrorMessage(%v) = %q", err, got)
		}
	}
	switchErr := &hotkeyMessageError{msg: coremsg.T(coremsg.HotkeySwitchManualFailed) + ": " + saveErr.Error(), err: saveErr}
	if got := hotkeyErrorMessage(switchErr); got != "切换到手动模式失败: disk full" {
		t.Errorf("zh: 切换失败文案 = %q", got)
	}
	if !errors.Is(switchErr, saveErr) {
		t.Error("hotkeyMessageError 应保留原始错误链")
	}

	coremsg.SetLanguage(coremsg.EnUS)
	cases := map[error]string{
		errNoCurveProfiles:                 "No curve profiles available",
		wrapped:                            "No curve profiles available",
		errAutoControlBlockedByCustomSpeed: "Smart control can't be enabled while custom speed is on",
		saveErr:                            "Action failed: disk full",
	}
	for err, want := range cases {
		if got := hotkeyErrorMessage(err); got != want {
			t.Errorf("en: hotkeyErrorMessage(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestCurveDisplayName(t *testing.T) {
	t.Cleanup(func() { coremsg.SetLanguage(coremsg.Default) })

	coremsg.SetLanguage(coremsg.ZhCN)
	if got := curveDisplayName("  "); got != "默认" {
		t.Errorf("zh 空名称 = %q", got)
	}
	coremsg.SetLanguage(coremsg.EnUS)
	if got := curveDisplayName(""); got != "Default" {
		t.Errorf("en 空名称 = %q", got)
	}
	if got := curveDisplayName("夜间静音"); got != "夜间静音" {
		t.Errorf("用户命名的方案不应翻译，得到 %q", got)
	}
}
