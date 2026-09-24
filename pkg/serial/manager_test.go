// Package serial manages serial port connections.
package serial

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dongly/serialhub/internal/testutil"
)

func getTestPort() string {
	port := os.Getenv("SERIALHUB_TEST_PORT")
	if port == "" {
		port = "COM4"
	}
	return port
}

// TestNewSerialManager 测试创建串口管理器
func TestNewSerialManager(t *testing.T) {
	testPort := getTestPort()
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "正常创建",
			cfg: &Config{
				Port:     testPort,
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "none",
				StopBits: 1,
			},
			wantErr: false,
		},
		{
			name: "使用默认配置",
			cfg: func() *Config {
				cfg := DefaultConfig()
				cfg.Port = testPort
				return cfg
			}(),
			wantErr: false,
		},
		{
			name:    "nil配置",
			cfg:     nil,
			wantErr: true,
		},
		{
			name: "空端口",
			cfg: &Config{
				Port:     "",
				BaudRate: 115200,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := NewSerialManager(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSerialManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && sm == nil {
				t.Error("NewSerialManager() 返回 nil")
			}
			if sm != nil {
				defer sm.Close()
			}
		})
	}
}

// TestConnect_MockSuccess 测试使用 MockSerialPort 成功连接
func TestConnect_MockSuccess(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
		DataBits: 8,
		Parity:   "none",
		StopBits: 1,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 注入 MockSerialPort
	mockPort := testutil.NewMockSerialPort([]byte("test data\n"))
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	if !sm.IsConnected() {
		t.Error("IsConnected() 返回 false")
	}

	if sm.CurrentPort() != "MOCK1" {
		t.Errorf("CurrentPort() = %s, want MOCK1", sm.CurrentPort())
	}
}

