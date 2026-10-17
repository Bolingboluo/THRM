package tray

import (
	"strings"
	"testing"

	"github.com/TIANLI0/THRM/internal/coremsg"
)

// 译文比中文长，最坏读数下三行也必须塞进 64 个单元，不能靠丢行满足预算。
func TestTooltipFitsWindowsBudgetInEveryLanguage(t *testing.T) {
	t.Cleanup(func() { coremsg.SetLanguage(coremsg.Default) })

	worstCases := []Status{
		{Connected: true, AutoControlState: true, CPUTemp: 100, GPUTemp: 100, CPUPower: 155.7, GPUPower: 155.7, CurrentRPM: 4000},
		{Connected: true, AutoControlState: false, CPUTemp: 100, GPUTemp: 100, CPUPower: 155.7, GPUPower: 155.7, CurrentRPM: 4000},
	}
	for _, lang := range coremsg.Supported() {
		coremsg.SetLanguage(lang)
		for _, status := range worstCases {
			got := statusTooltip(status)
			if n := utf16Len(got); n > trayTooltipMaxUnits {
				t.Errorf("%s: 提示 %d 个单元，超出上限 %d: %q", lang, n, trayTooltipMaxUnits, got)
			}
			if lines := strings.Count(got, "\n") + 1; lines != 3 {
				t.Errorf("%s: 最坏情况下应保留三行，得到 %d 行: %q", lang, lines, got)
			}
			if !strings.Contains(got, "4000 RPM") {
				t.Errorf("%s: 缺少转速: %q", lang, got)
			}
		}
	}
}

// 中文提示必须与引入多语言之前逐字一致。
func TestTooltipChineseUnchanged(t *testing.T) {
	coremsg.SetLanguage(coremsg.ZhCN)
	if got := statusTooltip(Status{}); got != "THRM - 设备未连接" {
		t.Errorf("未连接提示 = %q", got)
	}
	got := statusTooltip(Status{Connected: true, AutoControlState: true, CPUTemp: 70, GPUTemp: 60, CurrentRPM: 2000})
	if want := "THRM - 智能变频中\nCPU 70°C  GPU 60°C\n风扇 2000 RPM"; got != want {
		t.Errorf("智能变频提示 = %q, want %q", got, want)
	}
	if got := runningTooltip(); got != "THRM - 运行中" {
		t.Errorf("通用提示 = %q", got)
	}
}

func TestTooltipFollowsLanguage(t *testing.T) {
	t.Cleanup(func() { coremsg.SetLanguage(coremsg.Default) })

	status := Status{Connected: true, CPUTemp: 70, CurrentRPM: 2000}
	coremsg.SetLanguage(coremsg.EnUS)
	if got := statusTooltip(status); !strings.Contains(got, "Manual mode") || !strings.Contains(got, "Fan 2000 RPM") {
		t.Errorf("英文提示 = %q", got)
	}
	coremsg.SetLanguage(coremsg.JaJP)
	if got := statusTooltip(Status{}); !strings.Contains(got, "デバイス未接続") {
		t.Errorf("日文提示 = %q", got)
	}
}

// 托盘未就绪时切换语言要丢掉旧语言的提示缓存，否则图标刷新会把旧语言恢复回来。
func TestRefreshLanguageWhileNotReadyDropsCachedTooltip(t *testing.T) {
	t.Cleanup(func() { coremsg.SetLanguage(coremsg.Default) })

	m := NewManager(testLogger{}, nil)
	m.lastTooltip.Store("THRM - 手动模式")
	coremsg.SetLanguage(coremsg.EnUS)
	m.RefreshLanguage()
	if got := m.currentTooltip(); got != "THRM - Running" {
		t.Errorf("切换语言后应回退到新语言的通用提示，得到 %q", got)
	}
}
