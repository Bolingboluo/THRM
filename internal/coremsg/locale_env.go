package coremsg

import "strings"

// languageFromLocaleEnv 按 POSIX 优先级（LC_ALL > LC_MESSAGES > LANG）取界面语言，
// 并把 "zh_CN.UTF-8"、"ja_JP@euro" 之类的值转成 "zh-CN"/"ja-JP" 形式。
// C/POSIX 表示未设置语言，返回空串交给调用方回退。
func languageFromLocaleEnv(getenv func(string) string) string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value := strings.TrimSpace(getenv(name))
		if value == "" {
			continue
		}
		if i := strings.IndexAny(value, ".@"); i >= 0 {
			value = value[:i]
		}
		if value == "C" || value == "POSIX" {
			return ""
		}
		return strings.ReplaceAll(value, "_", "-")
	}
	return ""
}
