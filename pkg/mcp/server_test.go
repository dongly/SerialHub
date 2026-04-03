package mcp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/mcp/tools"
	"github.com/yourname/serialhub/pkg/serial"
)

func getTestPort() string {
	port := os.Getenv("SERIALHUB_TEST_PORT")
	if port == "" {
		port = "COM9"
	}
	return port
}

// newTestSerialManager 创建用于测试的 SerialManager
func newTestSerialManager(t *testing.T) *serial.SerialManager {
	t.Helper()
	cfg := serial.DefaultConfig()
	cfg.Port = getTestPort()
	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("创建 SerialManager 失败: %v", err)
	}
	t.Cleanup(func() {
		sm.Close()
	})
	return sm
}

// findFreePort 获取一个可用的随机端口地址
func findFreePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("获取随机端口失败: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()
	return addr
}

func TestNewMCPServer_串口管理器为空(t *testing.T) {
	buf := buffer.NewDataBuffer()
	server, err := NewMCPServer(nil, buf)
	if server != nil {
		t.Error("预期 server 为 nil")
	}
	if err == nil {
		t.Fatal("预期返回错误")
	}
	if !strings.Contains(err.Error(), "串口管理器不能为空") {
		t.Errorf("错误消息应包含 '串口管理器不能为空'，实际: %s", err.Error())
	}
}

func TestNewMCPServer_正常创建(t *testing.T) {
	sm := newTestSerialManager(t)
	buf := buffer.NewDataBuffer()

	server, err := NewMCPServer(sm, buf)
	if err != nil {
		t.Fatalf("预期无错误，实际: %v", err)
	}
	if server == nil {
		t.Fatal("预期 server 非 nil")
	}
	if server.serialManager == nil {
		t.Error("serialManager 不应为 nil")
	}
	if server.dataBuffer == nil {
		t.Error("dataBuffer 不应为 nil")
	}
}

func TestNewMCPServer_Buffer为空时自动创建(t *testing.T) {
	sm := newTestSerialManager(t)

	server, err := NewMCPServer(sm, nil)
	if err != nil {
		t.Fatalf("预期无错误，实际: %v", err)
	}
	if server == nil {
		t.Fatal("预期 server 非 nil")
	}
	if server.dataBuffer == nil {
		t.Error("dataBuffer 应被自动创建，不应为 nil")
	}
}

func TestRegisterTools(t *testing.T) {
	sm := newTestSerialManager(t)
	buf := buffer.NewDataBuffer()

	server, err := NewMCPServer(sm, buf)
	if err != nil {
		t.Fatalf("创建 MCPServer 失败: %v", err)
	}

	// RegisterTools 不应返回错误也不应 panic
	err = server.RegisterTools()
	if err != nil {
		t.Errorf("注册工具失败: %v", err)
	}

	// 验证 mcpServer 已创建（通过检查它不是 nil）
	if server.mcpServer == nil {
		t.Error("注册工具后 mcpServer 不应为 nil")
	}
}

func TestWithCORS(t *testing.T) {
	sm := newTestSerialManager(t)
	buf := buffer.NewDataBuffer()

	server, err := NewMCPServer(sm, buf)
	if err != nil {
		t.Fatalf("创建 MCPServer 失败: %v", err)
	}

	// 创建一个简单的 handler 用于测试
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("inner response"))
	})

	corsHandler := server.withCORS(innerHandler)

	// 测试 OPTIONS 请求
	t.Run("OPTIONS请求", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		rec := httptest.NewRecorder()
		corsHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("OPTIONS 预期状态码 200，实际: %d", rec.Code)
		}
		// OPTIONS 请求不应转发到内部 handler
		body := rec.Body.String()
		if body != "" {
			t.Errorf("OPTIONS 预期空响应体，实际: %s", body)
		}
		// 验证 CORS 头
		checkCORSHeaders(t, rec)
	})

	// 测试 GET 请求
	t.Run("GET请求转发", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		corsHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET 预期状态码 200，实际: %d", rec.Code)
		}
		body := rec.Body.String()
		if body != "inner response" {
			t.Errorf("GET 预期响应 'inner response'，实际: %s", body)
		}
		// 验证 CORS 头
		checkCORSHeaders(t, rec)
	})

	// 测试 POST 请求
	t.Run("POST请求转发", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		rec := httptest.NewRecorder()
		corsHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("POST 预期状态码 200，实际: %d", rec.Code)
		}
		body := rec.Body.String()
		if body != "inner response" {
			t.Errorf("POST 预期响应 'inner response'，实际: %s", body)
		}
		checkCORSHeaders(t, rec)
	})
}

func checkCORSHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("Access-Control-Allow-Origin 预期 '*'，实际: %s", origin)
	}
	methods := rec.Header().Get("Access-Control-Allow-Methods")
	if methods != "GET, POST, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods 预期 'GET, POST, OPTIONS'，实际: %s", methods)
	}
	headers := rec.Header().Get("Access-Control-Allow-Headers")
	if headers != "Content-Type" {
		t.Errorf("Access-Control-Allow-Headers 预期 'Content-Type'，实际: %s", headers)
	}
}

func TestStartHTTPServer_HealthEndpoint(t *testing.T) {
	sm := newTestSerialManager(t)
	buf := buffer.NewDataBuffer()

	server, err := NewMCPServer(sm, buf)
	if err != nil {
		t.Fatalf("创建 MCPServer 失败: %v", err)
	}

	err = server.RegisterTools()
	if err != nil {
		t.Fatalf("注册工具失败: %v", err)
	}

	addr := findFreePort(t)
	httpServer, err := server.StartHTTPServer(addr)
	if err != nil {
		t.Fatalf("启动 HTTP 服务器失败: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
	})

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 测试 health 端点
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("请求 health 端点失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("预期状态码 200，实际: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}

	expected := `{"status":"ok"}`
	if strings.TrimSpace(string(body)) != expected {
		t.Errorf("预期响应 '%s'，实际: '%s'", expected, string(body))
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type 预期包含 'application/json'，实际: %s", contentType)
	}
}

func TestStop(t *testing.T) {
	sm := newTestSerialManager(t)
	buf := buffer.NewDataBuffer()

	server, err := NewMCPServer(sm, buf)
	if err != nil {
		t.Fatalf("创建 MCPServer 失败: %v", err)
	}

	// Stop 不应返回错误
	err = server.Stop()
	if err != nil {
		t.Errorf("Stop 预期无错误，实际: %v", err)
	}

	// 重复调用 Stop 也不应出错
	err = server.Stop()
	if err != nil {
		t.Errorf("重复 Stop 预期无错误，实际: %v", err)
	}
}

func TestToolResultToMCPResult(t *testing.T) {
	sm := newTestSerialManager(t)
	buf := buffer.NewDataBuffer()

	server, err := NewMCPServer(sm, buf)
	if err != nil {
		t.Fatalf("创建 MCPServer 失败: %v", err)
	}

	t.Run("成功结果", func(t *testing.T) {
		result := tools.ToolResult{
			Success: true,
			Message: "操作成功",
			Data:    map[string]interface{}{"port": getTestPort()},
		}

		mcpResult, err := server.toolResultToMCPResult(result)
		if err != nil {
			t.Fatalf("toolResultToMCPResult 返回错误: %v", err)
		}
		if mcpResult == nil {
			t.Fatal("预期结果非 nil")
		}
		if len(mcpResult.Content) == 0 {
			t.Fatal("预期 Content 不为空")
		}

		textContent, ok := mcpResult.Content[0].(*mcpsdk.TextContent)
		if !ok {
			t.Fatal("Content[0] 类型应为 *TextContent")
		}

		// 成功时文本应包含 Message 和 Data
		if !strings.Contains(textContent.Text, "操作成功") {
			t.Errorf("成功结果文本应包含消息，实际: %s", textContent.Text)
		}
		if !strings.Contains(textContent.Text, getTestPort()) {
			t.Errorf("成功结果文本应包含 Data，实际: %s", textContent.Text)
		}
	})

	t.Run("失败结果", func(t *testing.T) {
		result := tools.ToolResult{
			Success: false,
			Message: "串口未连接",
		}

		mcpResult, err := server.toolResultToMCPResult(result)
		if err != nil {
			t.Fatalf("toolResultToMCPResult 返回错误: %v", err)
		}
		if mcpResult == nil {
			t.Fatal("预期结果非 nil")
		}

		textContent, ok := mcpResult.Content[0].(*mcpsdk.TextContent)
		if !ok {
			t.Fatal("Content[0] 类型应为 *TextContent")
		}

		// 失败时文本应只包含 Message
		if textContent.Text != "串口未连接" {
			t.Errorf("失败结果文本应只包含消息 '串口未连接'，实际: %s", textContent.Text)
		}
	})

	t.Run("成功结果无Data", func(t *testing.T) {
		result := tools.ToolResult{
			Success: true,
			Message: "执行完毕",
			Data:    nil,
		}

		mcpResult, err := server.toolResultToMCPResult(result)
		if err != nil {
			t.Fatalf("toolResultToMCPResult 返回错误: %v", err)
		}

		textContent, ok := mcpResult.Content[0].(*mcpsdk.TextContent)
		if !ok {
			t.Fatal("Content[0] 类型应为 *TextContent")
		}

		if !strings.Contains(textContent.Text, "执行完毕") {
			t.Errorf("成功结果文本应包含消息，实际: %s", textContent.Text)
		}
	})
}
