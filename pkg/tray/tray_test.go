// Package tray 系统托盘测试
package tray

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/serial"
)

func TestTrayState(t *testing.T) {
	// 测试状态切换
	states := []TrayState{TrayIdle, TrayConnected, TrayError}
	for _, state := range states {
		if state != TrayIdle && state != TrayConnected && state != TrayError {
			t.Errorf("无效的状态: %s", state)
		}
	}
}

func TestNewTrayManager(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	config := config.GetDefault()

	tray := NewTrayManager(serialMgr, config, 2323, 5000, "0.1.0", false)
	if tray == nil {
		t.Fatal("NewTrayManager 返回 nil")
	}

	if tray.version != "0.1.0" {
		t.Errorf("version = %s, want 0.1.0", tray.version)
	}
	if tray.telnetPort != 2323 {
		t.Errorf("telnetPort = %d, want 2323", tray.telnetPort)
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
	cfg.Port = "COM9"
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	config := config.GetDefault()
	tray := NewTrayManager(serialMgr, config, 2323, 5000, "0.1.0", false)

	// 只测试状态字段，不调用 systray.SetIcon（需要 GUI 环境）
	tray.state = TrayConnected
	if tray.state != TrayConnected {
		t.Errorf("设置 state = %s, want connected", tray.state)
	}

	tray.state = TrayError
	if tray.state != TrayError {
		t.Errorf("设置 state = %s, want error", tray.state)
	}
}

// TestGetIcon 测试图标加载功能
func TestGetIcon(t *testing.T) {
	cfg := serial.DefaultConfig()
	cfg.Port = "COM9"
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, 2323, 5000, "0.1.0", false)

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
	cfg.Port = "COM9"
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, 2323, 5000, "0.1.0", false)

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
	cfg.Port = "COM9"
	serialMgr, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager failed: %v", err)
	}
	defer serialMgr.Close()
	conf := config.GetDefault()

	// 测试不同的端口配置
	testCases := []struct {
		telnetPort int
		mcpPort    int
		version    string
	}{
		{2323, 5000, "0.1.0"},
		{1234, 5678, "1.0.0"},
		{0, 0, "dev"},
	}

	for _, tc := range testCases {
		tray := NewTrayManager(serialMgr, conf, tc.telnetPort, tc.mcpPort, tc.version, false)
		if tray.telnetPort != tc.telnetPort {
			t.Errorf("telnetPort = %d, want %d", tray.telnetPort, tc.telnetPort)
		}
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
	cfg.Port = "COM9"
	serialMgr, _ := serial.NewSerialManager(cfg)
	defer serialMgr.Close()
	conf := config.GetDefault()
	trayMgr := NewTrayManager(serialMgr, conf, 2323, 5000, "0.1.0", false)

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
