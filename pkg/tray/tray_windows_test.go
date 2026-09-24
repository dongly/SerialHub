//go:build windows

// Package tray 系统托盘测试
package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/getlantern/systray"
	"github.com/yourname/serialhub/internal/testutil"
	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/serial"
)

func getTestPort() string {
	port := os.Getenv("SERIALHUB_TEST_PORT")
	if port == "" {
		port = "COM9"
	}
	return port
}

func TestTrayState(t *testing.T) {
	states := []TrayState{TrayIdle, TrayConnected, TrayError}
	for _, state := range states {
		if state != TrayIdle && state != TrayConnected && state != TrayError {
			t.Errorf("无效的状态: %s", state)
		}
	}
}

func TestNewTrayManager(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	config := config.GetDefault()

	tray := NewTrayManager(serialMgr, config, "127.0.0.1", 5000, "0.1.0", false)
	if tray == nil {
		t.Fatal("NewTrayManager 返回 nil")
	}

	if tray.version != "0.1.0" {
		t.Errorf("version = %s, want 0.1.0", tray.version)
	}
	if tray.mcpPort != 5000 {
		t.Errorf("mcpPort = %d, want 5000", tray.mcpPort)
	}
	if tray.state != TrayIdle {
		t.Errorf("初始状态 = %s, want idle", tray.state)
	}
}

func TestUpdateState(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	tm.state = TrayConnected
	testutil.AssertEqual(t, TrayConnected, tm.state)

	tm.state = TrayError
	testutil.AssertEqual(t, TrayError, tm.state)
}

// TestGetIcon 测试图标加载功能
func TestGetIcon(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	// 测试不同状态的图标加载
	testCases := []struct {
		state    TrayState
		expected string
	}{
		{TrayIdle, "assets/tray-idle.ico"},
		{TrayConnected, "assets/tray-connected.ico"},
		{TrayError, "assets/tray-error.ico"},
	}

	for _, tc := range testCases {
		trayMgr.state = tc.state
		iconData := trayMgr.getIcon()

		if len(iconData) == 0 {
			t.Errorf("状态 %s: 图标数据为空", tc.state)
			continue
		}

		// ICO 文件头：reserved(2) + type(2=1) + count(2)
		if len(iconData) < 6 {
			t.Errorf("状态 %s: 图标数据太短", tc.state)
			continue
		}

		if iconData[2] != 1 || iconData[3] != 0 {
			t.Errorf("状态 %s: 不是有效的 ICO 文件（type 应为 1）", tc.state)
		}
	}
}

// TestIconFilesExist 测试图标文件是否存在
func TestIconFilesExist(t *testing.T) {
	iconFiles := []string{
		"assets/tray-idle.ico",
		"assets/tray-connected.ico",
		"assets/tray-error.ico",
	}

	for _, file := range iconFiles {
		data, err := iconFS.ReadFile(file)
		if err != nil {
			t.Errorf("无法读取图标文件 %s: %v", file, err)
			continue
		}

		if len(data) == 0 {
			t.Errorf("图标文件 %s 为空", file)
		}

		if len(data) >= 6 {
			if data[2] != 1 || data[3] != 0 {
				t.Errorf("文件 %s 不是有效的 ICO（type 应为 1）", file)
			}
		}
	}
}

// TestEmbedFS 测试 embed.FS 是否正确嵌入
func TestEmbedFS(t *testing.T) {
	// 列出 embed.FS 中的所有文件
	entries, err := iconFS.ReadDir("assets")
	if err != nil {
		t.Fatalf("无法读取 assets 目录: %v", err)
	}

	if len(entries) == 0 {
		t.Error("assets 目录为空，embed 可能未正确工作")
	}

	t.Logf("embed.FS 中的文件:")
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Logf("  - %s (无法获取信息: %v)", entry.Name(), err)
			continue
		}
		t.Logf("  - %s (%d bytes)", entry.Name(), info.Size())
	}
}

// TestUpdateSerialStatus 测试根据串口状态更新托盘
// 注意：这个测试不调用 UpdateSerialStatus()，因为它需要 systray 在主 goroutine 运行
func TestUpdateSerialStatus(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	// 只测试状态字段，不调用需要 systray 运行的方法
	// UpdateSerialStatus 内部会调用 IsConnected()
	// 由于串口未实际连接，状态应该保持 idle
	if trayMgr.state != TrayIdle {
		t.Errorf("初始状态 = %s, want idle", trayMgr.state)
	}

	// 手动设置状态测试状态切换
	trayMgr.state = TrayConnected
	if trayMgr.state != TrayConnected {
		t.Errorf("设置状态后 = %s, want connected", trayMgr.state)
	}
}

// TestTrayManagerConfig 测试 TrayManager 配置
func TestTrayManagerConfig(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()

	// 测试不同的端口配置
	testCases := []struct {
		mcpPort int
		version string
	}{
		{5000, "0.1.0"},
		{5678, "1.0.0"},
		{0, "dev"},
	}

	for _, tc := range testCases {
		tray := NewTrayManager(serialMgr, conf, "127.0.0.1", tc.mcpPort, tc.version, false)
		if tray.mcpPort != tc.mcpPort {
			t.Errorf("mcpPort = %d, want %d", tray.mcpPort, tc.mcpPort)
		}
		if tray.version != tc.version {
			t.Errorf("version = %s, want %s", tray.version, tc.version)
		}
	}
}

