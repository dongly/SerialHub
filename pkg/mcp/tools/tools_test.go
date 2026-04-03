package tools

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/serial"
)

func getTestPort() string {
	port := os.Getenv("SERIALHUB_TEST_PORT")
	if port == "" {
		port = "COM9"
	}
	return port
}

func newTestManager(t *testing.T) *serial.SerialManager {
	t.Helper()
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("创建 SerialManager 失败: %v", err)
	}
	t.Cleanup(func() {
		if sm.IsConnected() {
			sm.Disconnect()
		}
		sm.Close()
	})
	return sm
}

// ==================== SerialList ====================

func TestSerialList(t *testing.T) {
	sm := newTestManager(t)
	result := ExecuteSerialList(sm)
	if !result.Success {
		t.Errorf("预期成功，实际失败: %s", result.Message)
	}
	if result.Data == nil {
		t.Error("预期返回串口列表，实际为 nil")
	}
}

// ==================== SerialConnect ====================

func TestSerialConnect_Disconnect(t *testing.T) {
	sm := newTestManager(t)
	port := getTestPort()

	result := ExecuteSerialConnect(sm, ConnectInput{Port: port, BaudRate: 115200})
	if !result.Success {
		if strings.Contains(result.Message, "打开串口失败") {
			t.Skipf("串口 %s 不可用: %s", port, result.Message)
		}
		t.Fatalf("连接失败: %s", result.Message)
	}
	if !sm.IsConnected() {
		t.Fatal("连接后 IsConnected 应为 true")
	}

	// 验证返回数据包含端口和波特率信息
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("连接结果 Data 类型不正确")
	}
	if data["port"] != port {
		t.Errorf("预期端口 %s，实际 %v", port, data["port"])
	}
	if data["baudRate"] != 115200 {
		t.Errorf("预期波特率 115200，实际 %v", data["baudRate"])
	}

	result = ExecuteSerialDisconnect(sm)
	if !result.Success {
		t.Fatalf("断开失败: %s", result.Message)
	}
	if sm.IsConnected() {
		t.Error("断开后 IsConnected 应为 false")
	}
}

func TestSerialConnect_WithBaudRate(t *testing.T) {
	sm := newTestManager(t)
	port := getTestPort()

	result := ExecuteSerialConnect(sm, ConnectInput{Port: port, BaudRate: 9600})
	if !result.Success {
		if strings.Contains(result.Message, "打开串口失败") {
			t.Skipf("串口 %s 不可用: %s", port, result.Message)
		}
		t.Fatalf("连接失败: %s", result.Message)
	}

	cfg := sm.GetConfig()
	if cfg.BaudRate != 9600 {
		t.Errorf("预期波特率 9600，实际 %d", cfg.BaudRate)
	}
}

func TestSerialConnect_DefaultBaudRate(t *testing.T) {
	sm := newTestManager(t)
	port := getTestPort()

	// BaudRate=0 应使用默认值 115200
	result := ExecuteSerialConnect(sm, ConnectInput{Port: port, BaudRate: 0})
	if !result.Success {
		if strings.Contains(result.Message, "打开串口失败") {
			t.Skipf("串口 %s 不可用: %s", port, result.Message)
		}
		t.Fatalf("连接失败: %s", result.Message)
	}

	cfg := sm.GetConfig()
	if cfg.BaudRate != 115200 {
		t.Errorf("预期默认波特率 115200，实际 %d", cfg.BaudRate)
	}

	// 验证返回结果中的 baudRate
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("连接结果 Data 类型不正确")
	}
	if data["baudRate"] != 115200 {
		t.Errorf("预期返回波特率 115200，实际 %v", data["baudRate"])
	}
}

func TestSerialConnect_NegativeBaudRate(t *testing.T) {
	sm := newTestManager(t)
	port := getTestPort()

	// 负数波特率应使用默认值 115200
	result := ExecuteSerialConnect(sm, ConnectInput{Port: port, BaudRate: -1})
	if !result.Success {
		if strings.Contains(result.Message, "打开串口失败") {
			t.Skipf("串口 %s 不可用: %s", port, result.Message)
		}
		t.Fatalf("连接失败: %s", result.Message)
	}

	cfg := sm.GetConfig()
	if cfg.BaudRate != 115200 {
		t.Errorf("预期默认波特率 115200，实际 %d", cfg.BaudRate)
	}
}

