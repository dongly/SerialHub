// Package tools provides MCP tools for serial port operations.
package tools

import (
	"fmt"

	"github.com/yourname/serialhub/pkg/serial"
)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ExecuteSerialList lists all available serial ports
func ExecuteSerialList(sm *serial.SerialManager) ToolResult {
	ports, err := sm.ListPorts()
	if err != nil {
		return ToolResult{
			Success: false,
			Message: fmt.Sprintf("获取串口列表失败: %v", err),
		}
	}

	return ToolResult{
		Success: true,
		Message: fmt.Sprintf("找到 %d 个串口", len(ports)),
		Data:    ports,
	}
}
