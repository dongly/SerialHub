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
}

func TestSerialConnect_InvalidPort(t *testing.T) {
	sm := newTestManager(t)

	result := ExecuteSerialConnect(sm, ConnectInput{Port: "INVALID_PORT_99999", BaudRate: 115200})
	if result.Success {
		t.Error("无效端口应该连接失败")
	}
}

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

func TestSerialWrite_NotConnected(t *testing.T) {
	sm := newTestManager(t)

	result := ExecuteSerialWrite(sm, WriteInput{Data: "test"})
	if result.Success {
		t.Error("未连接时写入应该失败")
	}
}

func TestSerialRead_EmptyBuffer(t *testing.T) {
	buf := buffer.NewDataBuffer()
	result := ExecuteSerialRead(buf, ReadInput{Timeout: 100})
	if !result.Success {
		t.Errorf("空缓冲区读取应该成功: %s", result.Message)
	}
}

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
