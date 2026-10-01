package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/sirupsen/logrus"
)

type SerialConfig struct {
	Port     string
	BaudRate int
	DataBits int
	Parity   string
	StopBits float64
	// AutoConnect 启动时是否自动连接上次使用的串口端口（默认 true）。
	AutoConnect bool
}

type MCPConfig struct {
	HTTPPort int
}

// ScriptConfig 控制 MCP serial_script 工具允许的超时范围。
// 校验以配置值为准，调用方传入超范围的 timeoutMs 直接拒绝。
type ScriptConfig struct {
	TimeoutMinMs int // 允许的最小 timeoutMs（默认 100）
	TimeoutMaxMs int // 允许的最大 timeoutMs（默认 30min）
}

// DefaultHTTPPort 是 HTTP 服务的默认端口（MCP + WebSocket + Web 终端共用），
// 全仓库端口相关默认值均以此常量为唯一来源。
const DefaultHTTPPort = 5050

type Config struct {
	Serial SerialConfig
	MCP    MCPConfig
	Script ScriptConfig
	Host   string
	LogDir string
	Debug  bool
	// LogData 输出数据内容日志（串口/WebSocket 收发数据的 %q 内容，
	// 经 500ms 时间窗聚合、单条展示截断 512 字节）。此字段只承载配置文件
	// 的持久设置；--log-data 与 SERIALHUB_LOG_DATA 环境变量在运行时按
	// 「显式 flag > 环境变量 > 配置文件」合并生效，且不回写配置文件。
	LogData bool
}

// 脚本超时范围默认值（100ms～30min），配置文件缺省或非法时回退于此。
const (
	DefaultScriptTimeoutMinMs = 100
	DefaultScriptTimeoutMaxMs = 30 * 60 * 1000
)

func GetDefault() *Config {
	return &Config{
		Serial: SerialConfig{
			Port:        "",
			BaudRate:    115200,
			DataBits:    8,
			Parity:      "none",
			StopBits:    1,
			AutoConnect: true,
		},
		MCP: MCPConfig{
			HTTPPort: DefaultHTTPPort,
		},
		Script: ScriptConfig{
			TimeoutMinMs: DefaultScriptTimeoutMinMs,
			TimeoutMaxMs: DefaultScriptTimeoutMaxMs,
		},
		Host:    "127.0.0.1",
		Debug:   false,
		LogData: false,
	}
}

func Load(configPath string) (*Config, error) {
	cfg := GetDefault()

	if configPath == "" {
		logrus.Debugf("[SerialHub] 使用默认配置: %+v", cfg)
		return cfg, nil
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logrus.Warnf("[SerialHub] 配置文件不存在: %s", configPath)
		logrus.Debugf("[SerialHub] 使用默认配置: %+v", cfg)
		return cfg, nil
	}

	if _, err := toml.DecodeFile(configPath, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 脚本超时范围防御：配置文件只写一半或值非法（<=0）时回退默认，
	// 避免 min=0 放开下限、max=0 直接拒绝一切调用。
	if cfg.Script.TimeoutMinMs <= 0 {
		cfg.Script.TimeoutMinMs = DefaultScriptTimeoutMinMs
	}
	if cfg.Script.TimeoutMaxMs <= 0 {
		cfg.Script.TimeoutMaxMs = DefaultScriptTimeoutMaxMs
	}
	if cfg.Script.TimeoutMinMs > cfg.Script.TimeoutMaxMs {
		cfg.Script.TimeoutMinMs = DefaultScriptTimeoutMinMs
		cfg.Script.TimeoutMaxMs = DefaultScriptTimeoutMaxMs
	}

	logrus.Infof("[SerialHub] 已加载配置文件: %s", configPath)
	logrus.Debugf("[SerialHub] 配置内容: %+v", cfg)
	return cfg, nil
}

func Save(configPath string, cfg *Config) error {
	if configPath == "" {
		return fmt.Errorf("配置文件路径为空")
	}

	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("创建配置文件失败: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			logrus.Warnf("[SerialHub] 关闭配置文件失败: %v", cerr)
		}
	}()

	if err := toml.NewEncoder(file).Encode(cfg); err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}

	logrus.Debugf("[SerialHub] 已保存配置文件: %s", configPath)
	return nil
}
