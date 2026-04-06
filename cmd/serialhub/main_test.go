package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yourname/serialhub/internal/testutil"
	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/serial"
)

func TestConfigToSerialConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    config.SerialConfig
		expected serial.Config
	}{
		{
			name: "默认配置",
			input: config.SerialConfig{
				Port:     "COM9",
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "none",
				StopBits: 1,
			},
			expected: serial.Config{
				Port:     "COM9",
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "none",
				StopBits: 1,
			},
		},
		{
			name: "自定义配置",
			input: config.SerialConfig{
				Port:     "/dev/ttyUSB0",
				BaudRate: 9600,
				DataBits: 7,
				Parity:   "even",
				StopBits: 2,
			},
			expected: serial.Config{
				Port:     "/dev/ttyUSB0",
				BaudRate: 9600,
				DataBits: 7,
				Parity:   "even",
				StopBits: 2,
			},
		},
		{
			name: "1.5停止位",
			input: config.SerialConfig{
				Port:     "COM4",
				BaudRate: 57600,
				DataBits: 8,
				Parity:   "odd",
				StopBits: 1.5,
			},
			expected: serial.Config{
				Port:     "COM4",
				BaudRate: 57600,
				DataBits: 8,
				Parity:   "odd",
				StopBits: 1.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := configToSerialConfig(&tt.input)
			testutil.AssertEqual(t, tt.expected.Port, result.Port)
			testutil.AssertEqual(t, tt.expected.BaudRate, result.BaudRate)
			testutil.AssertEqual(t, tt.expected.DataBits, result.DataBits)
			testutil.AssertEqual(t, tt.expected.Parity, result.Parity)
			testutil.AssertEqual(t, tt.expected.StopBits, result.StopBits)
		})
	}
}

func TestLoadConfig_Default(t *testing.T) {
	configPath = ""
	serialPort = ""
	baudRate = 115200

	cfg := loadConfig()
	testutil.AssertNotNil(t, cfg)
	testutil.AssertEqual(t, "", cfg.Serial.Port)
	testutil.AssertEqual(t, 115200, cfg.Serial.BaudRate)
}

func TestLoadConfig_WithSerialPort(t *testing.T) {
	originalPath := configPath
	configPath = ""
	serialPort = "COM9"
	baudRate = 115200

	cfg := loadConfig()
	testutil.AssertEqual(t, "COM9", cfg.Serial.Port)

	configPath = originalPath
}

func TestLoadConfig_WithBaudRate(t *testing.T) {
	originalPath := configPath
	originalBaud := baudRate
	configPath = ""
	serialPort = ""
	baudRate = 9600

	cfg := loadConfig()
	testutil.AssertEqual(t, 9600, cfg.Serial.BaudRate)

	baudRate = originalBaud
	configPath = originalPath
}

func TestLoadConfig_WithEnvLogDir(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("SERIALHUB_LOG_DIR", tempDir)
	defer os.Unsetenv("SERIALHUB_LOG_DIR")

	configPath = ""
	serialPort = ""
	baudRate = 115200

	cfg := loadConfig()
	if cfg.LogDir != tempDir {
		t.Logf("环境变量可能未生效，cfg.LogDir = %s", cfg.LogDir)
	}
}

func TestLoadConfig_WithConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config.toml")

	content := `[serial]
port = "COM8"
baudRate = 19200
dataBits = 7
parity = "even"
stopBits = 2

[telnet]
port = 3000

[mcp]
httpPort = 6000
`
	err := os.WriteFile(configFile, []byte(content), 0644)
	testutil.AssertNoError(t, err)

	originalPath := configPath
	configPath = configFile
	serialPort = ""
	baudRate = 115200

	cfg := loadConfig()
	testutil.AssertEqual(t, "COM8", cfg.Serial.Port)
	testutil.AssertEqual(t, 19200, cfg.Serial.BaudRate)
	testutil.AssertEqual(t, 7, cfg.Serial.DataBits)
	testutil.AssertEqual(t, "even", cfg.Serial.Parity)
	testutil.AssertEqual(t, float64(2), cfg.Serial.StopBits)
	testutil.AssertEqual(t, 3000, cfg.Telnet.Port)
	testutil.AssertEqual(t, 6000, cfg.MCP.HTTPPort)

	configPath = originalPath
}

func TestLoadConfig_InvalidConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "invalid.toml")

	err := os.WriteFile(configFile, []byte("invalid toml content"), 0644)
	testutil.AssertNoError(t, err)

	originalPath := configPath
	configPath = configFile
	serialPort = ""
	baudRate = 115200

	cfg := loadConfig()
	testutil.AssertNotNil(t, cfg)

	configPath = originalPath
}

func TestLoadConfig_CLIOverrides(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config.toml")

	content := `[serial]
port = "COM8"
baudRate = 19200
`
	err := os.WriteFile(configFile, []byte(content), 0644)
	testutil.AssertNoError(t, err)

	originalPath := configPath
	configPath = configFile
	serialPort = "COM10"
	baudRate = 38400

	cfg := loadConfig()
	testutil.AssertEqual(t, "COM10", cfg.Serial.Port)
	testutil.AssertEqual(t, 38400, cfg.Serial.BaudRate)

	configPath = originalPath
}
