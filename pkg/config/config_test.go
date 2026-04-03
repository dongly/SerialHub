package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDefault(t *testing.T) {
	cfg := GetDefault()

	if cfg.Serial.BaudRate != 115200 {
		t.Errorf("default baud rate should be 115200, got %d", cfg.Serial.BaudRate)
	}
	if cfg.Serial.DataBits != 8 {
		t.Errorf("default data bits should be 8, got %d", cfg.Serial.DataBits)
	}
	if cfg.Serial.Parity != "none" {
		t.Errorf("default parity should be 'none', got '%s'", cfg.Serial.Parity)
	}
	if cfg.Serial.StopBits != 1 {
		t.Errorf("default stop bits should be 1, got %v", cfg.Serial.StopBits)
	}

	if cfg.Telnet.Port != 2323 {
		t.Errorf("default telnet port should be 2323, got %d", cfg.Telnet.Port)
	}

	if cfg.MCP.HTTPPort != 5000 {
		t.Errorf("default mcp http port should be 5000, got %d", cfg.MCP.HTTPPort)
	}

	if cfg.Debug != false {
		t.Errorf("default debug should be false, got %t", cfg.Debug)
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	tomlData := `
logDir = ""
debug = true

[serial]
port = "COM9"
baudRate = 9600
dataBits = 8
parity = "none"
stopBits = 1

[telnet]
port = 2324

[mcp]
httpPort = 5001
`
	if err := os.WriteFile(configPath, []byte(tomlData), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Serial.Port != "COM9" {
		t.Errorf("expected port 'COM9', got '%s'", cfg.Serial.Port)
	}
	if cfg.Serial.BaudRate != 9600 {
		t.Errorf("expected baud rate 9600, got %d", cfg.Serial.BaudRate)
	}
	if cfg.Telnet.Port != 2324 {
		t.Errorf("expected telnet port 2324, got %d", cfg.Telnet.Port)
	}
	if cfg.MCP.HTTPPort != 5001 {
		t.Errorf("expected mcp http port 5001, got %d", cfg.MCP.HTTPPort)
	}
	if cfg.Debug != true {
		t.Errorf("expected debug true, got %t", cfg.Debug)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "nonexistent.toml")
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("loading nonexistent config should return default config, got error: %v", err)
	}
	if cfg.Serial.BaudRate != 115200 {
		t.Errorf("expected default baud rate 115200, got %d", cfg.Serial.BaudRate)
	}
	if cfg.Telnet.Port != 2323 {
		t.Errorf("expected default telnet port 2323, got %d", cfg.Telnet.Port)
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.toml")
	invalidTOML := `[serial
port = "missing bracket"`
	if err := os.WriteFile(configPath, []byte(invalidTOML), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}
	_, err := Load(configPath)
	if err == nil {
		t.Error("loading invalid TOML should return error")
	}
}

func TestConfigMerge(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial.toml")
	tomlData := `
[serial]
port = "COM8"
baudRate = 57600

[mcp]
httpPort = 6000
`
	if err := os.WriteFile(configPath, []byte(tomlData), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.Serial.Port != "COM8" {
		t.Errorf("expected port 'COM8', got '%s'", cfg.Serial.Port)
	}
	if cfg.Serial.BaudRate != 57600 {
		t.Errorf("expected baud rate 57600, got %d", cfg.Serial.BaudRate)
	}
	if cfg.MCP.HTTPPort != 6000 {
		t.Errorf("expected mcp http port 6000, got %d", cfg.MCP.HTTPPort)
	}
	if cfg.Serial.DataBits != 8 {
		t.Errorf("unconfigured DataBits should use default 8, got %d", cfg.Serial.DataBits)
	}
	if cfg.Serial.Parity != "none" {
		t.Errorf("unconfigured Parity should use default 'none', got '%s'", cfg.Serial.Parity)
	}
	if cfg.Telnet.Port != 2323 {
		t.Errorf("unconfigured telnet port should use default 2323, got %d", cfg.Telnet.Port)
	}
}

func TestConfig_EmptyPath(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Errorf("empty path should not return error: %v", err)
	}
	if cfg == nil {
		t.Fatal("empty path should return default config, not nil")
	}
	defaultCfg := GetDefault()
	if cfg.Serial.BaudRate != defaultCfg.Serial.BaudRate {
		t.Errorf("empty path should use default config")
	}
}

func TestLoadConfig_TOMLWithComments(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	tomlData := `# SerialHub 配置文件

# 日志目录，为空则保存到可执行文件目录下的 logs/
logDir = "D:/Logs"
debug = true

# 串口配置
[serial]
port = "COM3"      # Windows 串口
baudRate = 9600
dataBits = 8       # 数据位：5/6/7/8
parity = "none"    # 校验位：none/even/odd
stopBits = 1

# 网络服务
[telnet]
port = 3333

[mcp]
httpPort = 6000
`
	if err := os.WriteFile(configPath, []byte(tomlData), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load toml config: %v", err)
	}
	if cfg.Serial.Port != "COM3" {
		t.Errorf("expected port 'COM3', got '%s'", cfg.Serial.Port)
	}
	if cfg.Serial.BaudRate != 9600 {
		t.Errorf("expected baud rate 9600, got %d", cfg.Serial.BaudRate)
	}
	if cfg.Telnet.Port != 3333 {
		t.Errorf("expected telnet port 3333, got %d", cfg.Telnet.Port)
	}
	if cfg.MCP.HTTPPort != 6000 {
		t.Errorf("expected mcp http port 6000, got %d", cfg.MCP.HTTPPort)
	}
	if cfg.LogDir != "D:/Logs" {
		t.Errorf("expected logDir 'D:/Logs', got '%s'", cfg.LogDir)
	}
	if cfg.Debug != true {
		t.Errorf("expected debug true, got %t", cfg.Debug)
	}
}

func TestLoadConfig_LogDir(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	tomlData := `
logDir = "C:/MyLogs"
`
	if err := os.WriteFile(configPath, []byte(tomlData), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.LogDir != "C:/MyLogs" {
		t.Errorf("expected logDir 'C:/MyLogs', got '%s'", cfg.LogDir)
	}
	if cfg.Serial.BaudRate != 115200 {
		t.Errorf("other fields should use defaults, baudRate got %d", cfg.Serial.BaudRate)
	}
}
