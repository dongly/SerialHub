package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

			// loadConfig 不落盘：文件应保持原值，运行期开关（flag/env）不得改变文件中的 logData
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

// TestResolveConfigPathImpl 覆盖非 Windows 配置查找顺序
// （-c > CWD > XDG；exe 同目录旧配置迁移并删除）与 Windows 保持 exe 同目录。
// xdgBase 直接注入，不依赖宿主环境与运行平台。
func TestResolveConfigPathImpl(t *testing.T) {
	writeConfig := func(t *testing.T, path, port string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("创建测试目录失败: %v", err)
		}
		content := "[serial]\nport = \"" + port + "\"\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("写入测试配置失败: %v", err)
		}
	}

	t.Run("显式路径优先", func(t *testing.T) {
		got := resolveConfigPathImpl("/explicit/config.toml", t.TempDir(), t.TempDir(), t.TempDir(), false)
		if got != "/explicit/config.toml" {
			t.Errorf("got %q, 期望显式路径原样返回", got)
		}
	})

	t.Run("exe获取失败但XDG可用时返回XDG路径", func(t *testing.T) {
		// exe 位置不可得不应短路 CWD/XDG 的独立可用性
		xdg := t.TempDir()
		want := filepath.Join(xdg, "serialhub", "config.toml")
		if got := resolveConfigPathImpl("", "", "", xdg, false); got != want {
			t.Errorf("got %q, 期望 XDG 路径 %q", got, want)
		}
	})

	t.Run("exe与XDG均不可用且CWD无配置返回空", func(t *testing.T) {
		if got := resolveConfigPathImpl("", "", "", "", false); got != "" {
			t.Errorf("got %q, 期望空", got)
		}
	})

	t.Run("CWD配置存在优先于XDG", func(t *testing.T) {
		xdg := t.TempDir()
		writeConfig(t, filepath.Join(xdg, "serialhub", "config.toml"), "XDG")

		cwd := t.TempDir()
		writeConfig(t, filepath.Join(cwd, "config.toml"), "CWD")

		got := resolveConfigPathImpl("", cwd, t.TempDir(), xdg, false)
		want := filepath.Join(cwd, "config.toml")
		if got != want {
			t.Errorf("got %q, 期望 CWD 配置 %q", got, want)
		}
	})

	t.Run("XDG不可用时CWD配置仍生效", func(t *testing.T) {
		// exe 与 XDG 均不可得，CWD 配置仍应被采用而非被提前返回短路
		cwd := t.TempDir()
		want := filepath.Join(cwd, "config.toml")
		writeConfig(t, want, "CWD")

		if got := resolveConfigPathImpl("", cwd, "", "", false); got != want {
			t.Errorf("got %q, 期望 CWD 配置 %q", got, want)
		}
	})

	t.Run("XDG存在直接使用", func(t *testing.T) {
		xdg := t.TempDir()
		want := filepath.Join(xdg, "serialhub", "config.toml")
		writeConfig(t, want, "XDG")

		got := resolveConfigPathImpl("", t.TempDir(), t.TempDir(), xdg, false)
		if got != want {
			t.Errorf("got %q, 期望 XDG 配置 %q", got, want)
		}
	})

	t.Run("exe旧配置迁移到XDG并删除原文件", func(t *testing.T) {
		xdg := t.TempDir()
		want := filepath.Join(xdg, "serialhub", "config.toml")

		exeDir := t.TempDir()
		legacy := filepath.Join(exeDir, "config.toml")
		writeConfig(t, legacy, "LEGACY")

		got := resolveConfigPathImpl("", t.TempDir(), exeDir, xdg, false)
		if got != want {
			t.Fatalf("got %q, 期望迁移后使用 XDG %q", got, want)
		}
		data, err := os.ReadFile(want)
		if err != nil || !containsPort(string(data), "LEGACY") {
			t.Errorf("XDG 配置内容不符合预期: %q, err=%v", data, err)
		}
		if fileExists(legacy) {
			t.Error("迁移是移动语义，旧位置文件应被删除")
		}
	})

	t.Run("XDG已有配置时不迁移", func(t *testing.T) {
		xdg := t.TempDir()
		xdgCfg := filepath.Join(xdg, "serialhub", "config.toml")
		writeConfig(t, xdgCfg, "XDG")

		exeDir := t.TempDir()
		legacy := filepath.Join(exeDir, "config.toml")
		writeConfig(t, legacy, "LEGACY")

		got := resolveConfigPathImpl("", t.TempDir(), exeDir, xdg, false)
		if got != xdgCfg {
			t.Fatalf("got %q, 期望 XDG %q", got, xdgCfg)
		}
		if !fileExists(legacy) {
			t.Error("XDG 已有配置时不应动旧位置文件")
		}
	})

	t.Run("CWD与legacy同存时用CWD且不迁移", func(t *testing.T) {
		xdg := t.TempDir()
		exeDir := t.TempDir()
		cwd := t.TempDir()
		legacy := filepath.Join(exeDir, "config.toml")
		writeConfig(t, legacy, "LEGACY")

		want := filepath.Join(cwd, "config.toml")
		writeConfig(t, want, "CWD")

		got := resolveConfigPathImpl("", cwd, exeDir, xdg, false)
		if got != want {
			t.Fatalf("got %q, 期望 CWD 配置 %q（CWD 优先于迁移）", got, want)
		}
		if !fileExists(legacy) {
			t.Error("CWD 命中时不应触发迁移（CWD 配置用而不迁）")
		}
	})

	t.Run("CWD等于exe目录时视为CWD配置", func(t *testing.T) {
		// exe 同目录的配置在 CWD 命中时按 CWD 语义使用，不触发迁移
		xdg := t.TempDir()
		dir := t.TempDir()
		want := filepath.Join(dir, "config.toml")
		writeConfig(t, want, "SAME")

		if got := resolveConfigPathImpl("", dir, dir, xdg, false); got != want {
			t.Errorf("got %q, 期望 %q", got, want)
		}
	})

	t.Run("三处皆无返回XDG路径", func(t *testing.T) {
		xdg := t.TempDir()
		want := filepath.Join(xdg, "serialhub", "config.toml")

		got := resolveConfigPathImpl("", t.TempDir(), t.TempDir(), xdg, false)
		if got != want {
			t.Errorf("got %q, 期望 XDG 路径 %q（由调用方 Save 落盘）", got, want)
		}
	})

	t.Run("迁移失败沿用旧位置且不残留临时文件", func(t *testing.T) {
		xdg := t.TempDir()
		// 目标位置被同名目录占用：fileExists 判定非文件，rename 与
		// 临时文件发布均会失败，覆盖迁移失败的回退路径。
		if err := os.MkdirAll(filepath.Join(xdg, "serialhub", "config.toml"), 0755); err != nil {
			t.Fatalf("创建同名目录失败: %v", err)
		}
		exeDir := t.TempDir()
		legacy := filepath.Join(exeDir, "config.toml")
		writeConfig(t, legacy, "LEGACY")

		got := resolveConfigPathImpl("", t.TempDir(), exeDir, xdg, false)
		if got != legacy {
			t.Fatalf("got %q, 期望迁移失败沿用旧位置 %q", got, legacy)
		}
		if !fileExists(legacy) {
			t.Error("迁移失败时旧文件必须保留")
		}
		if _, err := os.Stat(filepath.Join(xdg, "serialhub", "config.toml.migrate-tmp")); !os.IsNotExist(err) {
			t.Error("迁移失败不应残留临时文件")
		}
	})

	t.Run("Windows保持exe同目录", func(t *testing.T) {
		exeDir := t.TempDir()
		want := filepath.Join(exeDir, "config.toml")

		// CWD 与 XDG 均放置配置，Windows 语义仍应取 exe 同目录
		cwd := t.TempDir()
		writeConfig(t, filepath.Join(cwd, "config.toml"), "CWD")
		writeConfig(t, want, "EXE")

		got := resolveConfigPathImpl("", cwd, exeDir, t.TempDir(), true)
		if got != want {
			t.Errorf("got %q, 期望 exe 同目录 %q", got, want)
		}
	})

	t.Run("用户配置目录不可用时退回exe同目录", func(t *testing.T) {
		exeDir := t.TempDir()
		want := filepath.Join(exeDir, "config.toml")

		if got := resolveConfigPathImpl("", t.TempDir(), exeDir, "", false); got != want {
			t.Errorf("got %q, 期望退回 exe 同目录 %q", got, want)
		}
	})
}

