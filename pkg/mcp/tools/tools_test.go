package tools

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dongly/serialhub/internal/buffer"
	"github.com/dongly/serialhub/pkg/serial"
)

func getTestPort() string {
	port := os.Getenv("SERIALHUB_TEST_PORT")
	if port == "" {
		port = "COM9"
	}
	return port
}

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }

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
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("串口列表 Data 类型应为对象")
	}
	if _, ok := data["ports"].([]string); !ok {
		t.Errorf("预期 data['ports'] 为字符串数组，实际类型 %T", data["ports"])
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

	result := ExecuteSerialWrite(sm, WriteInput{Data: "AT", AddNewline: boolPtr(false)})
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

	result2 := ExecuteSerialWrite(sm, WriteInput{Data: "AT", AddNewline: boolPtr(true)})
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

	result := ExecuteSerialWrite(sm, WriteInput{Data: "", AddNewline: boolPtr(false)})
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

	// 不传 AddNewline 时默认自动追加换行符
	result := ExecuteSerialWrite(sm, WriteInput{Data: "test"})
	if !result.Success {
		t.Fatalf("写入失败: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("写入结果 Data 类型不正确")
	}
	if data["data"] != "test\n" {
		t.Errorf("预期写入数据 'test\\n'，实际 '%v'", data["data"])
	}
	if data["bytesWritten"] != 5 {
		t.Errorf("预期写入 5 字节，实际 %v", data["bytesWritten"])
	}
}

// ==================== SerialRead ====================

func TestSerialRead_EmptyBuffer(t *testing.T) {
	buf := buffer.NewDataBuffer()
	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(100)})
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

func TestSerialRead_DefaultTimeout(t *testing.T) {
	buf := buffer.NewDataBuffer()

	// 不传 Timeout 时默认超时 1000ms，空缓冲区应超时返回而非无限等待
	start := time.Now()
	result := ExecuteSerialRead(context.Background(), buf, ReadInput{})
	elapsed := time.Since(start)
	if !result.Success {
		t.Fatalf("默认超时读取应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("读取结果 Data 类型不正确")
	}
	if data["timedOut"] != true {
		t.Error("空缓冲区默认超时时 timedOut 应为 true")
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("默认超时应等待约 1000ms，实际仅等待 %v", elapsed)
	}
}

func TestSerialRead_Timeout(t *testing.T) {
	buf := buffer.NewDataBuffer()

	// 空缓冲区，短超时应返回超时
	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(50)})
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
	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(500)})
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
	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(100), MaxSize: 5})
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
	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(100), MaxSize: 8192})
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
	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(100), MaxSize: 0})
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
	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(100), MaxSize: -1})
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

	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(500)})
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
	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(0)})
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

// 无限等待期间 ctx 取消应立即返回失败，不得永久阻塞（goroutine 泄漏防护）
func TestSerialRead_InfiniteWait_Cancelled(t *testing.T) {
	buf := buffer.NewDataBuffer()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	result := ExecuteSerialRead(ctx, buf, ReadInput{Timeout: intPtr(0)})
	elapsed := time.Since(start)

	if result.Success {
		t.Fatal("取消后读取应失败")
	}
	if result.Message != "读取已取消" {
		t.Errorf("预期消息 '读取已取消'，实际: %s", result.Message)
	}
	if elapsed > 2*time.Second {
		t.Errorf("取消后应立即返回，实际耗时: %v", elapsed)
	}
}

func TestSerialRead_TimeoutWithDataAtLastMoment(t *testing.T) {
	buf := buffer.NewDataBuffer()

	// 在接近超时时间时写入数据
	go func() {
		time.Sleep(30 * time.Millisecond)
		buf.Append([]byte("lastms"))
	}()

	result := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(200)})
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

// ==================== SerialClear ====================

func TestSerialClear_WithData(t *testing.T) {
	buf := buffer.NewDataBuffer()
	buf.Append([]byte("hello world"))

	result := ExecuteSerialClear(buf)
	if !result.Success {
		t.Fatalf("清空应成功: %s", result.Message)
	}
	if buf.Length() != 0 {
		t.Errorf("清空后缓冲区长度应为 0，实际 %d", buf.Length())
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("清空结果 Data 类型不正确")
	}
	if data["clearedBytes"] != 11 {
		t.Errorf("预期清空 11 字节，实际 %v", data["clearedBytes"])
	}
	if data["bufferLength"] != 0 {
		t.Errorf("预期清空后长度 0，实际 %v", data["bufferLength"])
	}
}

func TestSerialClear_EmptyBuffer(t *testing.T) {
	buf := buffer.NewDataBuffer()

	result := ExecuteSerialClear(buf)
	if !result.Success {
		t.Fatalf("空缓冲区清空应成功: %s", result.Message)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("清空结果 Data 类型不正确")
	}
	if data["clearedBytes"] != 0 {
		t.Errorf("空缓冲区预期清空 0 字节，实际 %v", data["clearedBytes"])
	}

	// 清空后仍可继续使用
	buf.Append([]byte("new"))
	if buf.Length() != 3 {
		t.Errorf("清空后追加数据长度应为 3，实际 %d", buf.Length())
	}
}