func TestSerialConnect_AlreadyConnected(t *testing.T) {
	sm := newTestManager(t)
	port := getTestPort()

	result1 := ExecuteSerialConnect(sm, ConnectInput{Port: port, BaudRate: 115200})
	if !result1.Success {
		t.Skipf("串口 %s 不可用: %s", port, result1.Message)
	}

	result2 := ExecuteSerialConnect(sm, ConnectInput{Port: port, BaudRate: 115200})
	if result2.Success {
		t.Error("已连接时再次连接应该失败")
	}
	if !strings.Contains(result2.Message, "串口已连接") {
		t.Errorf("预期包含 '串口已连接'，实际 '%s'", result2.Message)
	}
}

func TestSerialConnect_InvalidPort(t *testing.T) {
	sm := newTestManager(t)

	result := ExecuteSerialConnect(sm, ConnectInput{Port: "INVALID_PORT_99999", BaudRate: 115200})
	if result.Success {
		t.Error("无效端口应该连接失败")
	}
	if !strings.Contains(result.Message, "连接失败") {
		t.Errorf("预期包含 '连接失败'，实际 '%s'", result.Message)
	}
}

// ==================== SerialDisconnect ====================

func TestSerialDisconnect_NotConnected(t *testing.T) {
	sm := newTestManager(t)

	result := ExecuteSerialDisconnect(sm)
	if result.Success {
		t.Error("未连接时断开应该失败")
	}
	if result.Message != "串口未连接" {
		t.Errorf("预期消息 '串口未连接'，实际 '%s'", result.Message)
	}
}

// ==================== SerialWrite ====================

func TestSerialWrite_NotConnected(t *testing.T) {
	sm := newTestManager(t)

	result := ExecuteSerialWrite(sm, WriteInput{Data: "test"})
	if result.Success {
		t.Error("未连接时写入应该失败")
	}
	if result.Message != "串口未连接" {
		t.Errorf("预期消息 '串口未连接'，实际 '%s'", result.Message)
	}
}

func connectTestPort(t *testing.T, sm *serial.SerialManager) {
	t.Helper()
	port := getTestPort()
	result := ExecuteSerialConnect(sm, ConnectInput{Port: port, BaudRate: 115200})
	if !result.Success {
		if strings.Contains(result.Message, "打开串口失败") {
			t.Skipf("串口 %s 不可用: %s", port, result.Message)
		}
		t.Fatalf("连接失败: %s", result.Message)
	}
}

