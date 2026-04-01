// Package transport tests MCP transport implementations.
package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNewStdioTransport 测试创建 stdio 传输
func TestNewStdioTransport(t *testing.T) {
	transport, err := NewStdioTransport(nil)
	if err != nil {
		t.Errorf("创建 stdio 传输失败: %v", err)
	}

	if transport == nil {
		t.Fatal("stdio 传输不应为 nil")
	}

	if transport.logger == nil {
		t.Error("logger 不应为 nil，应创建默认 logger")
	}

	if transport.transport == nil {
		t.Error("transport 不应为 nil")
	}
}

// TestStdioTransportClose 测试关闭 stdio 传输
func TestStdioTransportClose(t *testing.T) {
	transport, err := NewStdioTransport(nil)
	if err != nil {
		t.Fatalf("创建 stdio 传输失败: %v", err)
	}

	// Close should not panic
	err = transport.Close()
	if err != nil {
		t.Errorf("关闭传输失败: %v", err)
	}
}

// TestNewHTTPHandler 测试创建 HTTP 处理器
func TestNewHTTPHandler(t *testing.T) {
	mcpServer := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "test", Version: "v1.0.0"},
		nil,
	)

	handler, server, err := NewHTTPHandler(mcpServer, ":0", nil)
	if err != nil {
		t.Errorf("创建 HTTP 处理器失败: %v", err)
	}

	if handler == nil {
		t.Fatal("HTTP 处理器不应为 nil")
	}

	if handler.logger == nil {
		t.Error("logger 不应为 nil，应创建默认 logger")
	}

	if handler.mux == nil {
		t.Error("mux 不应为 nil")
	}

	if handler.sseHandler == nil {
		t.Error("sseHandler 不应为 nil")
	}

	if server == nil {
		t.Fatal("HTTP 服务器不应为 nil")
	}

	// Test server configuration
	if server.Addr != ":0" {
		t.Errorf("地址不正确，期望 ':0'，实际 '%s'", server.Addr)
	}
}

// TestHandleHealth 测试健康检查端点
func TestHandleHealth(t *testing.T) {
	// Test GET request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("期望 Content-Type 'application/json'，实际 '%s'", contentType)
	}

	expectedBody := "{\"status\":\"ok\"}"
	body := w.Body.String()
	if body != expectedBody {
		t.Errorf("响应体不正确，期望 '%s'，实际 '%s'", expectedBody, body)
	}

	// Test POST request (should fail)
	req = httptest.NewRequest(http.MethodPost, "/health", nil)
	w = httptest.NewRecorder()

	handleHealth(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// TestHandleSSE 测试 SSE 端点
func TestHandleSSE(t *testing.T) {
	// Test GET request
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	w := httptest.NewRecorder()

	handleSSE(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("期望 Content-Type 'text/event-stream'，实际 '%s'", contentType)
	}

	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl != "no-cache" {
		t.Errorf("期望 Cache-Control 'no-cache'，实际 '%s'", cacheControl)
	}

	// Test POST request (should fail)
	req = httptest.NewRequest(http.MethodPost, "/sse", nil)
	w = httptest.NewRecorder()

	handleSSE(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// TestWithCORS 测试 CORS 支持
func TestWithCORS(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Use the CORS wrapper
	corsHandler := withCORS(mux)
	corsHandler.ServeHTTP(w, req)

	// Check CORS headers
	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("期望 Access-Control-Allow-Origin '*'，实际 '%s'", origin)
	}

	methods := w.Header().Get("Access-Control-Allow-Methods")
	if methods != "GET, POST, OPTIONS" {
		t.Errorf("期望 Access-Control-Allow-Methods 'GET, POST, OPTIONS'，实际 '%s'", methods)
	}

	headers := w.Header().Get("Access-Control-Allow-Headers")
	if headers != "Content-Type" {
		t.Errorf("期望 Access-Control-Allow-Headers 'Content-Type'，实际 '%s'", headers)
	}
}

// TestWithCORSPreflight 测试 CORS 预检请求
func TestWithCORSPreflight(t *testing.T) {
	mux := http.NewServeMux()

	// Create OPTIONS request
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	w := httptest.NewRecorder()

	corsHandler := withCORS(mux)
	corsHandler.ServeHTTP(w, req)

	// OPTIONS request should return 200 OK
	if w.Code != http.StatusOK {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, w.Code)
	}

	// Check CORS headers
	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("期望 Access-Control-Allow-Origin '*'，实际 '%s'", origin)
	}
}

// TestHTTPHandlerClose 测试关闭 HTTP 服务器
func TestHTTPHandlerClose(t *testing.T) {
	mcpServer := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "test", Version: "v1.0.0"},
		nil,
	)

	handler, server, err := NewHTTPHandler(mcpServer, ":0", nil)
	if err != nil {
		t.Fatalf("创建 HTTP 处理器失败: %v", err)
	}

	// Use server.Handler as the HTTP handler for test server
	testServer := httptest.NewServer(server.Handler)
	defer testServer.Close()

	// Verify server is running
	resp, err := http.Get(testServer.URL + "/health")
	if err != nil {
		t.Fatalf("HTTP GET 失败: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望状态码 %d，实际 %d", http.StatusOK, resp.StatusCode)
	}

	// Test graceful shutdown
	err = handler.Close(server)
	if err != nil {
		t.Errorf("关闭服务器失败: %v", err)
	}

	// Server should be stopped
	time.Sleep(100 * time.Millisecond)
}

// TestHTTPServerTimeouts 测试服务器超时配置
func TestHTTPServerTimeouts(t *testing.T) {
	mcpServer := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "test", Version: "v1.0.0"},
		nil,
	)

	handler, server, err := NewHTTPHandler(mcpServer, ":0", nil)
	if err != nil {
		t.Fatalf("创建 HTTP 处理器失败: %v", err)
	}

	if handler == nil {
		t.Fatal("HTTP 处理器不应为 nil")
	}

	if server == nil {
		t.Fatal("HTTP 服务器不应为 nil")
	}

	// Verify timeout configurations
	expectedReadTimeout := 30 * time.Second
	if server.ReadTimeout != expectedReadTimeout {
		t.Errorf("期望 ReadTimeout %v，实际 %v", expectedReadTimeout, server.ReadTimeout)
	}

	expectedWriteTimeout := 30 * time.Second
	if server.WriteTimeout != expectedWriteTimeout {
		t.Errorf("期望 WriteTimeout %v，实际 %v", expectedWriteTimeout, server.WriteTimeout)
	}

	expectedIdleTimeout := 60 * time.Second
	if server.IdleTimeout != expectedIdleTimeout {
		t.Errorf("期望 IdleTimeout %v，实际 %v", expectedIdleTimeout, server.IdleTimeout)
	}
}
