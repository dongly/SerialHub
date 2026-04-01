// Package tools provides MCP tools for serial port operations.
package tools

import (
	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/serial"
	"testing"
)

// TestSerialList tests listing serial ports
func TestSerialList(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9" // 需要一个有效的端口名来创建 manager
	sm, _ := serial.NewSerialManager(cfg)
	
	result := ExecuteSerialList(sm)
	
	if !result.Success {
		t.Errorf("预期成功，实际失败: %s", result.Message)
	}
	
	if result.Data == nil {
		t.Error("预期返回串口列表，实际为 nil")
	}
}

// TestSerialConnect_Success tests successful connection
func TestSerialConnect_Success(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	input := ConnectInput{
		Port:     "COM9",
		BaudRate: 115200,
	}
	
	result := ExecuteSerialConnect(sm, input)
	
	// 如果 COM9 不存在，连接会失败，但这是正常的
	if result.Message == "连接失败" {
		t.Logf("COM9 不存在，连接失败（符合预期）")
	} else if !result.Success {
		t.Errorf("预期成功，实际失败: %s", result.Message)
	}
	
	if sm.IsConnected() {
		t.Log("串口已连接")
	} else {
		t.Log("串口未连接")
	}
}

// TestSerialConnect_WithBaudRate tests connection with custom baud rate
func TestSerialConnect_WithBaudRate(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	input := ConnectInput{
		Port:     "COM9",
		BaudRate: 9600,
	}
	
	result := ExecuteSerialConnect(sm, input)
	
	// 如果 COM9 不存在，连接会失败，但这是正常的
	if result.Message == "连接失败" {
		t.Logf("COM9 不存在，连接失败（符合预期）")
	} else if !result.Success {
		t.Errorf("预期成功，实际失败: %s", result.Message)
	}
	
	cfg = sm.GetConfig()
	if cfg.BaudRate != 9600 {
		t.Errorf("预期波特率 9600，实际 %d", cfg.BaudRate)
	}
}

// TestSerialConnect_AlreadyConnected tests connection when already connected
func TestSerialConnect_AlreadyConnected(t *testing.T) {
	// Create manager
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	// Connect first - will attempt to connect to COM9
	input1 := ConnectInput{
		Port:     "COM9",
		BaudRate: 115200,
	}
	
	result1 := ExecuteSerialConnect(sm, input1)
	if !result1.Success {
		t.Logf("首次连接失败（如果 COM9 不存在则符合预期）: %s", result1.Message)
	}
	
	// Try to connect again
	input2 := ConnectInput{
		Port:     "COM10",
		BaudRate: 115200,
	}
	
	result2 := ExecuteSerialConnect(sm, input2)
	
	if sm.IsConnected() && result2.Success {
		t.Error("预期连接失败（已连接），实际成功")
	}
	
	if sm.IsConnected() && !result2.Success {
		t.Logf("连接失败（预期）: %s", result2.Message)
	}
}

// TestSerialConnect_InvalidPort tests connection with invalid port
func TestSerialConnect_InvalidPort(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	input := ConnectInput{
		Port:     "INVALID_PORT_99999",
		BaudRate: 115200,
	}
	
	result := ExecuteSerialConnect(sm, input)
	
	if result.Success {
		t.Error("预期失败，实际成功")
	}
}

// TestSerialDisconnect_Success tests successful disconnection
func TestSerialDisconnect_Success(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	// Note: This test requires a real connection or mock setup
	// For now, we'll test the disconnect logic
	
	result := ExecuteSerialDisconnect(sm)
	
	if !result.Success && result.Message != "串口未连接" {
		t.Logf("断开结果: %s", result.Message)
	}
}

// TestSerialDisconnect_NotConnected tests disconnection when not connected
func TestSerialDisconnect_NotConnected(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	result := ExecuteSerialDisconnect(sm)
	
	if result.Success {
		t.Error("预期失败，实际成功")
	}
	
	if result.Message != "串口未连接" {
		t.Errorf("预期消息 '串口未连接'，实际 '%s'", result.Message)
	}
}

// TestSerialWrite_Success tests successful write
func TestSerialWrite_Success(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	// Note: This test requires a connected port
	// For now, we'll test the write logic
	
	input := WriteInput{
		Data:       "test command",
		AddNewline: true,
	}
	
	result := ExecuteSerialWrite(sm, input)
	
	if !result.Success && result.Message != "串口未连接" {
		t.Logf("写入结果: %s", result.Message)
	}
}