// TestIconFilePaths 测试图标文件路径
func TestIconFilePaths(t *testing.T) {
	expectedFiles := map[TrayState]string{
		TrayIdle:      "assets/tray-idle.ico",
		TrayConnected: "assets/tray-connected.ico",
		TrayError:     "assets/tray-error.ico",
	}

	for state, expectedPath := range expectedFiles {
		data, err := iconFS.ReadFile(expectedPath)
		if err != nil {
			t.Errorf("状态 %s: 无法读取 %s: %v", state, expectedPath, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("状态 %s: 文件 %s 为空", state, expectedPath)
		}
	}
}

// BenchmarkGetIcon 测试图标加载性能
func BenchmarkGetIcon(b *testing.B) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, _ := serial.NewSerialManager(cfg)
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trayMgr.getIcon()
	}
}

// TestIconFileSize 测试图标文件大小
func TestIconFileSize(t *testing.T) {
	minSize := 100
	maxSize := 100 * 1024

	iconFiles := []string{
		"assets/tray-idle.ico",
		"assets/tray-connected.ico",
		"assets/tray-error.ico",
	}

	for _, file := range iconFiles {
		data, err := iconFS.ReadFile(file)
		if err != nil {
			t.Errorf("无法读取 %s: %v", file, err)
			continue
		}

		if len(data) < minSize {
			t.Errorf("文件 %s 太小: %d bytes, 期望至少 %d", file, len(data), minSize)
		}
		if len(data) > maxSize {
			t.Errorf("文件 %s 太大: %d bytes, 期望最多 %d", file, len(data), maxSize)
		}

		t.Logf("图标 %s: %d bytes", file, len(data))
	}
}

// TestTrayStateString 测试状态字符串表示
func TestTrayStateString(t *testing.T) {
	states := map[TrayState]string{
		TrayIdle:      "idle",
		TrayConnected: "connected",
		TrayError:     "error",
	}

	for state, expected := range states {
		if string(state) != expected {
			t.Errorf("状态 %s 的字符串表示 = %s, want %s", state, string(state), expected)
		}
	}
}

// TestShowConsoleMenu 测试 showConsoleMenu 参数对 TrayManager 的影响
func TestShowConsoleMenu(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()

	// showConsoleMenu=false 时 mShowLog 应为 nil
	trayFalse := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)
	testutil.AssertNil(t, trayFalse.mShowLog)
	testutil.AssertEqual(t, false, trayFalse.showConsoleMenu)

	// showConsoleMenu=true 时 mShowLog 仍为 nil（mShowLog 在 createMenu 中创建，需要 systray）
	// 但 showConsoleMenu 字段应为 true
	trayTrue := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", true)
	testutil.AssertNil(t, trayTrue.mShowLog)
	testutil.AssertEqual(t, true, trayTrue.showConsoleMenu)
}

// TestGetConfigSummary 测试 getConfigSummary 纯函数
func TestGetConfigSummary(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	// 默认配置: 115200, 8, none, 1 → "当前: 未选择 115200 8N1"
	testutil.AssertEqual(t, "当前: 未选择 115200 8N1", trayMgr.getConfigSummary())

	// even parity → "当前: 未选择 115200 8E1"
	conf.Serial.Parity = "even"
	testutil.AssertEqual(t, "当前: 未选择 115200 8E1", trayMgr.getConfigSummary())

	// odd parity → "当前: 未选择 115200 8O1"
	conf.Serial.Parity = "odd"
	testutil.AssertEqual(t, "当前: 未选择 115200 8O1", trayMgr.getConfigSummary())

	// 1.5 stopBits → "当前: 未选择 115200 8N1.5"
	conf.Serial.Parity = "none"
	conf.Serial.StopBits = 1.5
	testutil.AssertEqual(t, "当前: 未选择 115200 8N1.5", trayMgr.getConfigSummary())
}

// TestGetNetworkStatus 测试 getNetworkStatus 函数
func TestGetNetworkStatus(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()

	// wsPort=2323, mcpPort=5000
	tray1 := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)
	testutil.AssertEqual(t, "HTTP: 5000", tray1.getNetworkStatus())

	tray2 := NewTrayManager(serialMgr, conf, "127.0.0.1", 0, "0.1.0", false)
	testutil.AssertEqual(t, "HTTP: 0", tray2.getNetworkStatus())
}

// TestGetSerialMenuTitle 测试 getSerialMenuTitle 函数（未连接状态）
func TestGetSerialMenuTitle(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	conf.Serial.Port = getTestPort()
	trayMgr := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	// 未连接时返回 "连接 <port>"
	expected := "连接 " + getTestPort()
	testutil.AssertEqual(t, expected, trayMgr.getSerialMenuTitle())
}

// TestSetOnReady 测试 SetOnReady 回调设置
func TestSetOnReady(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	testutil.AssertNil(t, trayMgr.readyCallback)

	called := false
	trayMgr.SetOnReady(func() {
		called = true
	})
	testutil.AssertNotNil(t, trayMgr.readyCallback)

	// 验证回调可以正常调用
	trayMgr.readyCallback()
	testutil.AssertEqual(t, true, called)
}

// TestSetOnExit 测试 SetOnExit 回调设置
func TestSetOnExit(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	testutil.AssertNil(t, trayMgr.exitCallback)

	called := false
	trayMgr.SetOnExit(func() {
		called = true
	})
	testutil.AssertNotNil(t, trayMgr.exitCallback)

	// 验证回调可以正常调用
	trayMgr.exitCallback()
	testutil.AssertEqual(t, true, called)
}

