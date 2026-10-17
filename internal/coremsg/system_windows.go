//go:build windows

package coremsg

import "golang.org/x/sys/windows"

// systemLanguageTag 返回用户首选的界面语言（如 "en-US"），取不到时返回空串。
// 与 WebView2 的 navigator.language 一样以用户界面语言为准，而非区域格式设置。
func systemLanguageTag() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(langs) == 0 {
		return ""
	}
	return langs[0]
}