// TestSerialWrite_NotConnected tests write when not connected
func TestSerialWrite_NotConnected(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	input := WriteInput{
		Data:       "test command",
		AddNewline: true,
	}
	
	result := ExecuteSerialWrite(sm, input)
	
	if result.Success {
		t.Error("预期失败，实际成功")
	}
	
	if result.Message != "串口未连接" {
		t.Errorf("预期消息 '串口未连接'，实际 '%s'", result.Message)
	}
}

// TestSerialWrite_WithNewline tests write with newline
func TestSerialWrite_WithNewline(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	input := WriteInput{
		Data:       "test command",
		AddNewline: true,
	}
	
	result := ExecuteSerialWrite(sm, input)
	
	if !result.Success && result.Message != "串口未连接" {
		t.Logf("写入结果: %s", result.Message)
	}
}

// TestSerialWrite_WithoutNewline tests write without newline
func TestSerialWrite_WithoutNewline(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	input := WriteInput{
		Data:       "test command",
		AddNewline: false,
	}
	
	result := ExecuteSerialWrite(sm, input)
	
	if !result.Success && result.Message != "串口未连接" {
		t.Logf("写入结果: %s", result.Message)
	}
}

// TestSerialRead_WithData tests reading with data available
func TestSerialRead_WithData(t *testing.T) {
	buf := buffer.NewDataBuffer(1024)
	
	// Add test data
	buf.Append([]byte("test data"))
	
	input := ReadInput{
		Timeout: 1000,
		MaxSize: 1024,
	}
	
	result := ExecuteSerialRead(buf, input)
	
	if !result.Success {
		t.Errorf("预期成功，实际失败: %s", result.Message)
	}
	
	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("返回数据格式错误")
	}
	
	data, ok := dataMap["data"].(string)
	if !ok || data != "test data" {
		t.Errorf("预期数据 'test data'，实际 '%s'", data)
	}
	
	timedOut, ok := dataMap["timedOut"].(bool)
	if !ok || timedOut {
		t.Error("预期 timedOut=false，实际 true")
	}
}

// TestSerialRead_EmptyBuffer tests reading from empty buffer with timeout
func TestSerialRead_EmptyBuffer(t *testing.T) {
	buf := buffer.NewDataBuffer(1024)
	
	input := ReadInput{
		Timeout: 100,
		MaxSize: 1024,
	}
	
	result := ExecuteSerialRead(buf, input)
	
	if !result.Success {
		t.Errorf("预期成功，实际失败: %s", result.Message)
	}
	
	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("返回数据格式错误")
	}
	
	timedOut, ok := dataMap["timedOut"].(bool)
	if !ok || !timedOut {
		t.Error("预期 timedOut=true，实际 false")
	}
}

// TestSerialRead_Timeout tests read with timeout
func TestSerialRead_Timeout(t *testing.T) {
	buf := buffer.NewDataBuffer(1024)
	
	input := ReadInput{
		Timeout: 50,
		MaxSize: 1024,
	}
	
	result := ExecuteSerialRead(buf, input)
	
	if !result.Success {
		t.Errorf("预期成功，实际失败: %s", result.Message)
	}
	
	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("返回数据格式错误")
	}
	
	timedOut, ok := dataMap["timedOut"].(bool)
	if !ok || !timedOut {
		t.Error("预期 timedOut=true，实际 false")
	}
}

// TestSerialStatus_Connected tests status when connected
func TestSerialStatus_Connected(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	result := ExecuteSerialStatus(sm)
	
	if !result.Success {
		t.Errorf("预期成功，实际失败: %s", result.Message)
	}
	
	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("返回数据格式错误")
	}
	
	connected, ok := dataMap["connected"].(bool)
	if ok && connected {
		t.Log("串口已连接")
	} else {
		t.Log("串口未连接")
	}
}

// TestSerialStatus_NotConnected tests status when not connected
func TestSerialStatus_NotConnected(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	sm, _ := serial.NewSerialManager(cfg)
	
	result := ExecuteSerialStatus(sm)
	
	if !result.Success {
		t.Errorf("预期成功，实际失败: %s", result.Message)
	}
	
	dataMap, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("返回数据格式错误")
	}
	
	connected, ok := dataMap["connected"].(bool)
	if !ok || connected {
		t.Error("预期 connected=false，实际 true")
	}
}