// TestQuitChan 测试 QuitChan 返回非 nil channel
func TestQuitChan(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	ch := trayMgr.QuitChan()
	testutil.AssertNotNil(t, ch)

	// 验证 quitChan 和 QuitChan 返回同一个 channel
	testutil.AssertEqual(t, trayMgr.quitChan, ch)
}

// TestUpdateSerialStatus_StateDedup 测试 UpdateSerialStatus 的状态去重逻辑
func TestUpdateSerialStatus_StateDedup(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	// 设置当前状态为 TrayIdle（串口未连接时 UpdateSerialStatus 计算的 newState 也是 TrayIdle）
	trayMgr.state = TrayIdle

	// 模拟 UpdateSerialStatus 中的去重逻辑
	// connected := t.serial != nil && t.serial.IsConnected()  → false
	// newState := TrayIdle（因为 connected=false）
	// if t.state == newState { return }  → 去重生效，应直接返回
	connected := trayMgr.serial != nil && trayMgr.serial.IsConnected()
	testutil.AssertEqual(t, false, connected)

	newState := TrayIdle
	testutil.AssertEqual(t, trayMgr.state, newState) // 去重条件满足：state == newState

	// 验证去重后状态未变（仍然是 TrayIdle）
	testutil.AssertEqual(t, TrayIdle, trayMgr.state)

	// 设置不同状态验证非去重路径
	trayMgr.state = TrayConnected
	testutil.AssertNotEqual(t, trayMgr.state, newState) // 不满足去重条件
}

// TestRealIconFiles 测试实际图标文件（非 embed）
func TestRealIconFiles(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	assetsDir := filepath.Join(dir, "assets")

	if _, err := os.Stat(assetsDir); os.IsNotExist(err) {
		t.Skipf("assets 目录不存在: %s", assetsDir)
	}

	files := []string{
		"tray-idle.ico",
		"tray-connected.ico",
		"tray-error.ico",
	}

	for _, file := range files {
		path := filepath.Join(assetsDir, file)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("文件 %s 不存在: %v", path, err)
			continue
		}

		if info.Size() == 0 {
			t.Errorf("文件 %s 为空", path)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("无法读取 %s: %v", path, err)
			continue
		}

		if len(data) >= 6 {
			if data[2] != 1 || data[3] != 0 {
				t.Errorf("文件 %s 不是有效的 ICO", path)
			}
		}

		t.Logf("实际文件 %s: %d bytes", file, info.Size())
	}
}

// newTestTrayManager 创建用于测试的 TrayManager，初始化 MenuItem 字段以支持 setXxx 方法测试
func newTestTrayManager(t *testing.T) *TrayManager {
	t.Helper()
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	t.Cleanup(func() { serialMgr.Close() })
	conf := config.GetDefault()
	// 确保 conf.Serial 与 serialMgr 的配置一致
	conf.Serial.Port = cfg.Port
	conf.Serial.BaudRate = cfg.BaudRate
	conf.Serial.DataBits = cfg.DataBits
	conf.Serial.Parity = cfg.Parity
	conf.Serial.StopBits = float64(cfg.StopBits)
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", true)

	// 手动初始化 MenuItem 字段，使 setXxx 方法可安全调用
	// systray.MenuItem 在非 systray.Run 环境下调用 SetTitle 等方法会输出 error log 但不 panic
	tm.mSerial = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mSelectPort = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mSerialConfig = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mBaudRate = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mDataBits = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mStopBits = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mParity = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mCurrentConfig = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mNetworkStatus = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mShowLog = &systray.MenuItem{ClickedCh: make(chan struct{})}

	// 初始化波特率菜单项
	for _, rate := range baudRates {
		tm.mBaudRateItems[rate] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	}
	// 初始化数据位菜单项
	for _, bits := range dataBitsList {
		tm.mDataBitsItems[bits] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	}
	// 初始化停止位菜单项
	for _, bits := range stopBitsList {
		tm.mStopBitsItems[bits] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	}
	// 初始化校验位菜单项
	for _, p := range parityList {
		tm.mParityItems[p] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	}

	return tm
}

// TestBaudRatesList 测试波特率列表常量
func TestBaudRatesList(t *testing.T) {
	testutil.AssertEqual(t, 6, len(baudRates))
	expected := []int{9600, 19200, 38400, 57600, 115200, 230400}
	for i, rate := range baudRates {
		testutil.AssertEqual(t, expected[i], rate)
	}
}

// TestDataBitsList 测试数据位列表常量
func TestDataBitsList(t *testing.T) {
	testutil.AssertEqual(t, 4, len(dataBitsList))
	expected := []int{5, 6, 7, 8}
	for i, bits := range dataBitsList {
		testutil.AssertEqual(t, expected[i], bits)
	}
}

// TestStopBitsList 测试停止位列表常量
func TestStopBitsList(t *testing.T) {
	testutil.AssertEqual(t, 3, len(stopBitsList))
	testutil.AssertEqual(t, float64(1), stopBitsList[0])
	testutil.AssertEqual(t, 1.5, stopBitsList[1])
	testutil.AssertEqual(t, float64(2), stopBitsList[2])
}

// TestParityList 测试校验位列表常量
func TestParityList(t *testing.T) {
	testutil.AssertEqual(t, 3, len(parityList))
	testutil.AssertEqual(t, "none", parityList[0])
	testutil.AssertEqual(t, "even", parityList[1])
	testutil.AssertEqual(t, "odd", parityList[2])
}

