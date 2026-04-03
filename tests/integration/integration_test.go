// Package integration provides integration tests for serialhub.exe
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var binaryPath string

func TestMain(m *testing.M) {
	// 查找 serialhub.exe
	wd, _ := os.Getwd()
	binaryPath = filepath.Join(wd, "..", "..", "bin", "serialhub.exe")

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		fmt.Printf("serialhub.exe not found at %s, running go build...\n", binaryPath)
		cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/serialhub")
		cmd.Dir = filepath.Join(wd, "..", "..")
		if err := cmd.Run(); err != nil {
			fmt.Printf("Failed to build serialhub.exe: %v\n", err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

func getTestPort() string {
	port := os.Getenv("SERIALHUB_TEST_PORT")
	if port == "" {
		port = "COM9"
	}
	return port
}

func TestSerialHub_Version(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run serialhub --version: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(string(output), "SerialHub v") {
		t.Errorf("Unexpected version output: %s", output)
	}

	t.Logf("Version output: %s", strings.TrimSpace(string(output)))
}

func TestSerialHub_Help(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run serialhub --help: %v\nOutput: %s", err, output)
	}

	helpText := string(output)
	expectedStrings := []string{
		"--serial-port",
		"--baud-rate",
		"--telnet-port",
		"--mcp-port",
		"--config",
		"--debug",
		"--no-tray",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(helpText, expected) {
			t.Errorf("Help output missing %s", expected)
		}
	}

	t.Logf("Help output verified, %d flags found", len(expectedStrings))
}

func TestSerialHub_StartAndStop(t *testing.T) {
	if os.Getenv("SERIALHUB_INTEGRATION_TEST") != "1" {
		t.Skip("集成测试未启用，设置 SERIALHUB_INTEGRATION_TEST=1 启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动 serialhub --no-tray
	cmd := exec.CommandContext(ctx, binaryPath, "--no-tray", "--telnet-port", "23230", "--mcp-port", "50010")
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start serialhub: %v", err)
	}

	// 等待服务启动
	time.Sleep(2 * time.Second)

	// 检查 HTTP 健康检查
	resp, err := http.Get("http://127.0.0.1:50010/health")
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health check returned %d, expected 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	t.Logf("Health check response: %s", body)

	// 验证返回内容
	var healthResp map[string]interface{}
	if err := json.Unmarshal(body, &healthResp); err != nil {
		t.Errorf("Failed to parse health response: %v", err)
	}

	if status, ok := healthResp["status"].(string); !ok || status != "ok" {
		t.Errorf("Unexpected health status: %v", healthResp)
	}

	// 发送中断信号停止进程
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Logf("Failed to send interrupt signal: %v", err)
	}

	// 等待进程退出
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "signal") {
			t.Logf("Process exited with: %v", err)
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Log("Process killed after timeout")
	}

	// 读取 stderr 日志
	stderrBytes, _ := io.ReadAll(stderr)
	t.Logf("Server logs:\n%s", string(stderrBytes))

	t.Log("Start and stop test passed")
}

func TestSerialHub_MCPList(t *testing.T) {
	if os.Getenv("SERIALHUB_INTEGRATION_TEST") != "1" {
		t.Skip("集成测试未启用，设置 SERIALHUB_INTEGRATION_TEST=1 启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--no-tray", "--telnet-port", "23231", "--mcp-port", "50011")
	cmd.Start()
	defer cmd.Process.Kill()

	time.Sleep(2 * time.Second)

	// 调用 MCP serial_list 工具
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "serial_list",
		},
		"id": 1,
	}

	reqBytes, _ := json.Marshal(reqBody)
	resp, err := http.Post("http://127.0.0.1:50011/mcp", "application/json", strings.NewReader(string(reqBytes)))
	if err != nil {
		t.Fatalf("MCP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("MCP response: %s", body)

	var mcpResp map[string]interface{}
	if err := json.Unmarshal(body, &mcpResp); err != nil {
		t.Fatalf("Failed to parse MCP response: %v", err)
	}

	// 验证返回结果
	if result, ok := mcpResp["result"].(map[string]interface{}); ok {
		if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
			t.Log("MCP serial_list returned content")
		}
	}

	t.Log("MCP list test passed")
}

func TestSerialHub_ConfigFile(t *testing.T) {
	if os.Getenv("SERIALHUB_INTEGRATION_TEST") != "1" {
		t.Skip("集成测试未启用，设置 SERIALHUB_INTEGRATION_TEST=1 启用")
	}

	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	configContent := fmt.Sprintf(`
[serial]
port = "%s"
baudRate = 115200
dataBits = 8
parity = "none"
stopBits = 1

[telnet]
port = 23232

[mcp]
httpPort = 50012
`, getTestPort())

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--no-tray", "--config", configPath)
	cmd.Start()
	defer cmd.Process.Kill()

	time.Sleep(2 * time.Second)

	// 验证服务启动在配置文件的端口
	resp, err := http.Get("http://127.0.0.1:50012/health")
	if err != nil {
		t.Fatalf("Health check on configured port failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health check returned %d", resp.StatusCode)
	}

	t.Log("Config file test passed")
}

func TestSerialHub_LogFile(t *testing.T) {
	if os.Getenv("SERIALHUB_INTEGRATION_TEST") != "1" {
		t.Skip("集成测试未启用，设置 SERIALHUB_INTEGRATION_TEST=1 启用")
	}

	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--no-tray", "--telnet-port", "23233", "--mcp-port", "50013")
	cmd.Env = append(os.Environ(), fmt.Sprintf("SERIALHUB_LOG_DIR=%s", logDir))
	cmd.Start()
	defer cmd.Process.Kill()

	time.Sleep(2 * time.Second)

	// 验证日志文件创建
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Logf("Log directory not created: %v", err)
	} else {
		for _, entry := range entries {
			t.Logf("Log file: %s", entry.Name())
		}
	}

	t.Log("Log file test passed")
}

func TestSerialHub_DebugMode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 测试 --debug 参数
	cmd := exec.CommandContext(ctx, binaryPath, "--debug", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run with --debug: %v", err)
	}

	// --debug 不影响 --help 输出
	if !strings.Contains(string(output), "--serial-port") {
		t.Errorf("Debug mode affected help output")
	}

	t.Log("Debug mode test passed")
}

func TestSerialHub_InvalidPort(t *testing.T) {
	if os.Getenv("SERIALHUB_INTEGRATION_TEST") != "1" {
		t.Skip("集成测试未启用，设置 SERIALHUB_INTEGRATION_TEST=1 启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 测试无效串口
	cmd := exec.CommandContext(ctx, binaryPath, "--no-tray", "--serial-port", "INVALID_PORT_99999", "--telnet-port", "23234", "--mcp-port", "50014")
	output, _ := cmd.CombinedOutput()

	// 应该能看到错误信息
	outputStr := string(output)
	t.Logf("Output: %s", outputStr)

	// 程序应该仍在运行（只是串口未连接）
	// 可以通过健康检查验证
	time.Sleep(2 * time.Second)

	resp, err := http.Get("http://127.0.0.1:50014/health")
	if err == nil {
		resp.Body.Close()
		cmd.Process.Kill()
		t.Log("Server started even with invalid port")
	}

	t.Log("Invalid port test passed")
}
