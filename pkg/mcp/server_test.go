// Package mcp tests MCP server and transport implementations.
package mcp

import (
	"testing"

	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/mcp/tools"
	"github.com/yourname/serialhub/pkg/serial"
)

// TestNewMCPServer 测试创建 MCP 服务器
func TestNewMCPServer(t *testing.T) {
	// Create serial manager config
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"

	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("创建串口管理器失败: %v", err)
	}
	defer sm.Close()

	buf := buffer.NewDataBuffer()

	server, err := NewMCPServer(sm, buf, nil)
	if err != nil {
		t.Fatalf("创建 MCP 服务器失败: %v", err)
	}

	if server == nil {
		t.Fatal("MCP 服务器不应为 nil")
	}

	if server.serialManager != sm {
		t.Error("串口管理器不匹配")
	}

	if server.dataBuffer != buf {
		t.Error("数据缓冲区不匹配")
	}

	if server.mcpServer == nil {
		t.Error("mcpServer 不应为 nil")
	}
}

// TestNewMCPServerWithNilManager 测试 nil 串口管理器
func TestNewMCPServerWithNilManager(t *testing.T) {
	buf := buffer.NewDataBuffer()

	_, err := NewMCPServer(nil, buf, nil)
	if err == nil {
		t.Error("应该返回错误，但返回了 nil")
	}

	if err.Error() != "串口管理器不能为空" {
		t.Errorf("错误消息不正确: %v", err)
	}
}

// TestNewMCPServerWithNilBuffer 测试 nil 数据缓冲区（应创建默认缓冲区）
func TestNewMCPServerWithNilBuffer(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"

	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("创建串口管理器失败: %v", err)
	}
	defer sm.Close()

	server, err := NewMCPServer(sm, nil, nil)
	if err != nil {
		t.Fatalf("创建 MCP 服务器失败: %v", err)
	}

	if server.dataBuffer == nil {
		t.Error("数据缓冲区不应为 nil，应创建默认缓冲区")
	}
}

// TestMCPServerRegisterTools 测试注册工具
func TestMCPServerRegisterTools(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"

	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("创建串口管理器失败: %v", err)
	}
	defer sm.Close()

	buf := buffer.NewDataBuffer()
	server, err := NewMCPServer(sm, buf, nil)
	if err != nil {
		t.Fatalf("创建 MCP 服务器失败: %v", err)
	}

	// Register tools
	err = server.RegisterTools()
	if err != nil {
		t.Errorf("注册工具失败: %v", err)
	}
}

// TestMCPServerStop 测试停止 MCP 服务器
func TestMCPServerStop(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"

	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("创建串口管理器失败: %v", err)
	}
	defer sm.Close()

	buf := buffer.NewDataBuffer()
	server, err := NewMCPServer(sm, buf, nil)
	if err != nil {
		t.Fatalf("创建 MCP 服务器失败: %v", err)
	}

	// Stop should not panic
	err = server.Stop()
	if err != nil {
		t.Errorf("停止服务器失败: %v", err)
	}
}

// TestMCPServerToolResultToMCPResult 测试工具结果转换
func TestMCPServerToolResultToMCPResult(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"

	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("创建串口管理器失败: %v", err)
	}
	defer sm.Close()

	buf := buffer.NewDataBuffer()
	server, err := NewMCPServer(sm, buf, nil)
	if err != nil {
		t.Fatalf("创建 MCP 服务器失败: %v", err)
	}

	// Test successful result
	result, err := server.toolResultToMCPResult(tools.ToolResult{
		Success: true,
		Message: "测试成功",
		Data:    map[string]interface{}{"key": "value"},
	})
	if err != nil {
		t.Errorf("转换结果失败: %v", err)
	}
	if result == nil {
		t.Fatal("结果不应为 nil")
	}

	// Test error result
	result, err = server.toolResultToMCPResult(tools.ToolResult{
		Success: false,
		Message: "测试失败",
	})
	if err != nil {
		t.Errorf("转换结果失败: %v", err)
	}
	if result == nil {
		t.Fatal("结果不应为 nil")
	}
}