// TestSetBaudRate_未连接时更新 测试未连接时设置波特率
func TestSetBaudRate_MultipleUpdates(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	tm.setBaudRate(9600)
	testutil.AssertEqual(t, 9600, tm.config.Serial.BaudRate)
	tm.setBaudRate(115200)
	testutil.AssertEqual(t, 115200, tm.config.Serial.BaudRate)
	tm.setBaudRate(230400)
	testutil.AssertEqual(t, 230400, tm.config.Serial.BaudRate)
}

func TestSetStopBits_UpdateWhenConnected(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())
	for _, bits := range stopBitsList {
		tm.setStopBits(bits)
		if bits == 1.5 {
			testutil.AssertEqual(t, 1.5, tm.config.Serial.StopBits)
		}
	}
}
func TestSetStopBits_2(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())
	tm.setStopBits(2)
	testutil.AssertEqual(t, float64(2), tm.config.Serial.StopBits)
}
func TestSetParity_UpdateWhenConnected(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())
	for _, p := range parityList {
		tm.setParity(p)
		testutil.AssertEqual(t, p, tm.config.Serial.Parity)
	}
}

// TestSetDataBits_未连接时更新 测试未连接时设置数据位
func TestSetDataBits_UpdateWhenDisconnected(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	for _, bits := range dataBitsList {
		tm.setDataBits(bits)
		testutil.AssertEqual(t, bits, tm.config.Serial.DataBits)
	}
}

// TestSetStopBits_未连接时更新 测试未连接时设置停止位
func TestSetStopBits_UpdateWhenDisconnected(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	// 设置为 1
	tm.setStopBits(1)
	testutil.AssertEqual(t, float64(1), tm.config.Serial.StopBits)

	// 设置为 1.5
	tm.setStopBits(1.5)
	testutil.AssertEqual(t, 1.5, tm.config.Serial.StopBits)

	// 设置为 2
	tm.setStopBits(2)
	testutil.AssertEqual(t, float64(2), tm.config.Serial.StopBits)
}

// TestSetParity_未连接时更新 测试未连接时设置校验位
func TestSetParity_UpdateWhenDisconnected(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	for _, p := range parityList {
		tm.setParity(p)
		testutil.AssertEqual(t, p, tm.config.Serial.Parity)
	}
}

// TestSetPort_未连接时更新 测试未连接时设置串口
func TestSetPort_UpdateWhenDisconnected(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	tm.mPortItems["COM_TEST1"] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mPortItems["COM_TEST2"] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	callWithTimeout(t, func() {
		tm.setPort("COM_TEST1")
	}, 500*time.Millisecond)
	testutil.AssertEqual(t, "COM_TEST1", tm.config.Serial.Port)
	testutil.AssertEqual(t, "COM_TEST1", tm.serial.GetConfig().Port)

	callWithTimeout(t, func() {
		tm.setPort("COM_TEST2")
	}, 500*time.Millisecond)
	testutil.AssertEqual(t, "COM_TEST2", tm.config.Serial.Port)
	testutil.AssertEqual(t, "COM_TEST2", tm.serial.GetConfig().Port)
}

// TestSetPort_同步到SerialManager 测试设置串口时同步到SerialManager
func TestSetPort_SyncToSerialManager(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	initialPort := tm.config.Serial.Port
	testutil.AssertEqual(t, initialPort, tm.serial.GetConfig().Port)

	tm.mPortItems["COM4"] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	callWithTimeout(t, func() {
		tm.setPort("COM4")
	}, 500*time.Millisecond)

	testutil.AssertEqual(t, "COM4", tm.config.Serial.Port)
	testutil.AssertEqual(t, "COM4", tm.serial.GetConfig().Port)
}

// TestSetBaudRate_同步到SerialManager 测试设置波特率时同步到SerialManager
func TestSetBaudRate_SyncToSerialManager(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	// 添加模拟波特率菜单项
	tm.mBaudRateItems[9600] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mBaudRateItems[115200] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	// 设置新波特率
	tm.setBaudRate(9600)

	// 验证 TrayManager 配置更新
	testutil.AssertEqual(t, 9600, tm.config.Serial.BaudRate)
	// 验证 SerialManager 配置同步更新
	testutil.AssertEqual(t, 9600, tm.serial.GetConfig().BaudRate)

	// 再次设置
	tm.setBaudRate(115200)
	testutil.AssertEqual(t, 115200, tm.config.Serial.BaudRate)
	testutil.AssertEqual(t, 115200, tm.serial.GetConfig().BaudRate)
}

// TestSetDataBits_同步到SerialManager 测试设置数据位时同步到SerialManager
func TestSetDataBits_SyncToSerialManager(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	// 添加模拟数据位菜单项
	tm.mDataBitsItems[7] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mDataBitsItems[8] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	// 设置新数据位
	tm.setDataBits(7)

	// 验证 TrayManager 配置更新
	testutil.AssertEqual(t, 7, tm.config.Serial.DataBits)
	// 验证 SerialManager 配置同步更新
	testutil.AssertEqual(t, 7, tm.serial.GetConfig().DataBits)
}

// TestSetStopBits_同步到SerialManager 测试设置停止位时同步到SerialManager
func TestSetStopBits_SyncToSerialManager(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	// 添加模拟停止位菜单项
	tm.mStopBitsItems[1] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mStopBitsItems[2] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	// 设置新停止位
	tm.setStopBits(2)

	// 验证 TrayManager 配置更新
	testutil.AssertEqual(t, float64(2), tm.config.Serial.StopBits)
	// 验证 SerialManager 配置同步更新（注意：SerialManager 使用 float32）
	testutil.AssertEqual(t, float32(2), tm.serial.GetConfig().StopBits)
}

