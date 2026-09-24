package main

import (
	"os"
	"path/filepath"

	"github.com/dongly/serialhub/pkg/config"
	"github.com/dongly/serialhub/pkg/serial"
	"github.com/sirupsen/logrus"
)

func loadConfig() *config.Config {
	cfg := config.GetDefault()

	if logDir := os.Getenv("SERIALHUB_LOG_DIR"); logDir != "" {
		cfg.LogDir = logDir
	}

	if configPath == "" {
		exePath, err := os.Executable()
		if err == nil {
			configPath = filepath.Join(filepath.Dir(exePath), "config.toml")
		}
	}

	if configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			logrus.Warnf("[SerialHub] 加载配置文件失败: %v", err)
		} else {
			cfg = loaded
		}
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
	if mcpPort != 5000 && mcpPort != 0 {
		cfg.MCP.HTTPPort = mcpPort
	}

	if configPath != "" {
		if err := config.Save(configPath, cfg); err != nil {
			logrus.Warnf("[SerialHub] 保存配置失败: %v", err)
		}
	}

	return cfg
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
