// Package i18n 提供轻量双语支持（中文/英文）。
// 语言在进程启动时检测一次：SERIALHUB_LANG 环境变量 > 系统语言
// （Linux/macOS 看 LC_ALL/LC_MESSAGES/LANG，Windows 看 UI 语言），
// 识别不出中文环境时默认英文。
package i18n

import "strings"

// Lang 界面语言。
type Lang int

const (
	En Lang = iota // 英文（默认/兜底）
	Zh             // 中文
)

// Current 返回当前界面语言。
func Current() Lang { return lang }

// T 按当前语言选择文案：中文环境返回 zh，否则返回 en。
// 用法：errors.New(i18n.T("串口未连接", "serial port not connected"))
func T(zh, en string) string {
	if lang == Zh {
		return zh
	}
	return en
}

// lang 为进程级界面语言，init 时由 detect() 确定。
var lang = detect()

// parseLangTag 解析语言标识（如 "zh_CN.UTF-8"、"en-US"、"zh"，大小写不敏感），
// zh 前缀识别为中文，其余（含空串、C、POSIX）一律英文兜底。
func parseLangTag(v string) Lang {
	v = strings.ToLower(v)
	if strings.HasPrefix(v, "zh") {
		return Zh
	}
	return En
}
