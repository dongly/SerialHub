// Package serial manages serial port connections.
package serial

import (
	"fmt"
	"strconv"
	"strings"

	serial "go.bug.st/serial"
)

// Config represents serial port connection configuration
type Config struct {
	// Port is the serial port name, e.g., "COM9" or "/dev/ttyUSB0"
	Port string

	// BaudRate is the baud rate, default 115200
	BaudRate int

	// DataBits is the number of data bits, default 8 (5/6/7/8)
	DataBits int

	// Parity is the parity bit, default "none" (none/even/odd)
	Parity string

	// StopBits is the number of stop bits, default 1 (1/2)
	StopBits float32
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   "none",
		StopBits: 1,
	}
}

// ToMode converts configuration to go.bug.st/serial.Mode
func (c *Config) ToMode() (*serial.Mode, error) {
	if c.BaudRate <= 0 {
		return nil, fmt.Errorf("无效的波特率: %d", c.BaudRate)
	}

	if c.DataBits < 5 || c.DataBits > 8 {
		return nil, fmt.Errorf("无效的数据位: %d（支持 5/6/7/8）", c.DataBits)
	}

	var parity serial.Parity
	switch c.Parity {
	case "none":
		parity = serial.NoParity
	case "even":
		parity = serial.EvenParity
	case "odd":
		parity = serial.OddParity
	default:
		return nil, fmt.Errorf("无效的校验位: %s（支持 none/even/odd）", c.Parity)
	}

	var stopBits serial.StopBits
	switch c.StopBits {
	case 1:
		stopBits = serial.OneStopBit
	case 2:
		stopBits = serial.TwoStopBits
	default:
		return nil, fmt.Errorf("无效的停止位: %v（支持 1/2）", c.StopBits)
	}

	return &serial.Mode{
		BaudRate: c.BaudRate,
		DataBits: c.DataBits,
		Parity:   parity,
		StopBits: stopBits,
	}, nil
}

// String returns the string representation of the configuration
func (c *Config) String() string {
	return fmt.Sprintf("%s@%d %d%s%d",
		c.Port,
		c.BaudRate,
		c.DataBits,
		strings.ToUpper(c.Parity[0:1]),
		int(c.StopBits),
	)
}

// Clone returns a deep copy of the configuration
func (c *Config) Clone() *Config {
	return &Config{
		Port:     c.Port,
		BaudRate: c.BaudRate,
		DataBits: c.DataBits,
		Parity:   c.Parity,
		StopBits: c.StopBits,
	}
}

// ParsePort parses port configuration from string, format "COM9" or "COM9@115200"
func ParsePort(portStr string) (*Config, error) {
	cfg := DefaultConfig()
	
	if portStr == "" {
		return nil, fmt.Errorf("端口字符串不能为空")
	}

	// Check if baud rate is included
	for i := 0; i < len(portStr); i++ {
		if portStr[i] == '@' {
			cfg.Port = portStr[:i]
			baudStr := portStr[i+1:]
			baud, err := strconv.Atoi(baudStr)
			if err != nil {
				return nil, fmt.Errorf("无效的波特率: %s", baudStr)
			}
			cfg.BaudRate = baud
			return cfg, nil
		}
	}

	// No baud rate, use default value
	cfg.Port = portStr
	return cfg, nil
}