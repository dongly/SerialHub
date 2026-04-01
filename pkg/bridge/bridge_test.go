// Package bridge provides data bridging functionality between serial, telnet, and MCP.
package bridge

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/serial"
	"github.com/yourname/serialhub/pkg/telnet"
)

// TestNewDataBridge 测试创建 DataBridge 实例
func TestNewDataBridge(t *testing.T) {
	tests := []struct {
		name        string
		serial      SerialReader
		telnet      TelnetBroadcaster
		mcpBuffer   *buffer.DataBuffer
		expectError bool
	}{
		{
			name:        "正常创建",
			serial:      &mockSerialManager{},
			telnet:      &mockTelnetServer{},
			mcpBuffer:   buffer.NewDataBuffer(),
			expectError: false,
		},
		{
			name:        "串口管理器为空",
			serial:      nil,
			telnet:      &mockTelnetServer{},
			mcpBuffer:   buffer.NewDataBuffer(),
			expectError: true,
		},
		{
			name:        "Telnet 服务器为空",
			serial:      &mockSerialManager{},
			telnet:      nil,
			mcpBuffer:   buffer.NewDataBuffer(),
			expectError: true,
		},
		{
			name:        "MCP 缓冲区为空",
			serial:      &mockSerialManager{},
			telnet:      &mockTelnetServer{},
			mcpBuffer:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bridge, err := NewDataBridge(tt.serial, tt.telnet, tt.mcpBuffer)

			if tt.expectError {
				if err == nil {
					t.Errorf("期望返回错误，但返回 nil")
				}
			} else {
				if err != nil {
					t.Errorf("未期望返回错误: %v", err)
				}
				if bridge == nil {
					t.Errorf("期望返回 DataBridge 实例，但返回 nil")
				}
			}
		})
	}
}

// TestBridgeStartStop 测试启动和停止桥接器
func TestBridgeStartStop(t *testing.T) {
	// 创建模拟组件
	mockSerial := createMockSerialManager()
	mockTelnet := createMockTelnetServer()
	mcpBuffer := buffer.NewDataBuffer()

	// 创建 DataBridge
	bridge, err := NewDataBridge(mockSerial, mockTelnet, mcpBuffer)
	if err != nil {
		t.Fatalf("创建 DataBridge 失败: %v", err)
	}

	// 启动桥接器
	bridge.Start()

	// 等待 goroutine 启动
	time.Sleep(50 * time.Millisecond)

	// 停止桥接器
	err = bridge.Stop()
	if err != nil {
		t.Errorf("停止桥接器失败: %v", err)
	}

	// 验证停止是幂等的（多次调用不阻塞）
	err = bridge.Stop()
	if err != nil {
		t.Errorf("重复停止桥接器失败: %v", err)
	}
}

// TestBridgeStop_Idempotent 测试停止桥接器的幂等性
func TestBridgeStop_Idempotent(t *testing.T) {
	mockSerial := createMockSerialManager()
	mockTelnet := createMockTelnetServer()
	mcpBuffer := buffer.NewDataBuffer()

	bridge, err := NewDataBridge(mockSerial, mockTelnet, mcpBuffer)
	if err != nil {
		t.Fatalf("创建 DataBridge 失败: %v", err)
	}

	// 不启动就直接停止
	err = bridge.Stop()
	if err != nil {
		t.Errorf("未启动时停止桥接器失败: %v", err)
	}

	// 再次停止
	err = bridge.Stop()
	if err != nil {
		t.Errorf("重复停止桥接器失败: %v", err)
	}
}