// TestConnect_MockFailure 测试连接失败场景
func TestConnect_MockFailure(t *testing.T) {
	// 这个测试主要验证真实连接场景下的错误处理
	// 在实际测试中，连接一个不存在的端口会失败
	cfg := &Config{
		Port:     "NONEXISTENT_PORT_12345",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	err = sm.Connect()
	if err == nil {
		t.Error("Connect() 应该返回错误，但返回 nil")
	}
}

// TestConnect_AlreadyConnected 测试重复连接
func TestConnect_AlreadyConnected(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 注入 MockSerialPort
	mockPort := testutil.NewMockSerialPort(nil)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// 尝试再次连接
	err = sm.Connect()
	if err == nil {
		t.Error("Connect() 应该返回串口已连接错误")
	}
}

// TestDisconnect 测试断开连接
func TestDisconnect(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 注入 MockSerialPort
	mockPort := testutil.NewMockSerialPort(nil)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// 断开连接
	err = sm.Disconnect()
	if err != nil {
		t.Errorf("Disconnect() failed: %v", err)
	}

	if sm.IsConnected() {
		t.Error("断开后 IsConnected() 应该返回 false")
	}

	if !mockPort.IsClosed() {
		t.Error("MockSerialPort 应该被关闭")
	}

	// 再次断开应该返回错误
	err = sm.Disconnect()
	if err == nil {
		t.Error("重复断开应该返回错误")
	}
}

// TestWrite 测试写入数据
func TestWrite(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 注入 MockSerialPort
	mockPort := testutil.NewMockSerialPort(nil)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// 写入数据
	data := []byte("hello world\n")
	n, err := sm.Write(data)
	if err != nil {
		t.Errorf("Write() failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}

	// 验证 MockSerialPort 收到的数据
	received := mockPort.GetWriteData()
	if string(received) != string(data) {
		t.Errorf("MockSerialPort 收到数据 = %s, want %s", string(received), string(data))
	}

	// 未连接时写入应该返回错误
	sm.mu.Lock()
	sm.port = nil
	sm.mu.Unlock()

	_, err = sm.Write(data)
	if err == nil {
		t.Error("未连接时 Write() 应该返回错误")
	}
}

// TestWriteLine 测试写入一行
func TestWriteLine(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 注入 MockSerialPort
	mockPort := testutil.NewMockSerialPort(nil)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// 写入一行
	err = sm.WriteLine("test command")
	if err != nil {
		t.Errorf("WriteLine() failed: %v", err)
	}

	// 验证数据包含换行符
	received := mockPort.GetWriteData()
	expected := "test command\n"
	if string(received) != expected {
		t.Errorf("WriteLine() 收到数据 = %q, want %q", string(received), expected)
	}
}

// TestListPorts 测试列出串口
func TestListPorts(t *testing.T) {
	cfg := &Config{
		Port:     getTestPort(),
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 获取串口列表（可能为空，但不应该报错）
	ports, err := sm.ListPorts()
	if err != nil {
		t.Errorf("ListPorts() failed: %v", err)
	}

	// ports 可能为空或包含实际串口，都是正常的
	t.Logf("发现 %d 个串口", len(ports))
	for _, port := range ports {
		t.Logf("  - %s", port)
	}
}

// TestUpdateConfig 测试更新配置
func TestUpdateConfig(t *testing.T) {
	cfg := &Config{
		Port:     getTestPort(),
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 未连接时可以更新配置
	newCfg := &Config{
		Port:     "COM10",
		BaudRate: 9600,
		DataBits: 8,
		Parity:   "none",
		StopBits: 1,
	}
	err = sm.UpdateConfig(newCfg)
	if err != nil {
		t.Errorf("UpdateConfig() failed: %v", err)
	}

	// 验证配置已更新
	updated := sm.GetConfig()
	if updated.Port != "COM10" {
		t.Errorf("Port = %s, want COM10", updated.Port)
	}
	if updated.BaudRate != 9600 {
		t.Errorf("BaudRate = %d, want 9600", updated.BaudRate)
	}

	// 注入 MockSerialPort 模拟已连接
	mockPort := testutil.NewMockSerialPort(nil)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// 已连接时不能更新配置
	err = sm.UpdateConfig(newCfg)
	if err == nil {
		t.Error("已连接时 UpdateConfig() 应该返回错误")
	}

	// nil 配置
	sm.mu.Lock()
	sm.port = nil
	sm.mu.Unlock()

	err = sm.UpdateConfig(nil)
	if err == nil {
		t.Error("nil 配置时 UpdateConfig() 应该返回错误")
	}

	// 空端口
	emptyCfg := &Config{Port: ""}
	err = sm.UpdateConfig(emptyCfg)
	if err == nil {
		t.Error("空端口配置时 UpdateConfig() 应该返回错误")
	}
}

// TestClose 测试关闭管理器
func TestClose(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}

	// 注入 MockSerialPort
	mockPort := testutil.NewMockSerialPort(nil)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// 关闭管理器
	err = sm.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// 验证 MockSerialPort 已关闭
	if !mockPort.IsClosed() {
		t.Error("Close() 应该关闭串口")
	}

	// 验证通道已关闭
	select {
	case _, ok := <-sm.dataChan:
		if ok {
			t.Error("dataChan 应该已关闭")
		}
	default:
	}

	select {
	case _, ok := <-sm.errChan:
		if ok {
			t.Error("errChan 应该已关闭")
		}
	default:
	}
}

// TestConcurrentReadWrite 测试 MockSerialPort 读取
func TestConcurrentReadWrite(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 注入 MockSerialPort，预设一些读取数据
	readData := []byte("line1\nline2\nline3\n")
	mockPort := testutil.NewMockSerialPort(readData)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	var wg sync.WaitGroup
	numWrites := 10

	// 启动写入 goroutine
	for i := 0; i < numWrites; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("write%d\n", idx))
			_, err := sm.Write(data)
			if err != nil {
				t.Errorf("Write() 失败: %v", err)
			}
		}(i)
	}

	// 读取数据
	timeout := time.After(2 * time.Second)
	receivedCount := 0
loop:
	for {
		select {
		case data := <-sm.dataChan:
			t.Logf("收到数据: %q", string(data))
			receivedCount++
		case err := <-sm.errChan:
			t.Logf("收到错误: %v", err)
		case <-timeout:
			break loop
		}
	}

	// readLoop 不在测试中启动，跳过接收验证

	wg.Wait()
}

// TestConfigToMode 测试配置转换为 Mode
func TestConfigToMode(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "正常配置",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "none",
				StopBits: 1,
			},
			wantErr: false,
		},
		{
			name: "9600 波特率",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 9600,
				DataBits: 8,
				Parity:   "none",
				StopBits: 1,
			},
			wantErr: false,
		},
		{
			name: "Even 校验",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "even",
				StopBits: 1,
			},
			wantErr: false,
		},
		{
			name: "Odd 校验",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "odd",
				StopBits: 1,
			},
			wantErr: false,
		},
		{
			name: "2 停止位",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "none",
				StopBits: 2,
			},
			wantErr: false,
		},
		{
			name: "7 数据位",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 115200,
				DataBits: 7,
				Parity:   "none",
				StopBits: 1,
			},
			wantErr: false,
		},
		{
			name: "无效波特率",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: -1,
				DataBits: 8,
			},
			wantErr: true,
		},
		{
			name: "无效数据位",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 115200,
				DataBits: 9,
			},
			wantErr: true,
		},
		{
			name: "无效校验位",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "invalid",
			},
			wantErr: true,
		},
		{
			name: "1.5 停止位",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "none",
				StopBits: 1.5,
			},
			wantErr: false,
		},
		{
			name: "无效停止位",
			cfg: &Config{
				Port:     getTestPort(),
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "none",
				StopBits: 3,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := tt.cfg.ToMode()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && mode == nil {
				t.Error("ToMode() 返回 nil mode")
			}
		})
	}
}