func TestSerialWrite_WithNewline(t *testing.T) {
	sm := newTestManager(t)
	connectTestPort(t, sm)

	result := ExecuteSerialWrite(sm, WriteInput{Data: "AT", AddNewline: false})
	if !result.Success {
		t.Fatalf("写入失败: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("写入结果 Data 类型不正确")
	}
	if data["data"] != "AT" {
		t.Errorf("预期写入数据 'AT'，实际 '%v'", data["data"])
	}
	if data["bytesWritten"] != 2 {
		t.Errorf("预期写入 2 字节，实际 %v", data["bytesWritten"])
	}

	result2 := ExecuteSerialWrite(sm, WriteInput{Data: "AT", AddNewline: true})
	if !result2.Success {
		t.Fatalf("写入失败: %s", result2.Message)
	}
	data2, ok := result2.Data.(map[string]any)
	if !ok {
		t.Fatal("写入结果 Data 类型不正确")
	}
	if data2["data"] != "AT\n" {
		t.Errorf("预期写入数据 'AT\\n'，实际 '%v'", data2["data"])
	}
	if data2["bytesWritten"] != 3 {
		t.Errorf("预期写入 3 字节，实际 %v", data2["bytesWritten"])
	}
}

func TestSerialWrite_EmptyData(t *testing.T) {
	sm := newTestManager(t)
	connectTestPort(t, sm)

	result := ExecuteSerialWrite(sm, WriteInput{Data: "", AddNewline: false})
	if !result.Success {
		t.Fatalf("写入失败: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("写入结果 Data 类型不正确")
	}
	if data["data"] != "\n" {
		t.Errorf("预期写入数据 '\\n'，实际 '%v'", data["data"])
	}
	if data["bytesWritten"] != 1 {
		t.Errorf("预期写入 1 字节，实际 %v", data["bytesWritten"])
	}
}

func TestSerialWrite_DefaultNewline(t *testing.T) {
	sm := newTestManager(t)
	connectTestPort(t, sm)

	result := ExecuteSerialWrite(sm, WriteInput{Data: "test"})
	if !result.Success {
		t.Fatalf("写入失败: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("写入结果 Data 类型不正确")
	}
	if data["data"] != "test" {
		t.Errorf("预期写入数据 'test'，实际 '%v'", data["data"])
	}
}

// ==================== SerialRead ====================

func TestSerialRead_EmptyBuffer(t *testing.T) {
	buf := buffer.NewDataBuffer()
	result := ExecuteSerialRead(buf, ReadInput{Timeout: 100})
	if !result.Success {
		t.Errorf("空缓冲区读取应该成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["timedOut"] != true {
		t.Error("空缓冲区超时时 timedOut 应为 true")
	}
	if data["data"] != "" {
		t.Errorf("预期空数据，实际 '%v'", data["data"])
	}
	if data["bytes"] != 0 {
		t.Errorf("预期 0 字节，实际 %v", data["bytes"])
	}
}

func TestSerialRead_Timeout(t *testing.T) {
	buf := buffer.NewDataBuffer()

	// 空缓冲区，短超时应返回超时
	result := ExecuteSerialRead(buf, ReadInput{Timeout: 50})
	if !result.Success {
		t.Fatalf("超时读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["timedOut"] != true {
		t.Error("预期超时")
	}
	if data["data"] != "" {
		t.Errorf("超时时预期空数据，实际 '%v'", data["data"])
	}
}

func TestSerialRead_TimeoutWithData(t *testing.T) {
	buf := buffer.NewDataBuffer()
	// 预先写入数据
	buf.Append([]byte("hello world"))

	// 超时模式下缓冲区有数据，应在 ticker 检测到后立即返回
	result := ExecuteSerialRead(buf, ReadInput{Timeout: 500})
	if !result.Success {
		t.Fatalf("读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["data"] != "hello world" {
		t.Errorf("预期数据 'hello world'，实际 '%v'", data["data"])
	}
	if data["timedOut"] == true {
		t.Error("有数据时不应超时")
	}
	if data["bytes"] != 11 {
		t.Errorf("预期 11 字节，实际 %v", data["bytes"])
	}
}

func TestSerialRead_MaxSize(t *testing.T) {
	buf := buffer.NewDataBuffer()
	// 写入较长数据
	buf.Append([]byte("abcdefghijklmnopqrstuvwxyz"))

	// maxSize=5 应只读取前 5 字节
	result := ExecuteSerialRead(buf, ReadInput{Timeout: 100, MaxSize: 5})
	if !result.Success {
		t.Fatalf("读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["data"] != "abcde" {
		t.Errorf("预期数据 'abcde'，实际 '%v'", data["data"])
	}
	if data["bytes"] != 5 {
		t.Errorf("预期 5 字节，实际 %v", data["bytes"])
	}
}

func TestSerialRead_MaxSizeExceedsDefault(t *testing.T) {
	buf := buffer.NewDataBuffer()
	buf.Append([]byte("short"))

	// maxSize > 4096 应被限制为 4096
	result := ExecuteSerialRead(buf, ReadInput{Timeout: 100, MaxSize: 8192})
	if !result.Success {
		t.Fatalf("读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["data"] != "short" {
		t.Errorf("预期数据 'short'，实际 '%v'", data["data"])
	}
}

func TestSerialRead_DefaultMaxSize(t *testing.T) {
	buf := buffer.NewDataBuffer()
	buf.Append([]byte("data"))

	// maxSize=0 应使用默认 4096
	result := ExecuteSerialRead(buf, ReadInput{Timeout: 100, MaxSize: 0})
	if !result.Success {
		t.Fatalf("读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["data"] != "data" {
		t.Errorf("预期数据 'data'，实际 '%v'", data["data"])
	}
}

func TestSerialRead_NegativeMaxSize(t *testing.T) {
	buf := buffer.NewDataBuffer()
	buf.Append([]byte("test"))

	// 负数 maxSize 应使用默认 4096
	result := ExecuteSerialRead(buf, ReadInput{Timeout: 100, MaxSize: -1})
	if !result.Success {
		t.Fatalf("读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["data"] != "test" {
		t.Errorf("预期数据 'test'，实际 '%v'", data["data"])
	}
}

func TestSerialRead_ConcurrentWrite(t *testing.T) {
	buf := buffer.NewDataBuffer()

	go func() {
		time.Sleep(50 * time.Millisecond)
		buf.Append([]byte("delayed"))
	}()

	result := ExecuteSerialRead(buf, ReadInput{Timeout: 500})
	if !result.Success {
		t.Fatalf("读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["data"] != "delayed" {
		t.Errorf("预期数据 'delayed'，实际 '%v'", data["data"])
	}
	if data["timedOut"] == true {
		t.Error("不应超时")
	}
}

func TestSerialRead_InfiniteWait(t *testing.T) {
	buf := buffer.NewDataBuffer()

	go func() {
		time.Sleep(100 * time.Millisecond)
		buf.Append([]byte("infinite"))
	}()

	// Timeout=0 表示无限等待
	result := ExecuteSerialRead(buf, ReadInput{Timeout: 0})
	if !result.Success {
		t.Fatalf("读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["data"] != "infinite" {
		t.Errorf("预期数据 'infinite'，实际 '%v'", data["data"])
	}
	if data["bytes"] != 8 {
		t.Errorf("预期 8 字节，实际 %v", data["bytes"])
	}
}

func TestSerialRead_TimeoutWithDataAtLastMoment(t *testing.T) {
	buf := buffer.NewDataBuffer()

	// 在接近超时时间时写入数据
	go func() {
		time.Sleep(30 * time.Millisecond)
		buf.Append([]byte("lastms"))
	}()

	result := ExecuteSerialRead(buf, ReadInput{Timeout: 200})
	if !result.Success {
		t.Fatalf("读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["timedOut"] == true {
		t.Error("数据在超时前到达，不应超时")
	}
	if data["data"] != "lastms" {
		t.Errorf("预期数据 'lastms'，实际 '%v'", data["data"])
	}
}

// ==================== SerialStatus ====================

func TestSerialStatus_NotConnected(t *testing.T) {
	sm := newTestManager(t)

	result := ExecuteSerialStatus(sm)
	if !result.Success {
		t.Errorf("状态查询应该成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data 类型不正确")
	}
	if data["connected"].(bool) {
		t.Error("未连接时 connected 应为 false")
	}
}

func TestSerialStatus_Connected(t *testing.T) {
	sm := newTestManager(t)
	port := getTestPort()

	// 先连接
	connectResult := ExecuteSerialConnect(sm, ConnectInput{Port: port, BaudRate: 115200})
	if !connectResult.Success {
		if strings.Contains(connectResult.Message, "打开串口失败") {
			t.Skipf("串口 %s 不可用: %s", port, connectResult.Message)
		}
		t.Fatalf("连接失败: %s", connectResult.Message)
	}

	// 查询已连接状态
	result := ExecuteSerialStatus(sm)
	if !result.Success {
		t.Errorf("状态查询应该成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Data 类型不正确")
	}
	if !data["connected"].(bool) {
		t.Error("已连接时 connected 应为 true")
	}
	if data["port"] != port {
		t.Errorf("预期端口 %s，实际 %v", port, data["port"])
	}
	if data["baudRate"] != 115200 {
		t.Errorf("预期波特率 115200，实际 %v", data["baudRate"])
	}
	if data["dataBits"] != 8 {
		t.Errorf("预期数据位 8，实际 %v", data["dataBits"])
	}
	if data["parity"] != "none" {
		t.Errorf("预期校验位 none，实际 %v", data["parity"])
	}
	if data["stopBits"] != float32(1) {
		t.Errorf("预期停止位 1，实际 %v", data["stopBits"])
	}

	// 验证消息包含配置信息
	if !strings.Contains(result.Message, "串口已连接") {
		t.Errorf("预期包含 '串口已连接'，实际 '%s'", result.Message)
	}
}

// ==================== Hardware Tests ====================

func TestSerialWriteRead_Hardware(t *testing.T) {
	port := os.Getenv("SERIALHUB_HARDWARE_TEST")
	if port == "" {
		t.Skip("跳过硬件测试: SERIALHUB_HARDWARE_TEST 未设置")
	}

	testPort := getTestPort()
	sm := newTestManager(t)
	buf := buffer.NewDataBuffer()

	connectResult := ExecuteSerialConnect(sm, ConnectInput{Port: testPort, BaudRate: 115200})
	if !connectResult.Success {
		t.Fatalf("连接失败: %s", connectResult.Message)
	}
	_ = port

	writeResult := ExecuteSerialWrite(sm, WriteInput{Data: "version", AddNewline: true})
	if !writeResult.Success {
		t.Fatalf("写入失败: %s", writeResult.Message)
	}

	time.Sleep(2 * time.Second)

	readResult := ExecuteSerialRead(buf, ReadInput{Timeout: 3000})
	if readResult.Success {
		data, _ := readResult.Data.(map[string]interface{})
		if data != nil {
			t.Logf("读取到数据: %v", data["data"])
		}
	}
}
