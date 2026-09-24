// Package tools provides MCP tools for serial port operations.
package tools

import (
	"fmt"

	"github.com/dongly/serialhub/pkg/serial"
)

// ConnectInput represents input for serial_connect tool
type ConnectInput struct {
	Port     string `json:"port" validate:"required"`
	BaudRate int    `json:"baudRate,omitempty"`
}

// ExecuteSerialConnect connects to the specified serial port
func ExecuteSerialConnect(sm *serial.SerialManager, input ConnectInput) ToolResult {
	if sm == nil {
		return ToolResult{
			Success: false,
			Message: "串口管理器未初始化",
		}
	}

	// Check if already connected
	if sm.IsConnected() {
		return ToolResult{
			Success: false,
			Message: fmt.Sprintf("串口已连接: %s", sm.CurrentPort()),
		}
	}

	// Set baud rate (default 115200)
	baudRate := input.BaudRate
	if baudRate <= 0 {
		baudRate = 115200
	}

	// Create config
	cfg := serial.DefaultConfig()
	cfg.Port = input.Port
	cfg.BaudRate = baudRate

	// Update config
	err := sm.UpdateConfig(cfg)
	if err != nil {
		return ToolResult{
			Success: false,
			Message: fmt.Sprintf("配置更新失败: %v", err),
		}
	}

	// Connect
	err = sm.Connect()
	if err != nil {
		return ToolResult{
			Success: false,
			Message: fmt.Sprintf("连接失败: %v", err),
		}
	}

	return ToolResult{
		Success: true,
		Message: fmt.Sprintf("串口已连接: %s@%d", input.Port, baudRate),
		Data: map[string]any{
			"port":     input.Port,
			"baudRate": baudRate,
		},
	}
}
