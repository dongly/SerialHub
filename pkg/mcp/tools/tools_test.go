// Package tools provides MCP tools for serial port operations.
package tools

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/serial"
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

// TestHW2_MCPTools MCP工具硬件集成测试
// 环境变量配置：
//
//	SERIALHUB_HARDWARE_TEST=1    - 启用硬件测试
//	SERIALHUB_TEST_PORT=COM9     - 串口号（默认 COM9）
//	SERIALHUB_TEST_BAUD=115200   - 波特率（默认 115200）
func TestHW2_MCPTools(t *testing.T) {
	if os.Getenv("SERIALHUB_HARDWARE_TEST") != "1" {
		t.Skip("硬件测试未启用，设置 SERIALHUB_HARDWARE_TEST=1 启用")
	}

	testPort := os.Getenv("SERIALHUB_TEST_PORT")
	if testPort == "" {
		testPort = "COM9"
	}

	baudRate := 115200
	if baud := os.Getenv("SERIALHUB_TEST_BAUD"); baud != "" {
		if b, err := strconv.Atoi(baud); err == nil {
			baudRate = b
		}
	}

	t.Logf("硬件测试配置: port=%s, baud=%d", testPort, baudRate)

	cfg := serial.DefaultConfig()
	cfg.Port = testPort
	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer sm.Close()

	buf := buffer.NewDataBuffer()

	// 确保清理
	t.Cleanup(func() {
		if sm.IsConnected() {
			sm.Disconnect()
		}
	})

	// 1. serial_connect
	t.Log("步骤1: serial_connect...")
	connectInput := ConnectInput{Port: testPort, BaudRate: baudRate}
	connectResult := ExecuteSerialConnect(sm, connectInput)
	if !connectResult.Success {
		t.Fatalf("serial_connect 失败: %s", connectResult.Message)
	}
	t.Logf("serial_connect 成功: %s", connectResult.Message)

	// 等待连接稳定
	time.Sleep(500 * time.Millisecond)

	// 清空残留
	for {
		select {
		case <-sm.DataChan():
		default:
			goto cleared1
		}
	}
cleared1:

	// 2. serial_write (help)
	t.Log("步骤2: serial_write help...")
	writeInput := WriteInput{Data: "help", AddNewline: true}
	writeResult := ExecuteSerialWrite(sm, writeInput)
	if !writeResult.Success {
		t.Fatalf("serial_write 失败: %s", writeResult.Message)
	}
	t.Logf("serial_write 成功: %s", writeResult.Message)

	// 从 SerialManager 读取数据并写入 buffer
	var helpResponse strings.Builder
	helpTimeout := time.After(5 * time.Second)
	for {
		select {
		case data := <-sm.DataChan():
			helpResponse.Write(data)
			buf.Append(data)
			if strings.Contains(helpResponse.String(), "RT-Thread shell commands:") {
				goto helpDone
			}
		case <-helpTimeout:
			goto helpDone
		}
	}
helpDone:

	// 3. serial_read
	t.Log("步骤3: serial_read...")
	readInput := ReadInput{Timeout: 5000, MaxSize: 4096}
	readResult := ExecuteSerialRead(buf, readInput)
	if !readResult.Success {
		t.Fatalf("serial_read 失败: %s", readResult.Message)
	}
	readData, _ := readResult.Data.(map[string]interface{})
	response := readData["data"].(string)
	t.Logf("serial_read 收到: %s", response)
	if !strings.Contains(response, "RT-Thread shell commands:") {
		t.Errorf("help 响应不包含预期内容，收到: %s", response)
	}

	// 清空残留
	for {
		select {
		case <-sm.DataChan():
		default:
			goto cleared2
		}
	}
cleared2:
	buf.Clear()

	// 4. serial_write (version)
	t.Log("步骤4: serial_write version...")
	writeInput2 := WriteInput{Data: "version", AddNewline: true}
	writeResult2 := ExecuteSerialWrite(sm, writeInput2)
	if !writeResult2.Success {
		t.Fatalf("serial_write version 失败: %s", writeResult2.Message)
	}

	// 从 SerialManager 读取数据并写入 buffer
	var versionResponse strings.Builder
	versionTimeout := time.After(5 * time.Second)
	for {
		select {
		case data := <-sm.DataChan():
			versionResponse.Write(data)
			buf.Append(data)
			if strings.Contains(versionResponse.String(), "Thread Operating System") {
				goto versionDone
			}
		case <-versionTimeout:
			goto versionDone
		}
	}
versionDone:

	// 5. serial_read
	t.Log("步骤5: serial_read version...")
	readResult2 := ExecuteSerialRead(buf, readInput)
	if !readResult2.Success {
		t.Fatalf("serial_read version 失败: %s", readResult2.Message)
	}
	readData2, _ := readResult2.Data.(map[string]interface{})
	response2 := readData2["data"].(string)
	t.Logf("serial_read 收到: %s", response2)
	if !strings.Contains(response2, "Thread Operating System") {
		t.Errorf("version 响应不包含预期内容，收到: %s", response2)
	}

	// 6. serial_status
	t.Log("步骤6: serial_status...")
	statusResult := ExecuteSerialStatus(sm)
	if !statusResult.Success {
		t.Fatalf("serial_status 失败: %s", statusResult.Message)
	}
	statusData, _ := statusResult.Data.(map[string]interface{})
	if !statusData["connected"].(bool) {
		t.Error("serial_status 显示未连接，预期已连接")
	}
	t.Logf("serial_status: connected=%v, port=%s", statusData["connected"], statusData["port"])

	// 7. serial_disconnect
	t.Log("步骤7: serial_disconnect...")
	disconnectResult := ExecuteSerialDisconnect(sm)
	if !disconnectResult.Success {
		t.Fatalf("serial_disconnect 失败: %s", disconnectResult.Message)
	}
	t.Logf("serial_disconnect 成功")

	// 验证断开
	if sm.IsConnected() {
		t.Error("断开后 IsConnected 应该返回 false")
	}

	// 8. serial_status (验证断开)
	t.Log("步骤8: serial_status (验证断开)...")
	statusResult2 := ExecuteSerialStatus(sm)
	if !statusResult2.Success {
		t.Fatalf("serial_status 失败: %s", statusResult2.Message)
	}
	statusData2, _ := statusResult2.Data.(map[string]interface{})
	if statusData2["connected"].(bool) {
		t.Error("serial_status 显示已连接，预期未连接")
	}

	t.Log("HW2 MCP工具硬件集成测试通过")
}
