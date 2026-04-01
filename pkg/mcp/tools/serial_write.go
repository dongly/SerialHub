// Package tools provides MCP tools for serial port operations.
package tools

import (
	"fmt"

	"github.com/yourname/serialhub/pkg/serial"
)

// WriteInput represents input for serial_write tool
type WriteInput struct {
	Data       string `json:"data" validate:"required"`
	AddNewline bool   `json:"addNewline,omitempty"`
}

// ExecuteSerialWrite writes data to the serial port
func ExecuteSerialWrite(sm *serial.SerialManager, input WriteInput) ToolResult {
	// Check if connected
	if !sm.IsConnected() {
		return ToolResult{
			Success: false,
			Message: "串口未连接",
		}
	}

	var data []byte
	var bytesWritten int
	var err error

	// Add newline if requested (default true)
	if input.AddNewline || input.Data == "" {
		data = []byte(input.Data + "\n")
		err = sm.WriteLine(input.Data)
	} else {
		data = []byte(input.Data)
		bytesWritten, err = sm.Write(data)
	}

	if err != nil {
		return ToolResult{
			Success: false,
			Message: fmt.Sprintf("写入失败: %v", err),
		}
	}

	// WriteLine doesn't return bytes written, use len(data)
	if input.AddNewline || input.Data == "" {
		bytesWritten = len(data)
	}

	return ToolResult{
		Success: true,
		Message: fmt.Sprintf("写入成功: %d 字节", bytesWritten),
		Data: map[string]any{
			"bytesWritten": bytesWritten,
			"data":         string(data),
		},
	}
}
