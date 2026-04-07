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
port = "COM3" # Windows 串口
baudRate = 9600
dataBits = 8 # 数据位：5/6/7/8
parity = "none" # 校验位：none/even/odd
stopBits = 1

# 网络服务
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

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	cfg := GetDefault()
	cfg.Serial.Port = "COM9"
	cfg.Serial.BaudRate = 9600
	cfg.Serial.DataBits = 7
	cfg.Serial.Parity = "even"
	cfg.Serial.StopBits = 2
	cfg.MCP.HTTPPort = 5001
	cfg.LogDir = "D:/Logs"
	cfg.Debug = true

	err := Save(configPath, cfg)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loaded.Serial.Port != "COM9" {
		t.Errorf("expected port 'COM9', got '%s'", loaded.Serial.Port)
	}
	if loaded.Serial.BaudRate != 9600 {
		t.Errorf("expected baud rate 9600, got %d", loaded.Serial.BaudRate)
	}
	if loaded.Serial.DataBits != 7 {
		t.Errorf("expected data bits 7, got %d", loaded.Serial.DataBits)
	}
	if loaded.Serial.Parity != "even" {
		t.Errorf("expected parity 'even', got '%s'", loaded.Serial.Parity)
	}
	if loaded.Serial.StopBits != 2 {
		t.Errorf("expected stop bits 2, got %v", loaded.Serial.StopBits)
	}
	if loaded.MCP.HTTPPort != 5001 {
		t.Errorf("expected mcp http port 5001, got %d", loaded.MCP.HTTPPort)
	}
	if loaded.LogDir != "D:/Logs" {
		t.Errorf("expected logDir 'D:/Logs', got '%s'", loaded.LogDir)
	}
	if loaded.Debug != true {
		t.Errorf("expected debug true, got %t", loaded.Debug)
	}
}

func TestSaveConfig_EmptyPath(t *testing.T) {
	cfg := GetDefault()
	err := Save("", cfg)
	if err == nil {
		t.Error("saving with empty path should return error")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "roundtrip.toml")
	cfg := GetDefault()
	cfg.Serial.Port = "COM5"
	cfg.Serial.BaudRate = 19200

	if err := Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Serial.Port != "COM5" {
		t.Errorf("expected port 'COM5', got '%s'", loaded.Serial.Port)
	}
	if loaded.Serial.BaudRate != 19200 {
		t.Errorf("expected baud rate 19200, got %d", loaded.Serial.BaudRate)
	}
	if loaded.Serial.DataBits != 8 {
		t.Errorf("expected default data bits 8, got %d", loaded.Serial.DataBits)
	}
}

func TestSaveConfig_PreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "existing.toml")
	originalContent := `# Original comment
[serial]
port = "COM1"
`
	if err := os.WriteFile(configPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create original file: %v", err)
	}

	cfg := GetDefault()
	cfg.Serial.Port = "COM9"
	cfg.Serial.BaudRate = 38400

	if err := Save(configPath, cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Serial.Port != "COM9" {
		t.Errorf("expected port 'COM9', got '%s'", loaded.Serial.Port)
	}
	if loaded.Serial.BaudRate != 38400 {
		t.Errorf("expected baud rate 38400, got %d", loaded.Serial.BaudRate)
	}
}
