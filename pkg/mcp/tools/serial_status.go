// Package tools provides MCP tools for serial port operations.
package tools

import (
	"fmt"

	"github.com/dongly/serialhub/pkg/serial"
)

// ExecuteSerialStatus gets the current serial port connection status
func ExecuteSerialStatus(sm *serial.SerialManager) ToolResult {
	if sm == nil || !sm.IsConnected() {
		return ToolResult{
			Success: true,
			Message: "串口未连接",
			Data: map[string]any{
				"connected": false,
			},
		}
	}

	cfg := sm.GetConfig()

	return ToolResult{
		Success: true,
		Message: fmt.Sprintf("串口已连接: %s", cfg.String()),
		Data: map[string]any{
			"connected": true,
			"port":      cfg.Port,
			"baudRate":  cfg.BaudRate,
			"dataBits":  cfg.DataBits,
			"parity":    cfg.Parity,
			"stopBits":  cfg.StopBits,
		},
	}
}