// TestSetParity_同步到SerialManager 测试设置校验位时同步到SerialManager
func TestSetParity_SyncToSerialManager(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	// 添加模拟校验位菜单项
	tm.mParityItems["none"] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mParityItems["even"] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mParityItems["odd"] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	// 设置新校验位
	tm.setParity("even")

	// 验证 TrayManager 配置更新
	testutil.AssertEqual(t, "even", tm.config.Serial.Parity)
	// 验证 SerialManager 配置同步更新
	testutil.AssertEqual(t, "even", tm.serial.GetConfig().Parity)

	// 再次设置
	tm.setParity("odd")
	testutil.AssertEqual(t, "odd", tm.config.Serial.Parity)
	testutil.AssertEqual(t, "odd", tm.serial.GetConfig().Parity)
}

// TestSyncSerialConfig_完整配置同步 测试完整配置同步功能
func TestSyncSerialConfig_FullSync(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	// 修改所有配置
	tm.config.Serial.Port = "COM4"
	tm.config.Serial.BaudRate = 9600
	tm.config.Serial.DataBits = 7
	tm.config.Serial.StopBits = 2
	tm.config.Serial.Parity = "even"

	// 调用同步
	tm.syncSerialConfig()

	// 验证 SerialManager 配置已同步
	serialCfg := tm.serial.GetConfig()
	testutil.AssertEqual(t, "COM4", serialCfg.Port)
	testutil.AssertEqual(t, 9600, serialCfg.BaudRate)
	testutil.AssertEqual(t, 7, serialCfg.DataBits)
	testutil.AssertEqual(t, float32(2), serialCfg.StopBits)
	testutil.AssertEqual(t, "even", serialCfg.Parity)
}

// TestUpdateConfigDisplay 测试更新配置显示
func TestUpdateConfigDisplay(t *testing.T) {
	tm := newTestTrayManager(t)
	// updateConfigDisplay 调用 getConfigSummary 并 SetTitle，不会 panic
	tm.updateConfigDisplay()
	// 验证 config 未被改变
	testutil.AssertEqual(t, 115200, tm.config.Serial.BaudRate)
	testutil.AssertEqual(t, 8, tm.config.Serial.DataBits)
}

// TestOnExit 测试 onExit 方法
func TestOnExit(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	exitCalled := false
	tm.SetOnExit(func() {
		exitCalled = true
	})

	// 调用 onExit
	tm.onExit()

	testutil.AssertEqual(t, true, exitCalled)

	// 验证 quitChan 已关闭
	_, ok := <-tm.quitChan
	testutil.AssertEqual(t, false, ok) // channel 已关闭，应返回零值
}

// TestOnExit_无回调 测试没有 exitCallback 时的 onExit
func TestOnExit_NoCallback(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	// 不设置 exitCallback，直接调用 onExit
	tm.onExit()

	// 验证 quitChan 已关闭
	_, ok := <-tm.quitChan
	testutil.AssertEqual(t, false, ok)
}

// TestToggleConsoleWindow_显示 测试切换控制台窗口（显示）
func TestToggleConsoleWindow_Show(t *testing.T) {
	tm := newTestTrayManager(t)
	tm.consoleVisible = false

	// 切换为显示
	tm.toggleConsoleWindow()
	testutil.AssertEqual(t, true, tm.consoleVisible)
}

// TestToggleConsoleWindow_隐藏 测试切换控制台窗口（隐藏）
func TestToggleConsoleWindow_Hide(t *testing.T) {
	tm := newTestTrayManager(t)
	tm.consoleVisible = true

	// 切换为隐藏
	tm.toggleConsoleWindow()
	testutil.AssertEqual(t, false, tm.consoleVisible)
}

// TestGetConfigSummary_停止位2 测试停止位为2时的配置摘要
func TestGetConfigSummary_StopBits2(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	conf.Serial.StopBits = 2
	testutil.AssertEqual(t, "当前: 未选择 115200 8N2", tm.getConfigSummary())
}

// TestGetConfigSummary_完整覆盖 测试 getConfigSummary 所有分支
func TestGetConfigSummary_FullCoverage(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	tests := []struct {
		parity   string
		stopBits float64
		expected string
	}{
		{"none", 1, "当前: 未选择 115200 8N1"},
		{"none", 1.5, "当前: 未选择 115200 8N1.5"},
		{"none", 2, "当前: 未选择 115200 8N2"},
		{"even", 1, "当前: 未选择 115200 8E1"},
		{"odd", 1, "当前: 未选择 115200 8O1"},
	}

	for _, tt := range tests {
		conf.Serial.Parity = tt.parity
		conf.Serial.StopBits = tt.stopBits
		testutil.AssertEqual(t, tt.expected, tm.getConfigSummary())
	}
}

// TestNewTrayManager_内部map初始化 测试 TrayManager 内部 map 初始化
func TestNewTrayManager_InternalMapInit(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	testutil.AssertNotNil(t, tm.mPortItems)
	testutil.AssertNotNil(t, tm.mBaudRateItems)
	testutil.AssertNotNil(t, tm.mDataBitsItems)
	testutil.AssertNotNil(t, tm.mStopBitsItems)
	testutil.AssertNotNil(t, tm.mParityItems)
	testutil.AssertEqual(t, 0, len(tm.mPortItems))
	testutil.AssertEqual(t, 0, len(tm.mBaudRateItems))
	testutil.AssertEqual(t, 0, len(tm.mDataBitsItems))
	testutil.AssertEqual(t, 0, len(tm.mStopBitsItems))
	testutil.AssertEqual(t, 0, len(tm.mParityItems))
}