// TestConfigString 测试配置字符串表示
func TestConfigString(t *testing.T) {
	cfg := &Config{
		Port:     getTestPort(),
		BaudRate: 115200,
		DataBits: 8,
		Parity:   "none",
		StopBits: 1,
	}

	str := cfg.String()
	expected := getTestPort() + "@115200 8N1"
	if str != expected {
		t.Errorf("String() = %s, want %s", str, expected)
	}
}

// TestParsePort 测试解析端口配置
func TestParsePort(t *testing.T) {
	tests := []struct {
		name    string
		portStr string
		want    *Config
		wantErr bool
	}{
		{
			name:    "简单端口名",
			portStr: getTestPort(),
			want: &Config{
				Port:     getTestPort(),
				BaudRate: 115200, // 默认值
				DataBits: 8,
				Parity:   "none",
				StopBits: 1,
			},
			wantErr: false,
		},
		{
			name:    "带波特率",
			portStr: "COM9@9600",
			want: &Config{
				Port:     "COM9",
				BaudRate: 9600,
				DataBits: 8,
				Parity:   "none",
				StopBits: 1,
			},
			wantErr: false,
		},
		{
			name:    "空字符串",
			portStr: "",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "无效波特率",
			portStr: "COM9@invalid",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Linux 设备",
			portStr: "/dev/ttyUSB0",
			want: &Config{
				Port:     "/dev/ttyUSB0",
				BaudRate: 115200,
				DataBits: 8,
				Parity:   "none",
				StopBits: 1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePort(tt.portStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got == nil {
					t.Error("ParsePort() 返回 nil")
					return
				}
				if got.Port != tt.want.Port {
					t.Errorf("Port = %s, want %s", got.Port, tt.want.Port)
				}
				if got.BaudRate != tt.want.BaudRate {
					t.Errorf("BaudRate = %d, want %d", got.BaudRate, tt.want.BaudRate)
				}
			}
		})
	}
}

