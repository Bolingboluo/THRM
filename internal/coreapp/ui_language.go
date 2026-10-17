package coreapp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TIANLI0/THRM/internal/coremsg"
	"github.com/TIANLI0/THRM/internal/types"
)

// 快捷键路径需要识别并翻译的错误。文案保持原样：它们同样经 IPC 原文返回给 GUI。
var (
	errNoCurveProfiles                 = errors.New("暂无可用温控曲线方案")
	errAutoControlBlockedByCustomSpeed = errors.New("自定义转速模式下无法开启智能变频")
)

// SetUILanguage 记录 GUI 当前的界面语言，并让托盘、系统通知、快捷键提示随之切换。
//
// 语言与上次相同时不落盘也不广播：GUI 每次启动、每次与核心重连都会上报一次，
// 这些上报绝大多数是重复的，不能变成配置写入。返回实际生效的语言。
func (a *CoreApp) SetUILanguage(lang string) (string, error) {
	normalized, ok := coremsg.Normalize(lang)
	if !ok {
		return "", fmt.Errorf("不支持的界面语言: %s", lang)
	}

	// 与 UpdateConfig 同锁：它在锁内读旧配置再整份写回，不加锁会把这里的改动覆盖掉。
	a.mutex.Lock()
	_, changed := a.configManager.Mutate(func(cfg *types.AppConfig) bool {
		if cfg.UILanguage == normalized {
			return false
		}
		cfg.UILanguage = normalized
		return true
	})
	a.mutex.Unlock()

	if changed {
		if err := a.configManager.Save(); err != nil {
			a.logError("保存界面语言失败: %v", err)
		}
		a.logInfo("界面语言已切换为 %s", normalized)
	}
	a.applyUILanguage(normalized)
	return normalized, nil
}

// initUILanguage 在启动时按配置确定核心侧文案的语言。
func (a *CoreApp) initUILanguage(configured string) {
	a.applyUILanguage(configured)
	a.logInfo("核心界面语言: %s（配置值 %q）", coremsg.Language(), configured)
}

// applyUILanguage 让核心侧文案使用指定语言（空或无法识别时按系统语言推断），
// 语言实际变化时立即刷新托盘文字。调用方不得持有 a.mutex：托盘刷新会读取状态。
func (a *CoreApp) applyUILanguage(configured string) {
	lang, changed := coremsg.SetLanguage(coremsg.Resolve(configured))
	if !changed {
		return
	}
	a.logDebug("核心界面语言: %s（配置值 %q）", lang, configured)
	if a.trayManager != nil {
		a.trayManager.RefreshLanguage()
	}
}

// curveDisplayName 是托盘里曲线方案的显示名：方案名由用户命名，原样显示；
// 只有名字为空时才用当前语言的"默认"。
func curveDisplayName(name string) string {
	if strings.TrimSpace(name) == "" {
		return coremsg.T(coremsg.CurveDefaultName)
	}
	return name
}
