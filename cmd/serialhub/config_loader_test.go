package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"

	"github.com/dongly/serialhub/pkg/config"
)

// logDataCase 覆盖数据内容日志开关「显式 flag > 环境变量 > 配置文件」的合并，
// 以及运行期开关不回写配置文件的行为。
type logDataCase struct {
	name string

	fileLogData bool    // 配置文件 logData
	envValue    *string // 环境变量 SERIALHUB_LOG_DATA（nil = 未设置）
	flagSet     bool    // flag 是否显式指定
	flagValue   bool    // flag 显式指定的值

	wantEffective bool // 期望 logDataEffective
	wantFileAfter bool // 期望 loadConfig 保存后文件中的 logData
}

func TestLoadConfigLogDataPriority(t *testing.T) {
	cases := []logDataCase{
		{
			name:          "配置文件开启",
			fileLogData:   true,
			wantEffective: true,
			wantFileAfter: true,
		},
		{
			name:          "配置文件开启_环境变量显式关闭",
			fileLogData:   true,
			envValue:      strPtr("false"),
			wantEffective: false,
			wantFileAfter: true,
		},
		{
			name:          "配置文件关闭_环境变量开启",
			envValue:      strPtr("1"),
			wantEffective: true,
			wantFileAfter: false,
		},
		{
			name:          "环境变量无效值忽略_沿用配置",
			fileLogData:   true,
			envValue:      strPtr("garbage"),
			wantEffective: true,
			wantFileAfter: true,
		},
		{
			name:          "环境变量开启_显式flag关闭",
			envValue:      strPtr("true"),
			flagSet:       true,
			flagValue:     false,
			wantEffective: false,
			wantFileAfter: false,
		},
		{
			name:          "配置开启_显式flag关闭",
			fileLogData:   true,
			flagSet:       true,
			flagValue:     false,
			wantEffective: false,
			wantFileAfter: true,
		},
		{
			name:          "显式flag开启",
			flagSet:       true,
			flagValue:     true,
			wantEffective: true,
			wantFileAfter: false,
		},
		{
			name:          "全部未设置",
			wantEffective: false,
			wantFileAfter: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.toml")

			base := config.GetDefault()
			base.LogData = tc.fileLogData
			if err := config.Save(cfgPath, base); err != nil {
				t.Fatalf("写入测试配置失败: %v", err)
			}

			// 隔离全局状态：flag 引用、env、配置路径
			origFlag, origMode, origEffective := logDataFlag, logDataMode, logDataEffective
			origCfgPath, origPort, origHost := configPath, mcpPort, host
			defer func() {
				logDataFlag, logDataMode, logDataEffective = origFlag, origMode, origEffective
				configPath, mcpPort, host = origCfgPath, origPort, origHost
			}()

			configPath = cfgPath
			mcpPort, host = config.DefaultHTTPPort, "127.0.0.1"

			logDataFlag = nil
			logDataMode = false
			if tc.flagSet {
				fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
				fs.Bool("log-data", false, "")
				_ = fs.Set("log-data", boolStr(tc.flagValue))
				logDataFlag = fs.Lookup("log-data")
				logDataMode = tc.flagValue // 模拟 cobra 解析：Set 同时写入 BoolVar 绑定的变量
			}

			// 真正取消环境变量（t.Setenv("") 会把空串当无效值告警，
			// 语义不同：LookupEnv ok=true 但 ParseBool 失败 → 沿用配置）
			origEnv, hadEnv := os.LookupEnv("SERIALHUB_LOG_DATA")
			os.Unsetenv("SERIALHUB_LOG_DATA")
			if tc.envValue != nil {
				os.Setenv("SERIALHUB_LOG_DATA", *tc.envValue)
			}
			defer func() {
				if hadEnv {
					os.Setenv("SERIALHUB_LOG_DATA", origEnv)
				} else {
					os.Unsetenv("SERIALHUB_LOG_DATA")
				}
			}()

			loadConfig()

			if logDataEffective != tc.wantEffective {
				t.Errorf("logDataEffective = %v, 期望 %v", logDataEffective, tc.wantEffective)
			}

			// loadConfig 会回写配置文件：运行期开关（flag/env）不得改变文件中的 logData
			saved, err := config.Load(cfgPath)
			if err != nil {
				t.Fatalf("读回配置失败: %v", err)
			}
			if saved.LogData != tc.wantFileAfter {
				t.Errorf("回写后文件 logData = %v, 期望 %v（运行期开关不应回写）",
					saved.LogData, tc.wantFileAfter)
			}

			// 连续两次 loadConfig：第二次不继承第一次的运行期开关
			if tc.flagSet {
				logDataFlag = nil // 模拟新进程：无显式 flag
			}
			loadConfig()
			wantSecond := tc.wantFileAfter
			if tc.envValue != nil {
				if b, err := parseBoolCompat(*tc.envValue); err == nil {
					wantSecond = b
				}
			}
			if logDataEffective != wantSecond {
				t.Errorf("第二次 loadConfig logDataEffective = %v, 期望 %v（不应继承上次运行期开关）",
					logDataEffective, wantSecond)
			}
		})
	}
}

// TestLoadConfigLogDataNilFlagSafe flag 引用为 nil（未经过 main 初始化，
// 如测试直接调用 loadConfig）时不 panic、按配置文件生效。
func TestLoadConfigLogDataNilFlagSafe(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	base := config.GetDefault()
	base.LogData = true
	if err := config.Save(cfgPath, base); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	// 隔离环境变量（外部环境可能设置了 SERIALHUB_LOG_DATA）
	origEnv, hadEnv := os.LookupEnv("SERIALHUB_LOG_DATA")
	os.Unsetenv("SERIALHUB_LOG_DATA")
	defer func() {
		if hadEnv {
			os.Setenv("SERIALHUB_LOG_DATA", origEnv)
		}
	}()

	origFlag, origEffective := logDataFlag, logDataEffective
	origCfgPath, origPort, origHost := configPath, mcpPort, host
	defer func() {
		logDataFlag, logDataEffective = origFlag, origEffective
		configPath, mcpPort, host = origCfgPath, origPort, origHost
	}()

	logDataFlag = nil
	configPath = cfgPath
	mcpPort, host = config.DefaultHTTPPort, "127.0.0.1"

	loadConfig()

	if !logDataEffective {
		t.Error("flag 为 nil 且配置开启时 logDataEffective 应为 true")
	}
}

func strPtr(s string) *string { return &s }

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// parseBoolCompat 与 config_loader 的 strconv.ParseBool 语义一致，测试用。
func parseBoolCompat(s string) (bool, error) {
	switch s {
	case "1", "t", "T", "true", "TRUE", "True":
		return true, nil
	case "0", "f", "F", "false", "FALSE", "False":
		return false, nil
	}
	return false, os.ErrInvalid
}