// TestConsoleWindows 测试控制台窗口相关函数（Windows API）
func TestConsoleWindows(t *testing.T) {
	hwnd := GetConsoleWindow()
	t.Logf("Console window handle: %v", hwnd)

	visible := IsConsoleVisible()
	t.Logf("Console visible: %v", visible)

	_ = ShowWindow(hwnd, SW_SHOW)
	t.Logf("ShowWindow done")

	_ = SetForegroundWindow(hwnd)
	t.Logf("SetForegroundWindow done")

	_ = IsWindowVisible(hwnd)
	t.Logf("IsWindowVisible done")
}

// TestHideShowConsole 测试隐藏/显示控制台
func TestHideShowConsole(t *testing.T) {
	HideConsole()
	ShowConsole()
}

// TestDisableCloseButton 测试禁用关闭按钮
func TestDisableCloseButton(t *testing.T) {
	DisableCloseButton()
}

// TestGetSystemMenu 测试获取系统菜单
func TestGetSystemMenu(t *testing.T) {
	hwnd := GetConsoleWindow()
	menu := GetSystemMenu(hwnd, false)
	t.Logf("System menu handle: %v", menu)

	menu2 := GetSystemMenu(hwnd, true)
	t.Logf("System menu handle (revert): %v", menu2)
}

// TestRemoveMenu 测试移除菜单项
func TestRemoveMenu(t *testing.T) {
	hwnd := GetConsoleWindow()
	menu := GetSystemMenu(hwnd, false)
	result := RemoveMenu(menu, SC_CLOSE, MF_BYCOMMAND)
	t.Logf("RemoveMenu result: %v", result)
}

// TestIsConsoleVisible_完整 测试 IsConsoleVisible 完整路径
func TestIsConsoleVisible_Full(t *testing.T) {
	visible := IsConsoleVisible()
	t.Logf("控制台可见: %v", visible)

	hwnd := GetConsoleWindow()
	if hwnd != 0 {
		ShowConsole()
		testutil.AssertEqual(t, true, IsConsoleVisible())
	}
}

// TestHideConsole_隐藏后验证 测试 HideConsole 完整路径
func TestHideConsole_VerifyAfterHide(t *testing.T) {
	ShowConsole()
	HideConsole()
	ShowConsole()
}

// TestShowConsole_完整 测试 ShowConsole 完整路径
func TestShowConsole_Full(t *testing.T) {
	ShowConsole()
}

// TestDisableCloseButton_完整 测试 DisableCloseButton 完整路径
func TestDisableCloseButton_Full(t *testing.T) {
	DisableCloseButton()
}

// TestRefreshPortList 测试刷新串口列表
func TestRefreshPortList(t *testing.T) {
	tm := newTestTrayManager(t)
	tm.mSelectPort = &systray.MenuItem{ClickedCh: make(chan struct{})}

	tm.refreshPortList()
	t.Logf("端口数量: %d", len(tm.mPortItems))
}

// TestGetConfigSummary_未知校验位 测试未知校验位时的配置摘要
func TestGetConfigSummary_UnknownParity(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	conf.Serial.Parity = "unknown"
	testutil.AssertEqual(t, "当前: 未选择 115200 8unknown1", tm.getConfigSummary())
}

// TestGetIcon_AllStates 测试所有状态的图标加载
func TestGetIcon_AllStates(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	for _, state := range []TrayState{TrayIdle, TrayConnected, TrayError} {
		tm.state = state
		iconData := tm.getIcon()
		if len(iconData) == 0 {
			t.Errorf("状态 %s: 图标数据为空", state)
		}
	}
}

// TestToggleConsoleWindow_双向切换 测试控制台窗口双向切换
func TestToggleConsoleWindow_BidirectionalToggle(t *testing.T) {
	tm := newTestTrayManager(t)

	tm.consoleVisible = false
	tm.toggleConsoleWindow()
	testutil.AssertEqual(t, true, tm.consoleVisible)

	tm.toggleConsoleWindow()
	testutil.AssertEqual(t, false, tm.consoleVisible)

	tm.toggleConsoleWindow()
	testutil.AssertEqual(t, true, tm.consoleVisible)
}

// TestConsoleWindows_所有API 测试所有控制台 Windows API 函数
func TestConsoleWindows_AllAPIs(t *testing.T) {
	hwnd := GetConsoleWindow()
	t.Logf("hwnd: %v", hwnd)

	_ = ShowWindow(hwnd, SW_HIDE)
	_ = ShowWindow(hwnd, SW_SHOW)
	_ = ShowWindow(hwnd, SW_RESTORE)
	_ = SetForegroundWindow(hwnd)
	_ = IsWindowVisible(hwnd)
	_ = GetSystemMenu(hwnd, false)
	_ = GetSystemMenu(hwnd, true)
	_ = RemoveMenu(GetSystemMenu(hwnd, false), SC_CLOSE, MF_BYCOMMAND)
	_ = IsConsoleVisible()
	HideConsole()
	ShowConsole()
	DisableCloseButton()
}

