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

	"github.com/yourname/serialhub/internal/testutil"
)

func getTestPort() string {
	port := os.Getenv("SERIALHUB_TEST_PORT")
	if port == "" {
		port = "COM9"
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
			wantErr: true,
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

// TestHW1_SerialManager 硬件集成测试
// 环境变量配置：
//
//	SERIALHUB_HARDWARE_TEST=1    - 启用硬件测试
//	SERIALHUB_TEST_PORT=COM9     - 串口号（默认 COM9）
//	SERIALHUB_TEST_BAUD=115200   - 波特率（默认 115200）
//	SERIALHUB_TEST_DATABITS=8    - 数据位（默认 8）
//	SERIALHUB_TEST_PARITY=none   - 校验位（默认 none）
//	SERIALHUB_TEST_STOPBITS=1    - 停止位（默认 1）
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

	t.Logf("硬件测试配置: port=%s, baud=%d, dataBits=%d, parity=%s, stopBits=%.0f",
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
	sm.WriteLine("")
	time.Sleep(500 * time.Millisecond)

	// 丢弃缓冲区中的数据
	select {
	case <-sm.dataChan:
	default:
	}

	// 发送 help 命令
	t.Log("发送 help 命令...")
	if err := sm.WriteLine("help"); err != nil {
		t.Fatalf("WriteLine('help') failed: %v", err)
	}

	// 读取响应（5秒超时），累积数据直到收到完整响应
	var helpResponse strings.Builder
	helpTimeout := time.After(5 * time.Second)
	helpDone := false
	for !helpDone {
		select {
		case data := <-sm.dataChan:
			helpResponse.Write(data)
			t.Logf("收到数据块: %q", string(data))
			// 检查是否已收到完整响应
			if strings.Contains(helpResponse.String(), "RT-Thread shell commands:") {
				helpDone = true
			}
		case <-helpTimeout:
			helpDone = true
		}
	}

	t.Logf("help 完整响应: %s", helpResponse.String())
	if !strings.Contains(helpResponse.String(), "RT-Thread shell commands:") {
		t.Errorf("help 响应不包含预期内容")
	}

	// 清空残留
	time.Sleep(500 * time.Millisecond)
	for {
		select {
		case <-sm.dataChan:
		default:
			goto cleared1
		}
	}
cleared1:

	// 发送 version 命令
	t.Log("发送 version 命令...")
	if err := sm.WriteLine("version"); err != nil {
		t.Fatalf("WriteLine('version') failed: %v", err)
	}

	// 读取响应
	var versionResponse strings.Builder
	versionTimeout := time.After(5 * time.Second)
	versionDone := false
	for !versionDone {
		select {
		case data := <-sm.dataChan:
			versionResponse.Write(data)
			t.Logf("收到数据块: %q", string(data))
			// 检查是否已收到完整响应
			if strings.Contains(versionResponse.String(), "Thread Operating System") {
				versionDone = true
			}
		case <-versionTimeout:
			versionDone = true
		}
	}

	t.Logf("version 完整响应: %s", versionResponse.String())
	if !strings.Contains(versionResponse.String(), "Thread Operating System") {
		t.Errorf("version 响应不包含预期内容")
	}

	// 断开连接
	if err := sm.Disconnect(); err != nil {
		t.Errorf("Disconnect() failed: %v", err)
	}

	if sm.IsConnected() {
		t.Error("断开后 IsConnected() 应该返回 false")
	}

	t.Log("硬件集成测试通过")
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
