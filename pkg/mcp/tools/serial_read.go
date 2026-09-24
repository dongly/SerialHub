// Package tools provides MCP tools for serial port operations.
package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/dongly/serialhub/internal/buffer"
)

// ReadInput represents input for serial_read tool
// Timeout 为 nil 时表示未指定，默认超时 1000ms；显式传入 0 表示无限等待
type ReadInput struct {
	Timeout *int `json:"timeout,omitempty"` // 毫秒，nil=默认 1000ms，0=无限等待
	MaxSize int  `json:"maxSize,omitempty"` // 最大读取字节数
}

// ExecuteSerialRead reads data from the buffer with timeout.
// ctx 用于取消：客户端断开或取消调用时（如 timeout=0 无限等待期间），
// 读取立即中止，避免 goroutine 泄漏。
func ExecuteSerialRead(ctx context.Context, buf *buffer.DataBuffer, input ReadInput) ToolResult {
	// Set defaults
	maxSize := input.MaxSize
	if maxSize <= 0 || maxSize > 4096 {
		maxSize = 4096
	}

	// 默认超时 1000ms；仅当显式传入 timeout:0 时无限等待
	timeoutMs := 1000
	if input.Timeout != nil {
		timeoutMs = *input.Timeout
	}

	// Handle timeout
	if timeoutMs == 0 {
		// Infinite wait: poll until data available or context cancelled
		for buf.Length() == 0 {
			select {
			case <-ctx.Done():
				return ToolResult{
					Success: false,
					Message: "读取已取消",
				}
			case <-time.After(10 * time.Millisecond):
			}
		}
		data := buf.Read(maxSize)
		if data == nil {
			return ToolResult{
				Success: false,
				Message: "读取失败",
			}
		}
		return ToolResult{
			Success: true,
			Message: fmt.Sprintf("读取成功: %d 字节", len(data)),
			Data: map[string]any{
				"data":  string(data),
				"bytes": len(data),
			},
		}
	}

	// Timed wait
	timeout := time.Duration(timeoutMs) * time.Millisecond
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 客户端取消/断开
			return ToolResult{
				Success: false,
				Message: "读取已取消",
			}
		case <-timer.C:
			// Timeout
			data := buf.Read(maxSize)
			if data == nil {
				return ToolResult{
					Success: true,
					Message: "读取超时",
					Data: map[string]any{
						"data":     "",
						"bytes":    0,
						"timedOut": true,
					},
				}
			}
			return ToolResult{
				Success: true,
				Message: fmt.Sprintf("读取成功: %d 字节", len(data)),
				Data: map[string]any{
					"data":     string(data),
					"bytes":    len(data),
					"timedOut": false,
				},
			}
		case <-ticker.C:
			// Check if data available
			if buf.Length() > 0 {
				data := buf.Read(maxSize)
				if data == nil {
					return ToolResult{
						Success: false,
						Message: "读取失败",
					}
				}
				return ToolResult{
					Success: true,
					Message: fmt.Sprintf("读取成功: %d 字节", len(data)),
					Data: map[string]any{
						"data":     string(data),
						"bytes":    len(data),
						"timedOut": false,
					},
				}
			}
		}
	}
}
