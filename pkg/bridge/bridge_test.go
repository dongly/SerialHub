package bridge

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/yourname/serialhub/internal/buffer"
)

// --- Mock 实现 ---

type mockSerialReader struct {
	dataChan  chan []byte
	writeData []byte
	writeErr  error
	mu        sync.Mutex
}

func (m *mockSerialReader) DataChan() <-chan []byte { return m.dataChan }
func (m *mockSerialReader) Write(data []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.writeData = append(m.writeData, data...)
	return len(data), nil
}
func (m *mockSerialReader) getWriteData() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(m.writeData))
	copy(cp, m.writeData)
	return cp
}

type mockWsBroadcaster struct {
	dataChan      chan []byte
	broadcastData []byte
	clientCount   int
	mu            sync.Mutex
}

func (m *mockWsBroadcaster) DataChan() <-chan []byte { return m.dataChan }
func (m *mockWsBroadcaster) Broadcast(data []byte) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcastData = append(m.broadcastData, data...)
	return m.clientCount
}
func (m *mockWsBroadcaster) getBroadcastData() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(m.broadcastData))
	copy(cp, m.broadcastData)
	return cp
}

// --- 辅助函数 ---

func newTestBridge() (*DataBridge, *mockSerialReader, *mockWsBroadcaster, *buffer.DataBuffer) {
	serialMock := &mockSerialReader{dataChan: make(chan []byte, 10)}
	wsMock := &mockWsBroadcaster{dataChan: make(chan []byte, 10)}
	buf := buffer.NewDataBuffer(4096)
	b, _ := NewDataBridge(serialMock, wsMock, buf)
	return b, serialMock, wsMock, buf
}

// --- NewDataBridge 构造测试 ---

