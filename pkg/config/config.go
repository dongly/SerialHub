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
}

type MCPConfig struct {
	HTTPPort int
}

// DefaultHTTPPort 是 HTTP 服务的默认端口（MCP + WebSocket + Web 终端共用），
// 全仓库端口相关默认值均以此常量为唯一来源。
const DefaultHTTPPort = 5050

type Config struct {
	Serial SerialConfig
	MCP    MCPConfig
	Host   string
	LogDir string
	Debug  bool
	// LogData 输出数据内容日志（串口/WebSocket 收发数据的 %q 内容，
	// 经 500ms 时间窗聚合、单条展示截断 512 字节）。此字段只承载配置文件
	// 的持久设置；--log-data 与 SERIALHUB_LOG_DATA 环境变量在运行时按
	// 「显式 flag > 环境变量 > 配置文件」合并生效，且不回写配置文件。
	LogData bool
}

func GetDefault() *Config {
	return &Config{
		Serial: SerialConfig{
			Port:     "",
			BaudRate: 115200,
			DataBits: 8,
			Parity:   "none",
			StopBits: 1,
		},
		MCP: MCPConfig{
			HTTPPort: DefaultHTTPPort,
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