func TestSerialClear_NilBuffer(t *testing.T) {
	result := ExecuteSerialClear(nil)
	if result.Success {
		t.Error("nil 缓冲区预期失败")
	}
	if result.Message != "数据缓冲区未初始化" {
		t.Errorf("预期消息 '数据缓冲区未初始化'，实际 '%s'", result.Message)
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

// ==================== Nil Manager Tests ====================

func TestSerialList_NilManager(t *testing.T) {
	result := ExecuteSerialList(nil)
	if result.Success {
		t.Error("nil manager 预期失败")
	}
	if result.Message != "串口管理器未初始化" {
		t.Errorf("预期消息 '串口管理器未初始化'，实际 '%s'", result.Message)
	}
}

func TestSerialConnect_NilManager(t *testing.T) {
	result := ExecuteSerialConnect(nil, ConnectInput{Port: "COM1", BaudRate: 115200})
	if result.Success {
		t.Error("nil manager 预期失败")
	}
	if result.Message != "串口管理器未初始化" {
		t.Errorf("预期消息 '串口管理器未初始化'，实际 '%s'", result.Message)
	}
}

func TestSerialDisconnect_NilManager(t *testing.T) {
	result := ExecuteSerialDisconnect(nil)
	if result.Success {
		t.Error("nil manager 预期失败")
	}
	if result.Message != "串口管理器未初始化" {
		t.Errorf("预期消息 '串口管理器未初始化'，实际 '%s'", result.Message)
	}
}

func TestSerialWrite_NilManager(t *testing.T) {
	result := ExecuteSerialWrite(nil, WriteInput{Data: "test"})
	if result.Success {
		t.Error("nil manager 预期失败")
	}
	if result.Message != "串口管理器未初始化" {
		t.Errorf("预期消息 '串口管理器未初始化'，实际 '%s'", result.Message)
	}
}

func TestSerialStatus_NilManager(t *testing.T) {
	result := ExecuteSerialStatus(nil)
	if !result.Success {
		t.Error("nil manager 状态查询应该成功（返回未连接状态）")
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatal("Data 类型不正确")
	}
	if data["connected"].(bool) {
		t.Error("nil manager 时 connected 应为 false")
	}
}

func TestToolResult_Fields(t *testing.T) {
	tests := []struct {
		name    string
		result  ToolResult
		success bool
		message string
	}{
		{
			name:    "成功结果",
			result:  ToolResult{Success: true, Message: "操作成功", Data: "test"},
			success: true,
			message: "操作成功",
		},
		{
			name:    "失败结果",
			result:  ToolResult{Success: false, Message: "操作失败"},
			success: false,
			message: "操作失败",
		},
		{
			name:    "nil Data",
			result:  ToolResult{Success: true, Message: "成功", Data: nil},
			success: true,
			message: "成功",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Success != tt.success {
				t.Errorf("Success = %v, want %v", tt.result.Success, tt.success)
			}
			if tt.result.Message != tt.message {
				t.Errorf("Message = %s, want %s", tt.result.Message, tt.message)
			}
		})
	}
}

func TestConnectInput_Defaults(t *testing.T) {
	input := ConnectInput{Port: "COM1"}
	if input.Port != "COM1" {
		t.Errorf("Port = %s, want COM1", input.Port)
	}
	if input.BaudRate != 0 {
		t.Errorf("默认 BaudRate 应为 0，实际 %d", input.BaudRate)
	}
}

func TestWriteInput_Defaults(t *testing.T) {
	input := WriteInput{Data: "hello"}
	if input.Data != "hello" {
		t.Errorf("Data = %s, want hello", input.Data)
	}
	if input.AddNewline != nil {
		t.Error("默认 AddNewline 应为 nil（未指定，行为上等同自动追加换行符）")
	}
}

func TestReadInput_Defaults(t *testing.T) {
	input := ReadInput{}
	if input.Timeout != nil {
		t.Error("默认 Timeout 应为 nil（未指定，行为上等同 1000ms 超时）")
	}
	if input.MaxSize != 0 {
		t.Errorf("默认 MaxSize 应为 0，实际 %d", input.MaxSize)
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

	writeResult := ExecuteSerialWrite(sm, WriteInput{Data: "version", AddNewline: boolPtr(true)})
	if !writeResult.Success {
		t.Fatalf("写入失败: %s", writeResult.Message)
	}

	time.Sleep(2 * time.Second)

	readResult := ExecuteSerialRead(context.Background(), buf, ReadInput{Timeout: intPtr(3000)})
	if readResult.Success {
		data, _ := readResult.Data.(map[string]interface{})
		if data != nil {
			t.Logf("读取到数据: %v", data["data"])
		}
	}
}
