package config

import (
	"encoding/json"

	"github.com/TIANLI0/THRM/internal/coremsg"
	"github.com/TIANLI0/THRM/internal/types"
)

// applyMissingUILanguageDefaults 为引入界面语言字段之前的旧配置补上 zh-CN。
//
// 旧版本的托盘与通知一律是中文；升级后在 GUI 上报真实语言之前保持中文，
// 才不会让开机自启、尚未打开过界面的老用户突然看到别的语言。
// 全新安装的默认配置该字段为空，由核心按系统界面语言推断（coremsg.Resolve）。
func applyMissingUILanguageDefaults(cfg *types.AppConfig, rawConfig map[string]json.RawMessage) {
	if cfg == nil {
		return
	}
	if _, ok := rawConfig["uiLanguage"]; !ok {
		cfg.UILanguage = coremsg.ZhCN
		return
	}
	if lang, ok := coremsg.Normalize(cfg.UILanguage); ok {
		cfg.UILanguage = lang
	} else {
		cfg.UILanguage = ""
	}
}