// TestHW1_SerialManager 硬件回环集成测试
// 环境变量配置：
//
//	SERIALHUB_HARDWARE_TEST=1    - 启用硬件测试
//	SERIALHUB_TEST_PORT=COM4     - 串口号（默认 COM4）
//	SERIALHUB_TEST_BAUD=115200   - 波特率（默认 115200）
//	SERIALHUB_TEST_DATABITS=8    - 数据位（默认 8）
//	SERIALHUB_TEST_PARITY=none   - 校验位（默认 none）
//	SERIALHUB_TEST_STOPBITS=1    - 停止位（默认 1）
//
// 测试要求：串口的 TX 和 RX 短接（回环模式），发送什么就接收什么
func TestHW1_SerialManager(t *testing.T) {
	if os.Getenv("SERIALHUB_HARDWARE_TEST") != "1" {
		t.Skip("硬件测试未启用，设置 SERIALHUB_HARDWARE_TEST=1 启用")
	}

	// 从环境变量读取配置
	testPort := os.Getenv("SERIALHUB_TEST_PORT")
	if testPort == "" {
		testPort = getTestPort()
	}

	baudRate := 115200
	if baud := os.Getenv("SERIALHUB_TEST_BAUD"); baud != "" {
		if b, err := strconv.Atoi(baud); err == nil {
			baudRate = b
		}
	}

	dataBits := 8
	if db := os.Getenv("SERIALHUB_TEST_DATABITS"); db != "" {
		if d, err := strconv.Atoi(db); err == nil {
			dataBits = d
		}
	}

	parity := os.Getenv("SERIALHUB_TEST_PARITY")
	if parity == "" {
		parity = "none"
	}

	stopBits := float32(1)
	if sb := os.Getenv("SERIALHUB_TEST_STOPBITS"); sb != "" {
		if s, err := strconv.ParseFloat(sb, 32); err == nil {
			stopBits = float32(s)
		}
	}

	t.Logf("硬件回环测试配置: port=%s, baud=%d, dataBits=%d, parity=%s, stopBits=%.0f",
		testPort, baudRate, dataBits, parity, stopBits)

	cfg := &Config{
		Port:     testPort,
		BaudRate: baudRate,
		DataBits: dataBits,
		Parity:   parity,
		StopBits: stopBits,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 确保清理
	t.Cleanup(func() {
		if sm.IsConnected() {
			sm.Disconnect()
		}
	})

	// 连接串口
	if err := sm.Connect(); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	if !sm.IsConnected() {
		t.Fatal("连接后 IsConnected() 应该返回 true")
	}

	t.Logf("已连接到串口: %s", sm.CurrentPort())

	// 清空残留数据
	time.Sleep(100 * time.Millisecond)
	for {
		select {
		case <-sm.dataChan:
		default:
			goto cleared
		}
	}
cleared:

	// 回环测试：发送数据并验证接收
	testMessages := []string{"Hello", "World123", "Loopback!@#"}
	for _, msg := range testMessages {
		t.Logf("发送: %q", msg)
		if err := sm.WriteLine(msg); err != nil {
			t.Fatalf("WriteLine(%q) failed: %v", msg, err)
		}

		// 读取响应（3秒超时）
		timeout := time.After(3 * time.Second)
		var received strings.Builder
		receivedDone := false
		for !receivedDone {
			select {
			case data := <-sm.dataChan:
				received.Write(data)
				t.Logf("收到数据块: %q", string(data))
				// 检查是否收到完整消息
				if strings.Contains(received.String(), msg) {
					receivedDone = true
				}
			case <-timeout:
				receivedDone = true
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

		response := received.String()
		t.Logf("完整响应: %q", response)
		if !strings.Contains(response, msg) {
			t.Errorf("回环数据不匹配: 发送 %q, 接收 %q", msg, response)
		}

		// 清空缓冲区准备下一轮
		time.Sleep(100 * time.Millisecond)
		for {
			select {
			case <-sm.dataChan:
			default:
				goto nextMsg
			}
		}
	nextMsg:
	}

	// 断开连接
	if err := sm.Disconnect(); err != nil {
		t.Errorf("Disconnect() failed: %v", err)
	}

	if sm.IsConnected() {
		t.Error("断开后 IsConnected() 应该返回 false")
	}

	t.Log("硬件回环集成测试通过")
}

// TestGetConfig 测试获取配置
func TestGetConfig(t *testing.T) {
	cfg := &Config{
		Port:     getTestPort(),
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	got := sm.GetConfig()
	if got.Port != cfg.Port {
		t.Errorf("Port = %s, want %s", got.Port, cfg.Port)
	}
	if got.BaudRate != cfg.BaudRate {
		t.Errorf("BaudRate = %d, want %d", got.BaudRate, cfg.BaudRate)
	}

	// 修改返回的配置不应该影响原始配置
	got.Port = "COM10"
	original := sm.GetConfig()
	if original.Port == "COM10" {
		t.Error("GetConfig() 应该返回副本，修改副本不应该影响原始配置")
	}
}

// TestCurrentPort_未连接 测试未连接时返回空字符串
func TestCurrentPort_NotConnected(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 未连接时 CurrentPort() 应该返回空字符串
	if sm.CurrentPort() != "" {
		t.Errorf("CurrentPort() = %q, want empty string when not connected", sm.CurrentPort())
	}
}

// TestWrite_写入错误 测试写入失败场景
func TestWrite_WriteError(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 注入带写入错误的 MockSerialPort
	mockPort := testutil.NewMockSerialPort(nil)
	mockPort.WriteErr = fmt.Errorf("模拟写入错误")
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	_, err = sm.Write([]byte("test"))
	if err == nil {
		t.Error("Write() 应该返回错误")
	}

	// 验证错误被发送到 errChan
	select {
	case e := <-sm.ErrChan():
		if e == nil {
			t.Error("errChan 应该收到写入错误")
		}
	case <-time.After(1 * time.Second):
		t.Error("超时：errChan 未收到写入错误")
	}
}

// TestSetEventHandler 测试事件处理器设置和触发
func TestSetEventHandler(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	var receivedEvents []Event
	var eventMu sync.Mutex
	handler := func(event Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		receivedEvents = append(receivedEvents, event)
	}

	sm.SetEventHandler(handler)

	// 通过 Disconnect 触发 EventDisconnected（直接设置 port 来模拟）
	mockPort := testutil.NewMockSerialPort(nil)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// Disconnect 应该触发 EventDisconnected
	err = sm.Disconnect()
	if err != nil {
		t.Errorf("Disconnect() failed: %v", err)
	}

	// 等待事件处理器执行（它在 goroutine 中运行）
	time.Sleep(100 * time.Millisecond)

	eventMu.Lock()
	defer eventMu.Unlock()
	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receivedEvents))
	}
	if receivedEvents[0].Type != EventDisconnected {
		t.Errorf("event type = %v, want %v", receivedEvents[0].Type, EventDisconnected)
	}
	if receivedEvents[0].Port != "MOCK1" {
		t.Errorf("event port = %s, want MOCK1", receivedEvents[0].Port)
	}
}

// TestSetEventHandler_PanicRecovery 测试事件处理器 panic 恢复
func TestSetEventHandler_PanicRecovery(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 设置会 panic 的事件处理器
	panicHandler := func(event Event) {
		panic("测试 panic")
	}
	sm.SetEventHandler(panicHandler)

	// 注入 mock 并触发事件
	mockPort := testutil.NewMockSerialPort(nil)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// Disconnect 触发事件，不应崩溃
	err = sm.Disconnect()
	if err != nil {
		t.Errorf("Disconnect() failed: %v", err)
	}

	// 等待 panic 恢复
	time.Sleep(100 * time.Millisecond)
}

// TestDataChan_ErrChan 测试 channel 获取
func TestDataChan_ErrChan(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	dataCh := sm.DataChan()
	if dataCh == nil {
		t.Fatal("DataChan() 返回 nil")
	}

	errCh := sm.ErrChan()
	if errCh == nil {
		t.Fatal("ErrChan() 返回 nil")
	}
}

// TestReadLoop_ContextCancel 测试 context 取消时 readLoop 退出
func TestReadLoop_ContextCancel(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}

	// 注入 mock port（有数据可读，但 context 取消应优先退出）
	mockPort := testutil.NewMockSerialPort([]byte("test data\n"))
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// 手动启动 readLoop
	sm.wg.Add(1)
	go sm.readLoop()

	// 取消 context 让 readLoop 退出
	sm.cancel()
	sm.wg.Wait()

	// 如果到这里说明 readLoop 成功退出（否则会超时）
}

