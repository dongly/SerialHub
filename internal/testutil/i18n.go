package testutil

import "strings"

// I18nPrefix 返回 i18n 模板字符串中第一个格式动词（%s/%d/%w…）之前的
// 稳定前缀；无动词时返回全串。供测试对本地化输出做语言无关的 Contains
// 断言：前缀随语言取自 i18n 本身，中英文环境均成立。
func I18nPrefix(msg string) string {
	if i := strings.Index(msg, "%"); i >= 0 {
		return msg[:i]
	}
	return msg
}

// I18nSuffix 返回最后一个格式动词之后的稳定后缀（跳过动词字母本身）；
// 无动词时返回全串。适用于动词位于句首的模板（如 "%s 不存在，跳过"），
// 此时 I18nPrefix 为空串、不具区分度。
func I18nSuffix(msg string) string {
	i := strings.LastIndex(msg, "%")
	if i < 0 {
		return msg
	}
	rest := msg[i+1:]
	for len(rest) > 0 {
		c := rest[0]
		rest = rest[1:]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			break // 动词字母已消费，其后即稳定后缀
		}
	}
	return rest
}