func TestNewDataBridge_ParameterValidation(t *testing.T) {
	serialMock := &mockSerialReader{dataChan: make(chan []byte)}
	wsMock := &mockWsBroadcaster{dataChan: make(chan []byte)}
	buf := buffer.NewDataBuffer()

	tests := []struct {
		name      string
		serialMgr SerialReader
		wsSrv     WebSocketBroadcaster
		mcpBuf    *buffer.DataBuffer
		wantErr   string
	}{
		{"serialMgr为nil", nil, wsMock, buf, "串口管理器不能为空"},
		{"wsSrv为nil", serialMock, nil, buf, "WebSocket 服务器不能为空"},
		{"mcpBuf为nil", serialMock, wsMock, nil, "MCP 缓冲区不能为空"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDataBridge(tt.serialMgr, tt.wsSrv, tt.mcpBuf)
			if err == nil {
				t.Fatalf("期望返回错误，但没有")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("错误消息 = %q, 期望 %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewDataBridge_NormalCreation(t *testing.T) {
	b, _, _, _ := newTestBridge()

	if b.serial == nil {
		t.Error("bridge.serial 不应为 nil")
	}
	if b.ws == nil {
		t.Error("bridge.ws 不应为 nil")
	}
	if b.mcpBuffer == nil {
		t.Error("bridge.mcpBuffer 不应为 nil")
	}
	if b.ctx == nil {
		t.Error("bridge.ctx 不应为 nil")
	}
	if b.cancel == nil {
		t.Error("bridge.cancel 不应为 nil")
	}
}

// --- Start/Stop 生命周期 ---

func TestDataBridge_StartStop(t *testing.T) {
	b, _, _, _ := newTestBridge()

	b.Start()
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		b.Stop()
		close(done)
	}()

	select {
	case <-done:
		// 正常退出
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() 阻塞超时")
	}
}

func TestDataBridge_StopReturnsNil(t *testing.T) {
	b, _, _, _ := newTestBridge()
	b.Start()
	time.Sleep(30 * time.Millisecond)

	err := b.Stop()
	if err != nil {
		t.Errorf("Stop() 应返回 nil, 实际: %v", err)
	}
}

// --- 串口 → Telnet + MCP 转发 ---

func TestForwardLoop_SerialToTelnetAndMCP(t *testing.T) {
	b, serialMock, wsMock, mcpBuf := newTestBridge()
	wsMock.clientCount = 2 // 模拟有 Telnet 客户端

	b.Start()
	defer b.Stop()

	testData := []byte("hello MCU")
	serialMock.dataChan <- testData

	time.Sleep(100 * time.Millisecond)

	// 验证 Telnet 广播
	got := wsMock.getBroadcastData()
	if string(got) != string(testData) {
		t.Errorf("Telnet 广播数据 = %q, 期望 %q", got, testData)
	}

	// 验证 MCP 缓冲区
	peeked := mcpBuf.Peek(4096)
	if string(peeked) != string(testData) {
		t.Errorf("MCP 缓冲区数据 = %q, 期望 %q", peeked, testData)
	}
}

func TestForwardLoop_SerialDataNoTelnetClient(t *testing.T) {
	b, serialMock, wsMock, mcpBuf := newTestBridge()
	wsMock.clientCount = 0 // 无 Telnet 客户端

	b.Start()
	defer b.Stop()

	testData := []byte("no clients")
	serialMock.dataChan <- testData

	time.Sleep(100 * time.Millisecond)

	// 即使 clientCount=0, Broadcast 仍被调用（只是返回 0）
	got := wsMock.getBroadcastData()
	if string(got) != string(testData) {
		t.Errorf("Telnet 广播数据 = %q, 期望 %q", got, testData)
	}

	// MCP 缓冲区仍然有数据
	peeked := mcpBuf.Peek(4096)
	if string(peeked) != string(testData) {
		t.Errorf("MCP 缓冲区数据 = %q, 期望 %q", peeked, testData)
	}
}

func TestForwardLoop_SerialMultipleDataForward(t *testing.T) {
	b, serialMock, wsMock, mcpBuf := newTestBridge()
	wsMock.clientCount = 1

	b.Start()
	defer b.Stop()

	msg1 := []byte("AAA")
	msg2 := []byte("BBB")
	serialMock.dataChan <- msg1
	serialMock.dataChan <- msg2

	time.Sleep(150 * time.Millisecond)

	// 验证 Telnet 广播合并
	got := wsMock.getBroadcastData()
	expected := "AAABBB"
	if string(got) != expected {
		t.Errorf("Telnet 广播数据 = %q, 期望 %q", got, expected)
	}

	// MCP 缓冲区也应合并
	peeked := mcpBuf.Peek(4096)
	if string(peeked) != expected {
		t.Errorf("MCP 缓冲区数据 = %q, 期望 %q", peeked, expected)
	}
}

func TestForwardLoop_SerialEmptyDataIgnored(t *testing.T) {
	b, serialMock, wsMock, mcpBuf := newTestBridge()
	wsMock.clientCount = 1

	b.Start()
	defer b.Stop()

	// 发送空数据（len == 0）
	serialMock.dataChan <- []byte{}

	time.Sleep(100 * time.Millisecond)

	// 空数据不应被转发
	got := wsMock.getBroadcastData()
	if len(got) != 0 {
		t.Errorf("空数据不应转发到 Telnet, 但得到 %q", got)
	}

	peeked := mcpBuf.Peek(4096)
	if len(peeked) != 0 {
		t.Errorf("空数据不应写入 MCP 缓冲区, 但得到 %q", peeked)
	}
}

// --- Telnet → 串口 转发 ---

func TestForwardLoop_TelnetToSerial(t *testing.T) {
	b, serialMock, wsMock, _ := newTestBridge()

	b.Start()
	defer b.Stop()

	testData := []byte("AT+RST\r\n")
	wsMock.dataChan <- testData

	time.Sleep(100 * time.Millisecond)

	got := serialMock.getWriteData()
	if string(got) != string(testData) {
		t.Errorf("Serial 写入数据 = %q, 期望 %q", got, testData)
	}
}

func TestForwardLoop_TelnetToSerialWriteError(t *testing.T) {
	b, serialMock, wsMock, _ := newTestBridge()
	serialMock.writeErr = errors.New("写入失败")

	b.Start()
	defer b.Stop()

	testData := []byte("will fail")
	wsMock.dataChan <- testData

	time.Sleep(100 * time.Millisecond)

	// 写入出错不应 panic, bridge 应继续运行
	// 验证 bridge 仍然存活：修正 writeErr, 发新数据应能成功
	serialMock.writeErr = nil
	wsMock.dataChan <- []byte("ok")

	time.Sleep(100 * time.Millisecond)

	got := serialMock.getWriteData()
	if string(got) != "ok" {
		t.Errorf("修正错误后 Serial 写入数据 = %q, 期望 %q", got, "ok")
	}
}

func TestForwardLoop_TelnetEmptyDataIgnored(t *testing.T) {
	b, serialMock, wsMock, _ := newTestBridge()

	b.Start()
	defer b.Stop()

	// 发送空数据
	wsMock.dataChan <- []byte{}

	time.Sleep(100 * time.Millisecond)

	got := serialMock.getWriteData()
	if len(got) != 0 {
		t.Errorf("空数据不应写入串口, 但得到 %q", got)
	}
}

// --- Channel 关闭 ---

func TestForwardLoop_SerialChannelClosed(t *testing.T) {
	serialMock := &mockSerialReader{dataChan: make(chan []byte)}
	wsMock := &mockWsBroadcaster{dataChan: make(chan []byte, 10)}
	buf := buffer.NewDataBuffer(4096)

	b, _ := NewDataBridge(serialMock, wsMock, buf)
	b.Start()

	// 关闭串口 channel → forwardLoop 应退出
	close(serialMock.dataChan)

	done := make(chan struct{})
	go func() {
		b.Stop()
		close(done)
	}()

	select {
	case <-done:
		// 正常退出
	case <-time.After(2 * time.Second):
		t.Fatal("串口 channel 关闭后 Stop 阻塞超时")
	}
}

func TestForwardLoop_TelnetChannelClosed(t *testing.T) {
	serialMock := &mockSerialReader{dataChan: make(chan []byte, 10)}
	wsMock := &mockWsBroadcaster{dataChan: make(chan []byte)}
	buf := buffer.NewDataBuffer(4096)

	b, _ := NewDataBridge(serialMock, wsMock, buf)
	b.Start()

	// 关闭 telnet channel → forwardLoop 应退出
	close(wsMock.dataChan)

	done := make(chan struct{})
	go func() {
		b.Stop()
		close(done)
	}()

	select {
	case <-done:
		// 正常退出
	case <-time.After(2 * time.Second):
		t.Fatal("Telnet channel 关闭后 Stop 阻塞超时")
	}
}

// --- Context 取消 ---

func TestForwardLoop_ContextCancel(t *testing.T) {
	b, _, _, _ := newTestBridge()

	b.Start()

	done := make(chan struct{})
	go func() {
		b.Stop()
		close(done)
	}()

	select {
	case <-done:
		// 正常退出
	case <-time.After(2 * time.Second):
		t.Fatal("Context 取消后 Stop 阻塞超时")
	}
}

// --- 双向同时转发 ---

func TestForwardLoop_BidirectionalSimultaneousForward(t *testing.T) {
	b, serialMock, wsMock, mcpBuf := newTestBridge()
	wsMock.clientCount = 1

	b.Start()
	defer b.Stop()

	// 同时发送串口和 Telnet 数据
	serialData := []byte("from-serial")
	telnetData := []byte("from-telnet")

	serialMock.dataChan <- serialData
	wsMock.dataChan <- telnetData

	time.Sleep(150 * time.Millisecond)

	// 串口数据 → Telnet + MCP
	telnetGot := wsMock.getBroadcastData()
	if string(telnetGot) != string(serialData) {
		t.Errorf("Telnet 广播数据 = %q, 期望 %q", telnetGot, serialData)
	}
	mcpGot := mcpBuf.Peek(4096)
	if string(mcpGot) != string(serialData) {
		t.Errorf("MCP 缓冲区数据 = %q, 期望 %q", mcpGot, serialData)
	}

	// Telnet 数据 → 串口
	serialGot := serialMock.getWriteData()
	if string(serialGot) != string(telnetData) {
		t.Errorf("Serial 写入数据 = %q, 期望 %q", serialGot, telnetData)
	}
}

// --- 并发安全 ---

func TestDataBridge_MultipleStartStop(t *testing.T) {
	b, serialMock, _, _ := newTestBridge()

	b.Start()
	serialMock.dataChan <- []byte("round1")
	time.Sleep(50 * time.Millisecond)
	b.Stop()

	b2, serialMock2, wsMock2, _ := newTestBridge()
	wsMock2.clientCount = 1
	b2.Start()
	serialMock2.dataChan <- []byte("round2")
	time.Sleep(50 * time.Millisecond)
	b2.Stop()

	// 验证第二次的数据不与第一次混淆
	got := wsMock2.getBroadcastData()
	if string(got) != "round2" {
		t.Errorf("第二轮 Telnet 广播 = %q, 期望 %q", got, "round2")
	}
}

func TestDataBridge_LargeDataForward(t *testing.T) {
	b, serialMock, wsMock, mcpBuf := newTestBridge()
	wsMock.clientCount = 1

	b.Start()
	defer b.Stop()

	// 发送多批数据
	var expected string
	for i := 0; i < 20; i++ {
		msg := fmt.Sprintf("msg-%02d|", i)
		expected += msg
		serialMock.dataChan <- []byte(msg)
	}

	time.Sleep(300 * time.Millisecond)

	got := wsMock.getBroadcastData()
	if string(got) != expected {
		t.Errorf("大数据量 Telnet 广播长度=%d 期望=%d", len(got), len(expected))
	}

	mcpGot := mcpBuf.Peek(8192)
	if string(mcpGot) != expected {
		t.Errorf("大数据量 MCP 缓冲区长度=%d 期望=%d", len(mcpGot), len(expected))
	}
}

// --- forwardSerialToBoth / forwardTelnetToSerial 直接测试 ---

func TestForwardSerialToBoth_NormalForward(t *testing.T) {
	serialMock := &mockSerialReader{dataChan: make(chan []byte, 1)}
	wsMock := &mockWsBroadcaster{dataChan: make(chan []byte, 1), clientCount: 3}
	buf := buffer.NewDataBuffer(4096)

	b, _ := NewDataBridge(serialMock, wsMock, buf)

	data := []byte("test-data")
	b.forwardSerialToBoth(data)

	// 验证 Telnet 广播
	got := wsMock.getBroadcastData()
	if string(got) != "test-data" {
		t.Errorf("Telnet 广播 = %q, 期望 %q", got, "test-data")
	}

	// 验证 MCP 缓冲区
	peeked := buf.Peek(4096)
	if string(peeked) != "test-data" {
		t.Errorf("MCP 缓冲区 = %q, 期望 %q", peeked, "test-data")
	}
}

func TestForwardTelnetToSerial_NormalForward(t *testing.T) {
	serialMock := &mockSerialReader{dataChan: make(chan []byte, 1)}
	wsMock := &mockWsBroadcaster{dataChan: make(chan []byte, 1)}
	buf := buffer.NewDataBuffer(4096)

	b, _ := NewDataBridge(serialMock, wsMock, buf)

	data := []byte("telnet-cmd")
	b.forwardWsToSerial(data)

	got := serialMock.getWriteData()
	if string(got) != "telnet-cmd" {
		t.Errorf("Serial 写入 = %q, 期望 %q", got, "telnet-cmd")
	}
}

func TestForwardTelnetToSerial_WriteError(t *testing.T) {
	serialMock := &mockSerialReader{
		dataChan: make(chan []byte, 1),
		writeErr: errors.New("串口写入失败"),
	}
	wsMock := &mockWsBroadcaster{dataChan: make(chan []byte, 1)}
	buf := buffer.NewDataBuffer(4096)

	b, _ := NewDataBridge(serialMock, wsMock, buf)

	// 不应 panic
	b.forwardWsToSerial([]byte("data"))

	got := serialMock.getWriteData()
	if len(got) != 0 {
		t.Errorf("写入错误时不应有数据记录, 但得到 %q", got)
	}
}

// --- convertLFToCRLF 单元测试 ---

func TestConvertLFToCRLF(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"空数据", "", ""},
		{"无换行符", "hello", "hello"},
		{"纯LF", "hello\nworld", "hello\r\nworld"},
		{"已有CRLF", "hello\r\nworld", "hello\r\nworld"},
		{"混合CRLF和LF", "a\r\nb\nc", "a\r\nb\r\nc"},
		{"LF开头", "\nhello", "\r\nhello"},
		{"连续LF", "a\n\nb", "a\r\n\r\nb"},
		{"仅有LF", "\n", "\r\n"},
		{"中文UTF8含LF", "你好\n世界", "你好\r\n世界"},
		{"中文UTF8含CRLF", "你好\r\n世界", "你好\r\n世界"},
		{"中文UTF8混合", "连接成功\r\n开始\n测试", "连接成功\r\n开始\r\n测试"},
		{"末尾LF", "hello\n", "hello\r\n"},
		{"末尾CRLF", "hello\r\n", "hello\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertLFToCRLF([]byte(tt.input))
			if string(got) != tt.want {
				t.Errorf("convertLFToCRLF(%q) = %q, 期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- 事件数据类型 ---

func TestEventDataTypes(t *testing.T) {
	t.Run("SerialDataEvent", func(t *testing.T) {
		e := SerialDataEvent{Data: []byte("serial data")}
		if string(e.Data) != "serial data" {
			t.Errorf("Data = %q, 期望 %q", e.Data, "serial data")
		}
	})

	t.Run("WebSocketDataEvent", func(t *testing.T) {
		e := WebSocketDataEvent{Data: []byte("websocket data")}
		if string(e.Data) != "websocket data" {
			t.Errorf("Data = %q, 期望 %q", e.Data, "websocket data")
		}
	})

	t.Run("ForwardEvent", func(t *testing.T) {
		e := ForwardEvent{Source: "serial", Data: []byte("fwd")}
		if e.Source != "serial" {
			t.Errorf("Source = %q, 期望 %q", e.Source, "serial")
		}
		if string(e.Data) != "fwd" {
			t.Errorf("Data = %q, 期望 %q", e.Data, "fwd")
		}
	})

	t.Run("ForwardEvent_telnet源", func(t *testing.T) {
		e := ForwardEvent{Source: "telnet", Data: nil}
		if e.Source != "telnet" {
			t.Errorf("Source = %q, 期望 %q", e.Source, "telnet")
		}
		if e.Data != nil {
			t.Errorf("Data 应为 nil, 但得到 %q", e.Data)
		}
	})
}