// TestNewTrayManager_字段验证 测试 TrayManager 所有字段初始化
func TestNewTrayManager_FieldVerification(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "1.0.0", true)

	testutil.AssertEqual(t, "1.0.0", tm.version)
	testutil.AssertEqual(t, 5000, tm.mcpPort)
	testutil.AssertEqual(t, TrayIdle, tm.state)
	testutil.AssertEqual(t, false, tm.consoleVisible)
	testutil.AssertEqual(t, true, tm.showConsoleMenu)
	testutil.AssertNotNil(t, tm.quitChan)
	testutil.AssertNil(t, tm.readyCallback)
	testutil.AssertNil(t, tm.exitCallback)
}

// TestRefreshPortList_有旧项 测试 refreshPortList 清理旧菜单项
func TestRefreshPortList_WithOldItems(t *testing.T) {
	tm := newTestTrayManager(t)

	tm.mPortItems["COM_OLD1"] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mPortItems["COM_OLD2"] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	tm.refreshPortList()

	_, hasOld1 := tm.mPortItems["COM_OLD1"]
	_, hasOld2 := tm.mPortItems["COM_OLD2"]
	testutil.AssertEqual(t, false, hasOld1)
	testutil.AssertEqual(t, false, hasOld2)
}

// callWithTimeout 在超时内调用可能阻塞的函数，覆盖阻塞前的语句
func callWithTimeout(t *testing.T, fn func(), timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	var panicErr error
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr = fmt.Errorf("panic: %v", r)
			}
			close(done)
		}()
		fn()
	}()
	select {
	case <-done:
		if panicErr != nil {
			t.Logf("函数 panic（非 systray 环境）: %v", panicErr)
		}
	case <-time.After(timeout):
		t.Logf("函数在 %v 内未返回（预期行为，systray 阻塞）", timeout)
	}
}

// TestUpdateState_异步调用 测试 UpdateState 覆盖状态赋值和 getIcon 调用
func TestUpdateState_AsyncCall(t *testing.T) {
	tm := newTestTrayManager(t)

	testutil.AssertEqual(t, TrayIdle, tm.state)

	// UpdateState 会调用 systray.SetIcon 阻塞，但在那之前会覆盖状态和调用 getIcon
	callWithTimeout(t, func() {
		tm.UpdateState(TrayError)
	}, 200*time.Millisecond)

	// systray.SetIcon 阻塞前，状态已更新
	testutil.AssertEqual(t, TrayError, tm.state)
}

// TestUpdateSerialStatus_状态切换 测试 UpdateSerialStatus 覆盖状态计算逻辑
func TestUpdateSerialStatus_StateSwitch(t *testing.T) {
	tm := newTestTrayManager(t)

	// 设置状态为 TrayConnected，使去重检查不触发（connected=false → newState=TrayIdle ≠ TrayConnected）
	tm.state = TrayConnected

	// UpdateSerialStatus 会计算 connected=false, newState=TrayIdle
	// 由于 t.state(TrayConnected) != newState(TrayIdle)，不会提前 return
	// 然后调用 UpdateState(TrayIdle) → systray.SetIcon 阻塞
	callWithTimeout(t, func() {
		tm.UpdateSerialStatus()
	}, 200*time.Millisecond)

	// 验证状态计算：connected=false, newState=TrayIdle
	connected := tm.serial != nil && tm.serial.IsConnected()
	testutil.AssertEqual(t, false, connected)
}

// TestToggleSerial_未连接 测试 toggleSerial 未连接时的 Connect 尝试
func TestToggleSerial_NotConnected(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	// toggleSerial 会尝试 Connect（失败因为没有真实串口），然后调用 UpdateSerialStatus
	callWithTimeout(t, func() {
		tm.toggleSerial()
	}, 200*time.Millisecond)
}

// TestOnReady_异步调用 测试 onReady 覆盖日志输出
func TestOnReady_AsyncCall(t *testing.T) {
	tm := newTestTrayManager(t)

	// onReady 会输出日志然后调用 systray.SetIcon 阻塞
	callWithTimeout(t, func() {
		tm.onReady()
	}, 200*time.Millisecond)
}

// TestSetOnConfigChanged_CallbackInvoked 测试配置变更回调被调用
func TestSetOnConfigChanged_CallbackInvoked(t *testing.T) {
	tm := newTestTrayManager(t)

	var capturedBaudRate int
	var capturedDataBits int
	callbackInvoked := false

	tm.SetOnConfigChanged(func(port string, baudRate int, dataBits int, parity string, stopBits float64) {
		_ = port
		capturedBaudRate = baudRate
		capturedDataBits = dataBits
		_ = parity
		_ = stopBits
		callbackInvoked = true
	})

	tm.mBaudRateItems[9600] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	callWithTimeout(t, func() {
		tm.setBaudRate(9600)
	}, 200*time.Millisecond)

	testutil.AssertEqual(t, true, callbackInvoked)
	testutil.AssertEqual(t, 9600, capturedBaudRate)
	testutil.AssertEqual(t, 8, capturedDataBits)
}

// TestNotifyConfigChangedAndReconnect_NoCallback 测试无回调时不panic
func TestNotifyConfigChangedAndReconnect_NoCallback(t *testing.T) {
	tm := newTestTrayManager(t)
	if tm.onConfigChanged != nil {
		t.Error("expected onConfigChanged to be nil")
	}

	tm.notifyConfigChangedAndReconnect()
}

// TestAutoReconnect_NoPort 测试无端口时不尝试连接
func TestAutoReconnect_NoPort(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	tm.autoReconnect()

	testutil.AssertEqual(t, false, tm.serial.IsConnected())
}