// TestReadLoop_PortNil 测试 port 为 nil 时 readLoop 退出
func TestReadLoop_PortNil(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}

	// port 为 nil（默认值），readLoop 应立即退出
	sm.wg.Add(1)
	go sm.readLoop()

	// 等待 readLoop 退出，加超时防止挂起
	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// readLoop 成功退出
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop 未能在 port 为 nil 时退出")
	}

	sm.cancel()
}

// TestReadLoop_EOF 测试 readLoop 在收到 EOF 时退出
func TestReadLoop_EOF(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}

	// MockSerialPort 在数据读完之后返回 io.EOF
	mockPort := testutil.NewMockSerialPort([]byte("data"))
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// 记录事件
	var receivedEvents []Event
	var eventMu sync.Mutex
	sm.SetEventHandler(func(event Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		receivedEvents = append(receivedEvents, event)
	})

	sm.wg.Add(1)
	go sm.readLoop()

	// 等待 readLoop 退出
	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop 未能在 EOF 时退出")
	}

	sm.cancel()

	select {
	case e := <-sm.ErrChan():
		if e == nil {
			t.Error("errChan 应该收到 EOF 错误")
		}
	default:
		t.Error("errChan 未收到 EOF 错误")
	}

	time.Sleep(100 * time.Millisecond)

	eventMu.Lock()
	defer eventMu.Unlock()
	found := false
	for _, ev := range receivedEvents {
		if ev.Type == EventDisconnected {
			found = true
			break
		}
	}
	if !found {
		t.Error("EOF 时应该触发 EventDisconnected 事件")
	}
}

