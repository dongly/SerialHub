//go:build windows

package i18n

import (
	"os"

	"golang.org/x/sys/windows"
)

// GetUserDefaultUILanguage 在 x/sys/windows 未导出，经 LazySystemDLL 直接调用
// （kernel32 导出，返回 LANGID=WORD）。
var procGetUserDefaultUILanguage = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// detect 检测界面语言：SERIALHUB_LANG > Windows 用户 UI 语言
// （GetUserDefaultUILanguage，主语言 ID 0x04 为中文），
// 识别不出中文时英文兜底。
func detect() Lang {
	if v := os.Getenv("SERIALHUB_LANG"); v != "" {
		return parseLangTag(v)
	}
	// LANGID 低 10 位为主语言 ID：0x04=Chinese, 0x09=English 等
	r, _, _ := procGetUserDefaultUILanguage.Call()
	if uint16(r)&0x3FF == 0x04 {
		return Zh
	}
	return En
}
