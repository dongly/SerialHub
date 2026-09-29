package i18n

import (
	"os"
	"runtime"
	"testing"
)

// withLangEnv 隔离语言相关环境变量并设置给定值（空字符串=删除）。
func withLangEnv(t *testing.T, serialhubLang, lang string) {
	t.Helper()
	for _, k := range []string{"SERIALHUB_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		old, present := os.LookupEnv(k)
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
	if serialhubLang != "" {
		t.Setenv("SERIALHUB_LANG", serialhubLang)
	}
	if lang != "" {
		t.Setenv("LANG", lang)
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name         string
		serialhubEnv string // SERIALHUB_LANG，空=不设置
		langEnv      string // LANG，空=不设置
		want         Lang
	}{
		{"强制中文", "zh", "en_US.UTF-8", Zh},
		{"强制英文", "en", "zh_CN.UTF-8", En},
		{"强制中文全拼", "zh_CN.UTF-8", "en_US.UTF-8", Zh},
		{"系统中文", "", "zh_CN.UTF-8", Zh},
		{"系统中文简写", "", "zh", Zh},
		{"系统英文", "", "en_US.UTF-8", En},
		{"系统 C 兜底英文", "", "C", En},
		{"系统 POSIX 兜底英文", "", "POSIX", En},
		{"全未设置兜底英文", "", "", En},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.serialhubEnv == "" && runtime.GOOS == "windows" {
				t.Skip("Windows 分支忽略 LANG，改用实际 UI 语言；测试不能假设系统语言")
			}
			withLangEnv(t, tc.serialhubEnv, tc.langEnv)
			if got := detect(); got != tc.want {
				t.Errorf("detect() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestT(t *testing.T) {
	orig := lang
	t.Cleanup(func() { lang = orig })

	lang = Zh
	if got := T("串口未连接", "serial port not connected"); got != "串口未连接" {
		t.Errorf("T(zh 环境) = %q", got)
	}
	lang = En
	if got := T("串口未连接", "serial port not connected"); got != "serial port not connected" {
		t.Errorf("T(en 环境) = %q", got)
	}
}

func TestParseLangTag(t *testing.T) {
	for tag, want := range map[string]Lang{
		"zh": Zh, "zh_CN": Zh, "zh_CN.UTF-8": Zh, "zh_TW": Zh, "ZH_cn": Zh, "ZH": Zh,
		"en": En, "en_US": En, "C": En, "POSIX": En, "": En, "ja": En,
	} {
		if got := parseLangTag(tag); got != want {
			t.Errorf("parseLangTag(%q) = %v, want %v", tag, got, want)
		}
	}
}