// TestLoadConfig_FirstRunWritesUserConfig 覆盖三处皆无时的新建路径：
// resolveConfigPath 对自动选择的路径创建父目录；loadConfig 本身不落盘
// （避免被拒绝的重复实例写配置），成为主实例后 persistConfig 首次落盘。
func TestLoadConfig_FirstRunWritesUserConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 的自动路径是 exe 同目录，不走 XDG 新建路径")
	}
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// 隔离 CWD：防止测试运行目录恰好有 config.toml 被优先采用。
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取 CWD 失败: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("切换 CWD 失败: %v", err)
	}
	defer func() {
		if err := os.Chdir(origWd); err != nil {
			t.Errorf("恢复 CWD 失败: %v", err)
		}
	}()

	// xdg/serialhub/ 子目录尚不存在，模拟全新安装
	origCfgPath, origPort, origHost := configPath, mcpPort, host
	defer func() { configPath, mcpPort, host = origCfgPath, origPort, origHost }()
	configPath = ""
	mcpPort, host = config.DefaultHTTPPort, "127.0.0.1"

	want := filepath.Join(xdg, "serialhub", "config.toml")

	cfg := loadConfig()
	if fileExists(want) {
		t.Fatalf("loadConfig 不应落盘（被拒实例不得写配置）: %q 不应存在", want)
	}

	persistConfig(cfg)
	if !fileExists(want) {
		t.Fatalf("成为主实例后应在 %q 落盘默认配置", want)
	}
	if _, err := config.Load(want); err != nil {
		t.Errorf("落盘的配置应可解析: %v", err)
	}
}