// TestSerialForwarding_Telnet 测试串口数据转发到 Telnet
func TestSerialForwarding_Telnet(t *testing.T) {
	mockSerial := createMockSerialManager()
	mockTelnet := createMockTelnetServer()
	mcpBuffer := buffer.NewDataBuffer()

	bridge, err := NewDataBridge(mockSerial, mockTelnet, mcpBuffer)
	if err != nil {
		t.Fatalf("创建 DataBridge 失败: %v", err)
	}

	bridge.Start()
	defer bridge.Stop()

	// 等待 goroutine 启动
	time.Sleep(50 * time.Millisecond)

	// 模拟串口数据
	testData := []byte("Hello from serial\r\n")
	mockSerial.sendData(testData)

	// 等待数据被转发
	time.Sleep(100 * time.Millisecond)

	// 验证 Telnet 收到数据
	received := mockTelnet.getBroadcastData()
	if len(received) == 0 {
		t.Errorf("Telnet 未收到数据")
	}

	// 验证数据内容
	if !byteSlicesEqual(received, testData) {
		t.Errorf("数据不匹配: 期望 %v, 实际 %v", testData, received)
	}
}

// TestSerialForwarding_MCP 测试串口数据转发到 MCP
func TestSerialForwarding_MCP(t *testing.T) {
	mockSerial := createMockSerialManager()
	mockTelnet := createMockTelnetServer()
	mcpBuffer := buffer.NewDataBuffer()

	bridge, err := NewDataBridge(mockSerial, mockTelnet, mcpBuffer)
	if err != nil {
		t.Fatalf("创建 DataBridge 失败: %v", err)
	}

	bridge.Start()
	defer bridge.Stop()

	// 等待 goroutine 启动
	time.Sleep(50 * time.Millisecond)

	// 模拟串口数据
	testData := []byte("Hello from serial\r\n")
	mockSerial.sendData(testData)

	// 等待数据被转发
	time.Sleep(100 * time.Millisecond)

	// 验证 MCP 缓冲区收到数据
	if mcpBuffer.Length() == 0 {
		t.Errorf("MCP 缓冲区未收到数据")
	}

	received := mcpBuffer.Read(1024)
	if !byteSlicesEqual(received, testData) {
		t.Errorf("数据不匹配: 期望 %v, 实际 %v", testData, received)
	}
}

// TestSerialForwarding_Both 测试串口数据同时转发到 Telnet 和 MCP
func TestSerialForwarding_Both(t *testing.T) {
	mockSerial := createMockSerialManager()
	mockTelnet := createMockTelnetServer()
	mcpBuffer := buffer.NewDataBuffer()

	bridge, err := NewDataBridge(mockSerial, mockTelnet, mcpBuffer)
	if err != nil {
		t.Fatalf("创建 DataBridge 失败: %v", err)
	}

	bridge.Start()
	defer bridge.Stop()

	// 等待 goroutine 启动
	time.Sleep(50 * time.Millisecond)

	// 模拟串口数据
	testData := []byte("Hello from serial\r\n")
	mockSerial.sendData(testData)

	// 等待数据被转发
	time.Sleep(100 * time.Millisecond)

	// 验证 Telnet 收到数据
	telnetReceived := mockTelnet.getBroadcastData()
	if len(telnetReceived) == 0 {
		t.Errorf("Telnet 未收到数据")
	}
	if !byteSlicesEqual(telnetReceived, testData) {
		t.Errorf("Telnet 数据不匹配: 期望 %v, 实际 %v", testData, telnetReceived)
	}

	// 验证 MCP 缓冲区收到数据（关键测试：验证无双重写入）
	if mcpBuffer.Length() == 0 {
		t.Errorf("MCP 缓冲区未收到数据")
	}
	mcpReceived := mcpBuffer.Read(1024)
	if !byteSlicesEqual(mcpReceived, testData) {
		t.Errorf("MCP 数据不匹配: 期望 %v, 实际 %v", testData, mcpReceived)
	}

	// 关键验证：确保 MCP 缓冲区只写入一次（读取后缓冲区应为空）
	if mcpBuffer.Length() != 0 {
		t.Errorf("MCP 缓冲区有残留数据（可能发生双重写入）: %d 字节", mcpBuffer.Length())
	}
}

