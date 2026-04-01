package config

import (
	"encoding/json"
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
		t.Errorf("default stop bits should be 1, got %d", cfg.Serial.StopBits)
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
	configPath := filepath.Join(tmpDir, "config.json")
	fullConfig := GetDefault()
	fullConfig.Serial.Port = "COM9"
	fullConfig.Serial.BaudRate = 9600
	fullConfig.Telnet.Port = 2324
	fullConfig.MCP.HTTPPort = 5001
	fullConfig.Debug = true
	jsonData, _ := fullConfig.ToJSON()
	if err := os.WriteFile(configPath, []byte(jsonData), 0644); err != nil {
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
	configPath := filepath.Join(t.TempDir(), "nonexistent.json")
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

func TestLoadConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")
	invalidJSON := "not valid json"
	if err := os.WriteFile(configPath, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}
	_, err := Load(configPath)
	if err == nil {
		t.Error("loading invalid JSON should return error")
	}
}

func TestConfigMerge(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial.json")
	partialConfig := GetDefault()
	partialConfig.Serial.Port = "COM8"
	partialConfig.Serial.BaudRate = 57600
	partialConfig.MCP.HTTPPort = 6000
	jsonData, _ := partialConfig.ToJSON()
	if err := os.WriteFile(configPath, []byte(jsonData), 0644); err != nil {
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

func TestConfig_ToJSON(t *testing.T) {
	cfg := GetDefault()
	cfg.Serial.Port = "COM9"
	jsonStr, err := cfg.ToJSON()
	if err != nil {
		t.Errorf("config serialization failed: %v", err)
	}
	if jsonStr == "" {
		t.Error("config serialization result should not be empty")
	}
	parsed := &Config{}
	if err := json.Unmarshal([]byte(jsonStr), parsed); err != nil {
		t.Errorf("serialized result cannot be deserialized: %v", err)
	}
	if parsed.Serial.Port != "COM9" {
		t.Errorf("data inconsistency after serialization/deserialization")
	}
}