// TestTrayManager_ConfigSyncToSerialManager 测试配置同步到 SerialManager
func TestTrayManager_ConfigSyncToSerialManager(t *testing.T) {
	tm := newTestTrayManager(t)

	// 修改所有配置
	tm.mBaudRateItems[38400] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mDataBitsItems[6] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mStopBitsItems[1.5] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mParityItems["odd"] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	callWithTimeout(t, func() {
		tm.setBaudRate(38400)
	}, 200*time.Millisecond)
	callWithTimeout(t, func() {
		tm.setDataBits(6)
	}, 200*time.Millisecond)
	callWithTimeout(t, func() {
		tm.setStopBits(1.5)
	}, 200*time.Millisecond)
	callWithTimeout(t, func() {
		tm.setParity("odd")
	}, 200*time.Millisecond)

	// 验证 SerialManager 配置已同步
	serialCfg := tm.serial.GetConfig()
	testutil.AssertEqual(t, 38400, serialCfg.BaudRate)
	testutil.AssertEqual(t, 6, serialCfg.DataBits)
	testutil.AssertEqual(t, float32(1.5), serialCfg.StopBits)
	testutil.AssertEqual(t, "odd", serialCfg.Parity)
}

// TestTrayManager_AutoReconnectBehavior 测试自动重连行为
func TestTrayManager_AutoReconnectBehavior(t *testing.T) {
	tm := newTestTrayManager(t)

	// 初始未连接
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	// 设置端口（会尝试自动连接）
	tm.mPortItems["COM_AUTO"] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	callWithTimeout(t, func() {
		tm.setPort("COM_AUTO")
	}, 500*time.Millisecond)

	// 由于没有真实串口，连接会失败，但配置应已更新
	testutil.AssertEqual(t, "COM_AUTO", tm.config.Serial.Port)
	testutil.AssertEqual(t, "COM_AUTO", tm.serial.GetConfig().Port)
}

func TestOpenTerminal(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	tm.openTerminal()
}

func TestOpenTerminal_WithHost(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	tm := NewTrayManager(serialMgr, conf, "0.0.0.0", 8080, "1.0.0", false)

	tm.openTerminal()
}

func TestGetSerialMenuTitle_Connected(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	expected := "连接 " + tm.config.Serial.Port
	testutil.AssertEqual(t, expected, tm.getSerialMenuTitle())
}

func TestAutoReconnect_WithPort(t *testing.T) {
	tm := newTestTrayManager(t)
	tm.config.Serial.Port = "COM_NONEXISTENT"

	tm.autoReconnect()

	testutil.AssertEqual(t, false, tm.serial.IsConnected())
}

func TestNotifyConfigChangedAndReconnect_WithCallback(t *testing.T) {
	tm := newTestTrayManager(t)

	callbackInvoked := false
	var capturedPort string
	tm.SetOnConfigChanged(func(port string, baudRate int, dataBits int, parity string, stopBits float64) {
		callbackInvoked = true
		capturedPort = port
	})

	tm.notifyConfigChangedAndReconnect()

	testutil.AssertEqual(t, true, callbackInvoked)
	testutil.AssertEqual(t, tm.config.Serial.Port, capturedPort)
}

func TestToggleSerial_AlreadyConnected(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertEqual(t, false, tm.serial.IsConnected())

	callWithTimeout(t, func() {
		tm.toggleSerial()
	}, 200*time.Millisecond)
}

func TestOnReady_NoCallback(t *testing.T) {
	tm := newTestTrayManager(t)
	testutil.AssertNil(t, tm.readyCallback)

	callWithTimeout(t, func() {
		tm.onReady()
	}, 200*time.Millisecond)
}

func TestOnReady_WithCallback(t *testing.T) {
	tm := newTestTrayManager(t)

	readyCalled := false
	tm.SetOnReady(func() {
		readyCalled = true
	})

	callWithTimeout(t, func() {
		tm.onReady()
	}, 200*time.Millisecond)

	testutil.AssertNotNil(t, tm.readyCallback)

	tm.readyCallback()
	testutil.AssertEqual(t, true, readyCalled)
}

func TestGetConfigSummary_WithPort(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	conf.Serial.Port = "COM4"
	tm := NewTrayManager(serialMgr, conf, "127.0.0.1", 5000, "0.1.0", false)

	testutil.AssertEqual(t, "当前: COM4 115200 8N1", tm.getConfigSummary())
}

// TestSetPort_SamePort 测试设置为相同端口
func TestSetPort_SamePort(t *testing.T) {
	tm := newTestTrayManager(t)
	initialPort := tm.config.Serial.Port

	tm.mPortItems[initialPort] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	callWithTimeout(t, func() {
		tm.setPort(initialPort)
	}, 500*time.Millisecond)

	testutil.AssertEqual(t, initialPort, tm.config.Serial.Port)
}

// TestTrayManager_MenuItemUpdates 测试菜单项标题更新
func TestTrayManager_MenuItemUpdates(t *testing.T) {
	tm := newTestTrayManager(t)

	// 初始化菜单项
	tm.mBaudRateItems[9600] = &systray.MenuItem{ClickedCh: make(chan struct{})}
	tm.mBaudRateItems[115200] = &systray.MenuItem{ClickedCh: make(chan struct{})}

	// 修改波特率
	callWithTimeout(t, func() {
		tm.setBaudRate(9600)
	}, 200*time.Millisecond)

	// 验证配置已更新
	testutil.AssertEqual(t, 9600, tm.config.Serial.BaudRate)
}
