//go:build !windows

package i18n

import "os"

// detect 检测界面语言：SERIALHUB_LANG > LC_ALL > LC_MESSAGES > LANG，
// 均未设置或识别不出中文时英文兜底。
func detect() Lang {
	if v := os.Getenv("SERIALHUB_LANG"); v != "" {
		return parseLangTag(v)
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return parseLangTag(v)
		}
	}
	return En
}