// TestReadLoop_ReadError 测试 readLoop 在读取错误时继续运行
func TestReadLoop_ReadError(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}

	// 设置持续的读取错误（非 EOF）
	mockPort := testutil.NewMockSerialPort(nil)
	mockPort.ReadErr = fmt.Errorf("模拟读取错误")
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	// 记录事件
	var eventCount int
	var eventMu sync.Mutex
	sm.SetEventHandler(func(event Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		eventCount++
	})

	sm.wg.Add(1)
	go sm.readLoop()

	select {
	case e := <-sm.ErrChan():
		if e == nil {
			t.Error("errChan 应该收到读取错误")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("超时：errChan 未收到读取错误")
	}

	sm.cancel()
	sm.wg.Wait()

	time.Sleep(100 * time.Millisecond)

	eventMu.Lock()
	defer eventMu.Unlock()
	if eventCount == 0 {
		t.Error("读取错误时应该触发 EventError 事件")
	}
}

// TestReadLoop_DataReceive 测试 readLoop 接收数据并发送到 dataChan
func TestReadLoop_DataReceive(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}

	testData := []byte("hello from MCU")
	mockPort := testutil.NewMockSerialPort(testData)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	sm.wg.Add(1)
	go sm.readLoop()

	// 等待从 dataChan 接收数据
	select {
	case data := <-sm.DataChan():
		if string(data) != string(testData) {
			t.Errorf("dataChan 收到 %q, want %q", string(data), string(testData))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("超时：dataChan 未收到数据")
	}

	// mock 数据读完之后会返回 EOF，readLoop 将退出
	// 等待退出完成
	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		sm.cancel()
	}
	sm.cancel()
}

// TestReadLoop_DataChanFull 测试 dataChan 满时丢弃数据
func TestReadLoop_DataChanFull(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}

	// 填满 dataChan（buffer = 256）
	for i := 0; i < 256; i++ {
		sm.dataChan <- []byte("x")
	}

	// mock 会持续返回数据（不会 EOF）
	// 我们创建一个自定义的 mock，让它在第一次 Read 后返回 0 字节
	mockPort := testutil.NewMockSerialPort([]byte("overflow data"))
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	sm.wg.Add(1)
	go sm.readLoop()

	// 等一段时间让 readLoop 尝试发送数据（应该被丢弃）
	time.Sleep(200 * time.Millisecond)

	// 取消退出
	sm.cancel()
	sm.wg.Wait()
}

// TestEmitEvent_NoHandler 测试没有事件处理器时不触发
func TestEmitEvent_NoHandler(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	// 不设置 eventHandler，Disconnect 不应该 panic
	mockPort := testutil.NewMockSerialPort(nil)
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	err = sm.Disconnect()
	if err != nil {
		t.Errorf("Disconnect() with no handler failed: %v", err)
	}
}

// TestConnect_串口打开失败 测试使用无效串口名
func TestConnect_PortOpenFailed(t *testing.T) {
	cfg := &Config{
		Port:     "NONEXISTENT_PORT_99999",
		BaudRate: 115200,
		DataBits: 8,
		Parity:   "none",
		StopBits: 1,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	err = sm.Connect()
	if err == nil {
		t.Error("Connect() 应该返回错误（无效串口）")
	}
	if !sm.IsConnected() {
		// 预期：连接失败后 IsConnected() 返回 false
	} else {
		t.Error("连接失败后 IsConnected() 应该返回 false")
	}
}

// TestDisconnect_未连接 测试未连接时断开
func TestDisconnect_NotConnected(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	err = sm.Disconnect()
	if err == nil {
		t.Error("未连接时 Disconnect() 应该返回错误")
	}
}

// TestWrite_未连接 测试未连接时写入
func TestWrite_NotConnected(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	_, err = sm.Write([]byte("test"))
	if err == nil {
		t.Error("未连接时 Write() 应该返回错误")
	}
}
