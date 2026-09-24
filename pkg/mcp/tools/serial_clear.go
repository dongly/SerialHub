// Package tools provides MCP tools for serial port operations.
package tools

import (
	"fmt"

	"github.com/dongly/serialhub/internal/buffer"
)

// ExecuteSerialClear 清空 read 缓冲区，丢弃尚未被 serial_read 读取的数据
func ExecuteSerialClear(buf *buffer.DataBuffer) ToolResult {
	if buf == nil {
		return ToolResult{
			Success: false,
			Message: "数据缓冲区未初始化",
		}
	}

	cleared := buf.Length()
	buf.Clear()

	return ToolResult{
		Success: true,
		Message: fmt.Sprintf("缓冲区已清空: %d 字节", cleared),
		Data: map[string]any{
			"clearedBytes": cleared,
			"bufferLength": 0,
		},
	}
}
