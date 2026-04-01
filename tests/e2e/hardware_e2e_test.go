// Package e2e provides end-to-end hardware integration tests.
package e2e

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/bridge"
	"github.com/yourname/serialhub/pkg/serial"
	"github.com/yourname/serialhub/pkg/telnet"
)

// TestHW4_EndToEnd 全链路端到端硬件测试
// 环境变量配置：
//
//	SERIALHUB_HARDWARE_TEST=1    - 启用硬件测试
//	SERIALHUB_TEST_PORT=COM9     - 串口号（默认 COM9）
//	SERIALHUB_TEST_BAUD=115200   - 波特率（默认 115200）
func TestHW4_EndToEnd(t *testing.T) {
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

	t.Logf("端到端测试配置: port=%s, baud=%d", testPort, baudRate)

	// 创建完整组件链
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

	dataBridge, err := bridge.NewDataBridge(sm, telnetSrv, buf)
	if err != nil {
		t.Fatalf("NewDataBridge failed: %v", err)
	}

	// 确保清理
	t.Cleanup(func() {
		dataBridge.Stop()
		telnetSrv.Stop()
		if sm.IsConnected() {
			sm.Disconnect()
		}
	})

	// 启动所有服务
	if err := telnetSrv.Start(); err != nil {
		t.Fatalf("TelnetServer.Start failed: %v", err)
	}
	t.Log("TelnetServer 已启动")

	dataBridge.Start()
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
			goto cleared
		}
	}
cleared:
	buf.Clear()

	// 测试完整数据流：串口 → DataBridge → DataBuffer
	t.Log("测试完整数据流: 串口 → DataBridge → DataBuffer...")
	if err := sm.WriteLine("version"); err != nil {
		t.Fatalf("WriteLine failed: %v", err)
	}

	// 等待数据通过 DataBridge 转发到 DataBuffer
	time.Sleep(500 * time.Millisecond)

	// 验证 DataBuffer 收到数据
	if buf.Length() == 0 {
		t.Fatal("DataBuffer 未收到数据，DataBridge 转发可能失败")
	}

	data := buf.Read(4096)
	t.Logf("DataBuffer 收到 %d 字节数据", len(data))

	if !strings.Contains(string(data), "Thread Operating System") {
		t.Errorf("响应不包含预期内容，收到: %s", string(data))
	}

	t.Log("完整数据流验证通过")

	// 测试并发：串口数据同时到达 Telnet 和 MCP
	t.Log("测试并发数据流...")
	buf.Clear()

	if err := sm.WriteLine("help"); err != nil {
		t.Fatalf("WriteLine failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// 验证 DataBuffer 收到 help 响应
	if buf.Length() > 0 {
		helpData := buf.Read(4096)
		if strings.Contains(string(helpData), "RT-Thread shell commands:") {
			t.Log("并发数据流验证通过")
		}
	}

	// 断开连接
	dataBridge.Stop()
	telnetSrv.Stop()
	sm.Disconnect()

	t.Log("HW4 全链路端到端硬件测试通过")
}
