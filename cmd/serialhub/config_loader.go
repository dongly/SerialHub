package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/dongly/serialhub/pkg/config"
	"github.com/dongly/serialhub/pkg/serial"
	"github.com/sirupsen/logrus"
)

func loadConfig() *config.Config {
	cfg := config.GetDefault()

	configPath = resolveConfigPath()

	if configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			logrus.Warnf("[SerialHub] 加载配置文件失败: %v", err)
		} else {
			cfg = loaded
		}
	}

	// 日志目录环境变量在配置加载后应用，保证覆盖配置文件中的 logDir。
	if logDir := os.Getenv("SERIALHUB_LOG_DIR"); logDir != "" {
		cfg.LogDir = logDir
	}

	if serialPort != "" {
		cfg.Serial.Port = serialPort
	}
	if baudRate != 115200 {
		cfg.Serial.BaudRate = baudRate
	}
	if host != "" && host != "127.0.0.1" {
		cfg.Host = host
	}
	if mcpPort != config.DefaultHTTPPort && mcpPort != 0 {
		cfg.MCP.HTTPPort = mcpPort
	}
	// --log-data 与 SERIALHUB_LOG_DATA 仅决定本次运行是否输出数据内容日志，
	// 不回写配置文件（cfg.LogData 保持文件原值）：显式 flag（含
	// --log-data=false 显式关闭）> 环境变量 > 配置文件 logData。
	logDataEffective = cfg.LogData
	if v, ok := os.LookupEnv("SERIALHUB_LOG_DATA"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			logDataEffective = b
		} else {
			logrus.Warnf("[SerialHub] SERIALHUB_LOG_DATA 值无效（%q），已忽略", v)
		}
	}
	if logDataFlag != nil && logDataFlag.Changed {
		logDataEffective = logDataMode
	}

	if configPath != "" {
		if err := config.Save(configPath, cfg); err != nil {
			logrus.Warnf("[SerialHub] 保存配置失败: %v", err)
		}
	}

	// 回填全局变量：serve 流程（联邦发现、HTTP 监听、反代）统一使用
	// 最终生效的端口与地址——否则配置文件指定的端口不会传导，
	// 发现与监听仍停留在 flag 默认值（如 5050）。
	host = cfg.Host
	mcpPort = cfg.MCP.HTTPPort

	return cfg
}

// resolveConfigPath 决定配置文件路径，包级 configPath 作为显式 -c 参数：
//   - Windows：-c > exe 同目录（保持原有行为，不迁移）。
//   - 非 Windows：-c > ./config.toml（CWD，用而不迁）>
//     用户配置目录 serialhub/config.toml；exe 同目录存在旧配置且用户配置
//     目录无 → 迁移（移动）到用户配置目录；三处皆无 → 返回用户配置目录
//     路径（首次由 loadConfig 的 Save 落盘）。
func resolveConfigPath() string {
	var exeDir string
	if exePath, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exePath)
	}
	cwd, _ := os.Getwd()
	resolved := resolveConfigPathImpl(configPath, cwd, exeDir, xdgConfigDir(), runtime.GOOS == "windows")
	// 自动选择的路径（非显式 -c）确保父目录存在，首次 Save 才能落盘；
	// 显式 -c 保持原有语义（父目录缺失时 Save 自然报错）。
	if resolved != "" && resolved != configPath {
		if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
			logrus.Warnf("[SerialHub] 创建配置目录失败: %v", err)
		}
	}
	return resolved
}

// resolveConfigPathImpl 是 resolveConfigPath 的可测内核（参数注入代替
// 全局状态与环境），路径语义见 resolveConfigPath 注释。xdgBase 为用户
// 配置根目录（空表示不可用）。
func resolveConfigPathImpl(explicit, cwdDir, exeDir, xdgBase string, isWindows bool) string {
	if explicit != "" {
		return explicit
	}
	if isWindows {
		if exeDir == "" {
			return ""
		}
		return filepath.Join(exeDir, "config.toml")
	}
	// CWD 配置用而不迁；优先级高于用户配置目录，且不依赖 exe/XDG
	// 是否可得（即使两者均解析失败，CWD 配置仍应生效）。
	if cwdDir != "" {
		if cwdCfg := filepath.Join(cwdDir, "config.toml"); fileExists(cwdCfg) {
			return cwdCfg
		}
	}
	if xdgBase != "" {
		xdgCfg := filepath.Join(xdgBase, "serialhub", "config.toml")
		if fileExists(xdgCfg) {
			return xdgCfg
		}
		if exeDir != "" {
			if legacy := filepath.Join(exeDir, "config.toml"); fileExists(legacy) {
				err := migrateConfigFile(legacy, xdgCfg)
				switch {
				case err == nil:
					logrus.Infof("[SerialHub] 已迁移配置文件到用户配置目录: %s → %s", legacy, xdgCfg)
					return xdgCfg
				case errors.Is(err, errLegacyRemoveFailed):
					logrus.Warnf("[SerialHub] 配置已迁移到 %s，但旧文件删除失败，请手动删除: %s", xdgCfg, legacy)
					return xdgCfg
				default:
					logrus.Warnf("[SerialHub] 配置文件迁移失败（%v），沿用旧位置: %s", err, legacy)
					return legacy
				}
			}
		}
		return xdgCfg
	}
	// 拿不到用户配置目录：退回 exe 同目录，保证有可用路径。
	if exeDir != "" {
		return filepath.Join(exeDir, "config.toml")
	}
	return ""
}