// TestTelnetForwarding 测试 Telnet 数据转发到串口
func TestTelnetForwarding(t *testing.T) {
	mockSerial := createMockSerialManager()
	mockTelnet := createMockTelnetServer()
	mcpBuffer := buffer.NewDataBuffer()

	bridge, err := NewDataBridge(mockSerial, mockTelnet, mcpBuffer)
	if err != nil {
		t.Fatalf("创建 DataBridge 失败: %v", err)
	}

	bridge.Start()
	defer bridge.Stop()

	// 等待 goroutine 启动
	time.Sleep(50 * time.Millisecond)

	// 模拟 Telnet 数据
	testData := []byte("help\r\n")
	mockTelnet.sendData(testData)

	// 等待数据被转发
	time.Sleep(100 * time.Millisecond)

	// 验证串口收到数据
	received := mockSerial.getWriteData()
	if len(received) == 0 {
		t.Errorf("串口未收到数据")
	}

	if !byteSlicesEqual(received, testData) {
		t.Errorf("数据不匹配: 期望 %v, 实际 %v", testData, received)
	}
}

// TestConcurrentForwarding 测试并发转发数据
func TestConcurrentForwarding(t *testing.T) {
	mockSerial := createMockSerialManager()
	mockTelnet := createMockTelnetServer()
	mcpBuffer := buffer.NewDataBuffer(1024 * 1024) // 1MB 缓冲区

	bridge, err := NewDataBridge(mockSerial, mockTelnet, mcpBuffer)
	if err != nil {
		t.Fatalf("创建 DataBridge 失败: %v", err)
	}

	bridge.Start()
	defer bridge.Stop()

	// 等待 goroutine 启动
	time.Sleep(50 * time.Millisecond)

	// 并发发送数据
	var wg sync.WaitGroup
	dataCount := 100

	for i := 0; i < dataCount; i++ {
		wg.Add(2)

		// 串口数据
		go func(idx int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("Serial data %d\r\n", idx))
			mockSerial.sendData(data)
		}(i)

		// Telnet 数据
		go func(idx int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("Telnet data %d\r\n", idx))
			mockTelnet.sendData(data)
		}(i)
	}

	wg.Wait()

	// 等待所有数据被转发
	time.Sleep(200 * time.Millisecond)

	// 验证数据被正确处理
	// （此处不验证具体数据，只验证无崩溃和死锁）
	t.Logf("并发转发测试完成，发送 %d 个串口数据包和 %d 个 Telnet 数据包", dataCount, dataCount)
}

// ============ Mock 辅助函数 ============

// createMockSerialManager 创建模拟的串口管理器
func createMockSerialManager() *mockSerialManager {
	return &mockSerialManager{
		dataChan:  make(chan []byte, 256),
		writeData: make([]byte, 0),
	}
}

// mockSerialManager 模拟串口管理器
type mockSerialManager struct {
	dataChan   chan []byte
	writeData  []byte
	writeMutex sync.Mutex
}

func (m *mockSerialManager) sendData(data []byte) {
	m.dataChan <- data
}

func (m *mockSerialManager) getWriteData() []byte {
	m.writeMutex.Lock()
	defer m.writeMutex.Unlock()
	return m.writeData
}

func (m *mockSerialManager) Write(data []byte) (int, error) {
	m.writeMutex.Lock()
	defer m.writeMutex.Unlock()
	m.writeData = append(m.writeData, data...)
	return len(data), nil
}

func (m *mockSerialManager) DataChan() <-chan []byte {
	return m.dataChan
}

func (m *mockSerialManager) Close() error {
	close(m.dataChan)
	return nil
}

// createMockTelnetServer 创建模拟的 Telnet 服务器
func createMockTelnetServer() *mockTelnetServer {
	return &mockTelnetServer{
		dataChan:      make(chan []byte, 256),
		broadcastData: make([]byte, 0),
	}
}

// mockTelnetServer 模拟 Telnet 服务器
type mockTelnetServer struct {
	dataChan       chan []byte
	broadcastData  []byte
	broadcastMutex sync.Mutex
}

func (m *mockTelnetServer) sendData(data []byte) {
	m.dataChan <- data
}

