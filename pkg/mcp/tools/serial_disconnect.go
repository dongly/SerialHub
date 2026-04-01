// Package tools provides MCP tools for serial port operations.
package tools

import (
	"fmt"

	"github.com/yourname/serialhub/pkg/serial"
)

// ExecuteSerialDisconnect disconnects from the serial port
func ExecuteSerialDisconnect(sm *serial.SerialManager) ToolResult {
	// Check if connected
	if !sm.IsConnected() {
		return ToolResult{
			Success: false,
			Message: "串口未连接",
		}
	}

	port := sm.CurrentPort()

	// Disconnect
	err := sm.Disconnect()
	if err != nil {
		return ToolResult{
			Success: false,
			Message: fmt.Sprintf("断开连接失败: %v", err),
		}
	}

	return ToolResult{
		Success: true,
		Message: fmt.Sprintf("串口已断开: %s", port),
		Data: map[string]any{
			"port": port,
		},
	}
}