// xdgConfigDir 返回用户配置根目录，Linux/macOS 统一走 XDG 约定：
// 优先 $XDG_CONFIG_HOME（须为绝对路径，相对路径按规范忽略），否则
// $HOME/.config；均拿不到返回空。不复用 os.UserConfigDir（macOS 上
// 它返回 Application Support，与本项目「统一 ~/.config」的约定不符）。
func xdgConfigDir() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" && filepath.IsAbs(v) {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// errLegacyRemoveFailed 表示迁移已发布到新位置，但旧文件删除失败：
// 新副本已提交，应采用新位置并提示手动清理残留。
var errLegacyRemoveFailed = errors.New("旧配置文件删除失败")

// osLink 是 os.Link 的可注入包装，仅测试使用（模拟跨设备等失败）。
var osLink = os.Link

// migrateConfigFile 将旧配置迁移（移动语义）到新位置。
//
// 并发安全：发布一律走硬链接原子不覆盖——目标已被并发创建时
// link 返回 EEXIST，本轮采用既有目标、保留自己的源文件，绝不覆盖
// 他人内容；只有本次成功发布了自己的副本才删除源文件。迁移只在
// Linux/macOS 发生（Windows 分支直接用 exe 同目录），要求目标文件
// 系统支持硬链接（主流本地文件系统均支持）；跨设备或目标文件系统
// 不支持时迁移失败、沿用旧位置（非致命）。复制路径经目标目录内独有
// 临时文件（CreateTemp）原子发布，临时文件 best-effort 清理（清理
// 失败可能残留，不影响已发布目标的内容）。未做 fsync，掉电耐久性
// 不在保证范围（配置文件可接受）。
func migrateConfigFile(src, dst string) error {
	// 快路径（非原子，仅省去无谓工作；正确性由 link 原子发布保证）。
	if fileExists(dst) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}
	// 先尝试硬链接直接发布（同文件系统、原子、不覆盖目标）。
	switch err := osLink(src, dst); {
	case err == nil:
		if rmErr := os.Remove(src); rmErr != nil {
			return fmt.Errorf("%w: %v", errLegacyRemoveFailed, rmErr)
		}
		return nil
	case errors.Is(err, os.ErrExist):
		if fileExists(dst) {
			// 目标已被并发发布：采用既有目标，不动他人内容，也不删源。
			return nil
		}
		return fmt.Errorf("目标路径被占用: %s", dst)
	case errors.Is(err, os.ErrNotExist):
		// 源已消失：可能是另一进程刚完成迁移并删除源文件。
		if fileExists(dst) {
			return nil
		}
		return fmt.Errorf("源配置文件不存在: %w", err)
	}
	// link 失败（典型为跨设备）：复制到目标目录内独有临时文件后
	// 经 link 原子发布。
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("读取源配置失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".serialhub-migrate-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	published := false
	defer func() {
		if !published {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("写临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	switch err := osLink(tmpName, dst); {
	case err == nil:
		published = true
		// 链接已就位；移除临时文件失败仅残留临时文件，已发布内容
		// 不受影响，不因此回退旧位置。
		os.Remove(tmpName)
	case errors.Is(err, os.ErrExist):
		if !fileExists(dst) {
			return fmt.Errorf("目标路径被占用: %s", dst)
		}
		// 目标已被并发发布：采用既有目标（defer 清理临时文件），
		// 自己的源文件保留——只有成功发布自己的副本才允许删源。
		return nil
	default:
		return fmt.Errorf("发布迁移文件失败: %w", err)
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("%w: %v", errLegacyRemoveFailed, err)
	}
	return nil
}

func configToSerialConfig(cfg *config.SerialConfig) *serial.Config {
	return &serial.Config{
		Port:     cfg.Port,
		BaudRate: cfg.BaudRate,
		DataBits: cfg.DataBits,
		Parity:   cfg.Parity,
		StopBits: float32(cfg.StopBits),
	}
}