// TestMigrateConfigFile_NoOverwriteExistingTarget 覆盖并发迁移防护：目标已被
// 另一进程迁移完成时直接采用，不覆盖、不重复迁移。
func TestMigrateConfigFile_NoOverwriteExistingTarget(t *testing.T) {
	src := filepath.Join(t.TempDir(), "config.toml")
	dst := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(src, []byte("[serial]\nport = \"SRC\"\n"), 0644); err != nil {
		t.Fatalf("写入源配置失败: %v", err)
	}
	if err := os.WriteFile(dst, []byte("[serial]\nport = \"DST\"\n"), 0644); err != nil {
		t.Fatalf("写入目标配置失败: %v", err)
	}

	if err := migrateConfigFile(src, dst); err != nil {
		t.Fatalf("目标已存在应视为并发迁移完成（返回 nil），got %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil || !containsPort(string(data), "DST") {
		t.Errorf("目标文件被覆盖: %q, err=%v", data, err)
	}
}

// TestLoadConfig_LogDirEnvOverridesFile 验证 SERIALHUB_LOG_DIR 在配置
// 加载后应用，能覆盖配置文件中的 logDir。用显式 -c 路径隔离，不依赖
// resolver 的路径选择，也不受 CWD/XDG 环境影响。
func TestLoadConfig_LogDirEnvOverridesFile(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.toml")

	base := config.GetDefault()
	base.LogDir = "/from/file"
	if err := config.Save(cfgFile, base); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	origCfgPath, origPort, origHost := configPath, mcpPort, host
	defer func() { configPath, mcpPort, host = origCfgPath, origPort, origHost }()
	configPath = cfgFile
	mcpPort, host = config.DefaultHTTPPort, "127.0.0.1"

	t.Setenv("SERIALHUB_LOG_DIR", "/from/env")

	cfg := loadConfig()
	if cfg.LogDir != "/from/env" {
		t.Errorf("cfg.LogDir = %q, 期望环境变量 /from/env 覆盖配置文件值", cfg.LogDir)
	}
}

// TestMigrateConfigFile_ConcurrentNoClobber 两个迁移同时进行时，发布
// 走 link 原子不覆盖：目标内容是其中一个源的完整副本（bytes.Equal，
// 绝不交叉损坏）；输掉发布竞争的一方保留自己的源文件；不残留临时
// 文件。通过注入 osLink 让「源 → 目标」的 link 恒返回跨设备错误，
// 确定性覆盖 CreateTemp+link 复制 fallback 路径（临时文件发布用
// 真实 link）。
func TestMigrateConfigFile_ConcurrentNoClobber(t *testing.T) {
	origLink := osLink
	defer func() { osLink = origLink }()
	// 模拟跨设备错误：既非 EEXIST 也非 ENOENT，migrateConfigFile 会
	// 落入 CreateTemp 复制 fallback（无需真实 syscall.EXDEV，跨平台）。
	errCrossDevice := errors.New("simulated cross-device link")
	osLink = func(oldname, newname string) error {
		// 源文件（config.toml 结尾且非临时文件）的 link 视为跨设备；
		// 临时文件（.serialhub-migrate-* 前缀）的发布走真实 link。
		if strings.HasPrefix(filepath.Base(oldname), ".serialhub-migrate-") {
			return origLink(oldname, newname)
		}
		return errCrossDevice
	}

	dirA, dirB, dirDst := t.TempDir(), t.TempDir(), t.TempDir()
	srcA := filepath.Join(dirA, "config.toml")
	srcB := filepath.Join(dirB, "config.toml")
	dst := filepath.Join(dirDst, "config.toml")
	bodyA := []byte("[serial]\nport = \"AAAA\"\n")
	bodyB := []byte("[serial]\nport = \"BBBB\"\n")
	if err := os.WriteFile(srcA, bodyA, 0644); err != nil {
		t.Fatalf("写入源 A 失败: %v", err)
	}
	if err := os.WriteFile(srcB, bodyB, 0644); err != nil {
		t.Fatalf("写入源 B 失败: %v", err)
	}

	const rounds = 20
	for i := 0; i < rounds; i++ {
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			t.Fatalf("清理上一轮目标失败: %v", err)
		}
		if err := os.WriteFile(srcA, bodyA, 0644); err != nil {
			t.Fatalf("重写源 A 失败: %v", err)
		}
		if err := os.WriteFile(srcB, bodyB, 0644); err != nil {
			t.Fatalf("重写源 B 失败: %v", err)
		}
		// 双参与者屏障：两个迁移都到达「临时文件发布」后一起放行，
		// 确定性触发一胜一负的发布竞争（而非依赖多轮概率覆盖）。
		// 放行前目标必然不存在（双方都被挡在发布前），因此双方都
		// 不会走 fileExists 快路径，必然进入 fallback。
		arrived := make(chan struct{}, 2)
		release := make(chan struct{})
		var mu sync.Mutex
		publishes := 0
		osLink = func(oldname, newname string) error {
			if !strings.HasPrefix(filepath.Base(oldname), ".serialhub-migrate-") {
				return errCrossDevice
			}
			mu.Lock()
			publishes++
			mu.Unlock()
			arrived <- struct{}{}
			<-release
			return origLink(oldname, newname)
		}
		errs := make(chan error, 2)
		go func() { errs <- migrateConfigFile(srcA, dst) }()
		go func() { errs <- migrateConfigFile(srcB, dst) }()
		<-arrived
		<-arrived
		close(release)
		// 先收齐两个结果再判错，避免失败路径提前恢复全局 osLink
		// 造成与未完成 goroutine 的竞态。
		r1, r2 := <-errs, <-errs
		if r1 != nil || r2 != nil {
			t.Fatalf("第 %d 轮并发迁移返回错误: %v / %v", i, r1, r2)
		}
		mu.Lock()
		publishCount := publishes
		mu.Unlock()
		if publishCount != 2 {
			t.Fatalf("第 %d 轮临时文件发布次数 = %d, 期望 2（双方均应走 fallback 发布）", i, publishCount)
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("第 %d 轮读取目标失败: %v", i, err)
		}
		isA := bytes.Equal(got, bodyA)
		isB := bytes.Equal(got, bodyB)
		if !isA && !isB {
			t.Fatalf("第 %d 轮目标内容非任一源的完整副本: %q", i, got)
		}
		// 胜者的源文件已删（移动语义）；败者的源文件必须保留且内容
		// 不变——输掉发布竞争不得丢失自己的配置。
		var winnerSrc, loserSrc string
		var loserWant []byte
		if isA {
			winnerSrc, loserSrc, loserWant = srcA, srcB, bodyB
		} else {
			winnerSrc, loserSrc, loserWant = srcB, srcA, bodyA
		}
		if _, err := os.Stat(winnerSrc); !os.IsNotExist(err) {
			t.Errorf("第 %d 轮胜者源文件应已删除（移动语义）: %v", i, err)
		}
		loserData, err := os.ReadFile(loserSrc)
		if err != nil {
			t.Fatalf("第 %d 轮败者源文件丢失: %v", i, err)
		}
		if !bytes.Equal(loserData, loserWant) {
			t.Fatalf("第 %d 轮败者源文件内容被篡改", i)
		}
	}

	// 不残留临时文件。
	entries, err := os.ReadDir(dirDst)
	if err != nil {
		t.Fatalf("读取目标目录失败: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "config.toml" {
			t.Errorf("目标目录残留临时文件: %s", e.Name())
		}
	}
}

func containsPort(content, port string) bool {
	return strings.Contains(content, "port = \""+port+"\"")
}

// TestDefaultLogDir 验证默认日志目录跟随用户配置目录（非 Windows）。
func TestDefaultLogDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 默认日志目录保持 exe 同目录，由 Windows 环境验证")
	}
	t.Run("XDG_CONFIG_HOME绝对路径优先", func(t *testing.T) {
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)

		want := filepath.Join(xdg, "serialhub", "logs")
		if got := defaultLogDir(); got != want {
			t.Errorf("defaultLogDir() = %q, 期望 %q", got, want)
		}
	})
	t.Run("XDG相对路径时回退HOME下的config", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", "relative/path")
		t.Setenv("HOME", home)

		want := filepath.Join(home, ".config", "serialhub", "logs")
		if got := defaultLogDir(); got != want {
			t.Errorf("defaultLogDir() = %q, 期望 %q", got, want)
		}
	})
}

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
