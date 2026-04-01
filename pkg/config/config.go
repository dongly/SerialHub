package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type SerialConfig struct {
	Port     string
	BaudRate int
	DataBits int
	Parity   string
	StopBits int
}

type TelnetConfig struct {
	Port int
}

type MCPConfig struct {
	HTTPPort int
}

type Config struct {
	Serial SerialConfig
	Telnet TelnetConfig
	MCP    MCPConfig
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
		Telnet: TelnetConfig{
			Port: 2323,
		},
		MCP: MCPConfig{
			HTTPPort: 5000,
		},
		Debug: false,
	}
}

func Load(configPath string) (*Config, error) {
	cfg := GetDefault()

	if configPath == "" {
		logrus.Debugf("[SerialHub] ????: %+v", cfg)
		return cfg, nil
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logrus.Warnf("[SerialHub] ???????: %s", configPath)
		logrus.Debugf("[SerialHub] ????: %+v", cfg)
		return cfg, nil
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("json")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("????????: %w", err)
	}

	logrus.Infof("[SerialHub] ??????: %s", configPath)

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("??????: %w", err)
	}

	logrus.Debugf("[SerialHub] ????: %+v", cfg)
	return cfg, nil
}

func (c *Config) ToJSON() (string, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", fmt.Errorf("???????: %w", err)
	}
	return string(data), nil
}
