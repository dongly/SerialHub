// Package tools provides MCP tools for serial port operations.
package tools

import (
	"fmt"
	"sync"

	"github.com/dongly/serialhub/pkg/serial"
)

// ConnectInput represents input for serial_connect tool
type ConnectInput struct {
	Port     string `json:"port" validate:"required"`
	BaudRate int    `json:"baudRate,omitempty"`
}

// connectMu 串行化 serial_connect 的“检查-更新配置-连接”整个序列，
// 使同端口并发请求原子化：后到者进入时必然看到已建立的连接，
// 从而走幂等成功分支，而非在连接途中撞上“连接时不能更新配置”。
var connectMu sync.Mutex

// ExecuteSerialConnect connects to the specified serial port
func ExecuteSerialConnect(sm *serial.SerialManager, input ConnectInput) ToolResult {
	if sm == nil {
		return ToolResult{
			Success: false,
			Message: "串口管理器未初始化",
		}
	}

	connectMu.Lock()
	defer connectMu.Unlock()

	// 已有连接：同一端口视为幂等成功，不同端口仍拒绝（需先断开）
	if sm.IsConnected() {
		return existingConnectionResult(sm, input.Port)
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
	if err := sm.UpdateConfig(cfg); err != nil {
		return recheckResult(sm, input.Port, fmt.Sprintf("配置更新失败: %v", err))
	}

	// Connect
	if err := sm.Connect(); err != nil {
		return recheckResult(sm, input.Port, fmt.Sprintf("连接失败: %v", err))
	}

	return connectedResult(sm)
}

// connectedResult 构造“已连接”成功结果；数据取当前实际生效配置
// （幂等分支不改变连接参数，波特率以既有连接为准）。
func connectedResult(sm *serial.SerialManager) ToolResult {
	cfg := sm.GetConfig()
	return ToolResult{
		Success: true,
		Message: fmt.Sprintf("串口已连接: %s@%d", cfg.Port, cfg.BaudRate),
		Data: map[string]any{
			"port":     cfg.Port,
			"baudRate": cfg.BaudRate,
		},
	}
}

// existingConnectionResult 已有连接时的判定：请求端口与当前一致则幂等成功，
// 不一致则拒绝并提示先断开。
func existingConnectionResult(sm *serial.SerialManager, want string) ToolResult {
	current := sm.CurrentPort()
	if current == want {
		return connectedResult(sm)
	}
	return ToolResult{
		Success: false,
		Message: fmt.Sprintf("串口已连接: %s，请先断开后再连接其他端口", current),
	}
}

// recheckResult UpdateConfig/Connect 失败后的复核：连接可能在本次调用
// 期间被其他路径（托盘、命令处理）建立。若当前连接的正是请求端口，
// 仍按幂等成功返回；否则返回原失败原因（含已连接其他端口的提示）。
func recheckResult(sm *serial.SerialManager, want string, cause string) ToolResult {
	if sm.IsConnected() {
		return existingConnectionResult(sm, want)
	}
	return ToolResult{
		Success: false,
		Message: cause,
	}
}