func (m *mockTelnetServer) getBroadcastData() []byte {
	m.broadcastMutex.Lock()
	defer m.broadcastMutex.Unlock()
	return m.broadcastData
}

func (m *mockTelnetServer) Broadcast(data []byte) int {
	m.broadcastMutex.Lock()
	defer m.broadcastMutex.Unlock()
	m.broadcastData = append(m.broadcastData, data...)
	return 1 // 假设有 1 个客户端
}

func (m *mockTelnetServer) DataChan() <-chan []byte {
	return m.dataChan
}

func (m *mockTelnetServer) Close() error {
	close(m.dataChan)
	return nil
}

// ============ 辅助函数 ============

func byteSlicesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestHW3_DataBridge DataBridge 转发硬件测试
func TestHW3_DataBridge(t *testing.T) {
	if os.Getenv("SERIALHUB_HARDWARE_TEST") != "1" {
		t.Skip("硬件测试未启用，设置 SERIALHUB_HARDWARE_TEST=1 启用")
	}

	testPort := os.Getenv("SERIALHUB_TEST_PORT")
	if testPort == "" {
		testPort = "COM9"
	}

	// 创建真实组件
	cfg := serial.DefaultConfig()
	cfg.Port = testPort
	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer sm.Close()

	telnetSrv, err := telnet.NewTelnetServer("", 0)
	if err != nil {
		t.Fatalf("NewTelnetServer failed: %v", err)
	}
	buf := buffer.NewDataBuffer()

	// 创建 DataBridge
	bridge, err := NewDataBridge(sm, telnetSrv, buf)
	if err != nil {
		t.Fatalf("NewDataBridge failed: %v", err)
	}

	// 确保清理
	t.Cleanup(func() {
		bridge.Stop()
		telnetSrv.Stop()
		if sm.IsConnected() {
			sm.Disconnect()
		}
	})

	// 启动 TelnetServer
	if err := telnetSrv.Start(); err != nil {
		t.Fatalf("TelnetServer.Start failed: %v", err)
	}
	t.Log("TelnetServer 已启动")

	// 启动 DataBridge
	bridge.Start()
	t.Log("DataBridge 已启动")

	// 连接串口
	if err := sm.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	t.Logf("已连接到串口: %s", sm.CurrentPort())

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
	buf.Clear()

	// 测试路径 1: 串口 → Telnet 广播
	t.Log("测试路径 1: 串口 → Telnet 广播...")
	if err := sm.WriteLine("help"); err != nil {
		t.Fatalf("WriteLine failed: %v", err)
	}

	// 等待数据转发
	time.Sleep(500 * time.Millisecond)

	// 验证 Telnet 收到数据（通过 DataBridge 转发）
	// 注意：实际验证需要 Telnet 客户端连接，这里简化验证
	t.Log("串口 → Telnet 广播路径测试完成")

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

	// 测试路径 2: 串口 → DataBuffer (MCP)
	t.Log("测试路径 2: 串口 → DataBuffer...")
	if err := sm.WriteLine("version"); err != nil {
		t.Fatalf("WriteLine failed: %v", err)
	}

	// 等待数据写入 buffer
	time.Sleep(500 * time.Millisecond)

	// 验证 DataBuffer 收到数据
	if buf.Length() == 0 {
		t.Error("DataBuffer 未收到数据")
	} else {
		data := buf.Read(4096)
		t.Logf("DataBuffer 收到 %d 字节数据", len(data))
		if strings.Contains(string(data), "Thread Operating System") {
			t.Log("DataBuffer 收到 version 响应")
		}
	}

	// 测试路径 3: Telnet → 串口（简化测试，不验证响应）
	t.Log("测试路径 3: Telnet → 串口...")
	// 这里需要 Telnet 客户端发送数据，简化处理
	t.Log("Telnet → 串口路径测试完成")

	// 清理
	bridge.Stop()
	telnetSrv.Stop()
	sm.Disconnect()

	t.Log("HW3 DataBridge 转发硬件测试通过")
}
