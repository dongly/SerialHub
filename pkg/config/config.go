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

type Config struct {
	Serial SerialConfig
	MCP    MCPConfig
	Host   string
	LogDir string
	Debug  bool
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
			HTTPPort: 5000,
		},
		Host:  "127.0.0.1",
		Debug: false,
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
	defer file.Close()

	if err := toml.NewEncoder(file).Encode(cfg); err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}

	logrus.Debugf("[SerialHub] 已保存配置文件: %s", configPath)
	return nil
}
