//go:build !windows

package coremsg

import "os"

// systemLanguageTag 返回 locale 环境变量中的界面语言（如 "en-US"），未设置时返回空串。
func systemLanguageTag() string {
	return languageFromLocaleEnv(os.Getenv)
}
