package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dongly/serialhub/internal/buffer"
	"github.com/dongly/serialhub/internal/instance"
	"github.com/dongly/serialhub/pkg/mcp/tools"
	"github.com/dongly/serialhub/pkg/serial"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestNewMCPServer_NilSerialManager(t *testing.T) {
	buf := buffer.NewDataBuffer()
	server, err := NewMCPServer(nil, buf)
	if err != nil {
		t.Fatalf("预期无错误，实际: %v", err)
	}
	if server == nil {
		t.Fatal("预期 server 非 nil")
	}
	if server.serialManager != nil {
		t.Error("serialManager 应为 nil")
	}
}

func TestNewMCPServer_NormalCreation(t *testing.T) {
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

func TestNewMCPServer_AutoCreateBuffer(t *testing.T) {
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
	if headers != "Content-Type, Mcp-Session-Id, Mcp-Protocol-Version, Authorization, Last-Event-ID" {
		t.Errorf("Access-Control-Allow-Headers 预期含 MCP 请求头，实际: %s", headers)
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
	httpServer, err := server.StartHTTPServer(addr, false)
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

	// JSON 解析断言（含 side/sideDetail 字段；Windows/WSL 双实例并存时客户端靠它区分）
	var health struct {
		Status     string `json:"status"`
		Service    string `json:"service"`
		Role       string `json:"role"`
		Side       string `json:"side"`
		SideDetail string `json:"sideDetail"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("解析 health 响应失败: %v（响应: %s）", err, string(body))
	}
	if health.Status != "ok" || health.Service != "serialhub" || health.Role != "master" {
		t.Errorf("health 字段异常: %+v", health)
	}
	if health.Side != instance.LocalSide() {
		t.Errorf("side 预期 %q，实际 %q", instance.LocalSide(), health.Side)
	}
	if health.SideDetail != instance.SideDetail() {
		t.Errorf("sideDetail 预期 %q，实际 %q", instance.SideDetail(), health.SideDetail)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type 预期包含 'application/json'，实际: %s", contentType)
	}

	// 测试 version 端点
	resp2, err := http.Get("http://" + addr + "/version")
	if err != nil {
		t.Fatalf("请求 version 端点失败: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("version 端点预期状态码 200，实际: %d", resp2.StatusCode)
	}

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("读取 version 响应体失败: %v", err)
	}

	// 验证返回的版本号格式
	bodyStr := strings.TrimSpace(string(body2))
	if !strings.HasPrefix(bodyStr, `{"version":"`) || !strings.HasSuffix(bodyStr, `"}`) {
		t.Errorf("version 响应格式不正确，实际: '%s'", bodyStr)
	}

	// 验证版本号不为空
	if strings.Contains(bodyStr, `""`) {
		t.Errorf("version 响应中版本号为空")
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

		// 成功时文本应为合法 JSON（规范 SHOULD：structuredContent 的
		// 回退文本块为序列化 JSON），且包含 message 与数据字段
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(textContent.Text), &parsed); err != nil {
			t.Fatalf("成功结果文本应为合法 JSON，实际: %s（错误: %v）", textContent.Text, err)
		}
		if parsed["message"] != "操作成功" {
			t.Errorf("JSON 文本 message 字段应为 '操作成功'，实际: %v", parsed["message"])
		}
		if parsed["port"] != getTestPort() {
			t.Errorf("JSON 文本应包含端口字段，实际: %s", textContent.Text)
		}
		// isError 应为 false（omitempty 序列化时省略）
		if mcpResult.IsError {
			t.Error("成功结果 IsError 应为 false")
		}
		// structuredContent 应包含 Data
		if mcpResult.StructuredContent == nil {
			t.Error("成功结果 StructuredContent 不应为 nil")
		}
		if sc, ok := mcpResult.StructuredContent.(map[string]interface{}); ok {
			if sc["port"] != getTestPort() {
				t.Errorf("StructuredContent 应包含端口，实际: %v", sc)
			}
		} else {
			t.Errorf("StructuredContent 类型应为对象，实际 %T", mcpResult.StructuredContent)
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
		// 失败时 IsError 应为 true，向客户端标记执行失败
		if !mcpResult.IsError {
			t.Error("失败结果 IsError 应为 true")
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

	t.Run("invalidParamsError返回-32602", func(t *testing.T) {
		err := server.invalidParamsError(fmt.Errorf("无法解析参数"))
		if err == nil {
			t.Fatal("invalidParamsError 不应返回 nil")
		}
		jrErr, ok := err.(*jsonrpc.Error)
		if !ok {
			t.Fatalf("错误类型应为 *jsonrpc.Error，实际 %T", err)
		}
		if jrErr.Code != jsonrpc.CodeInvalidParams {
			t.Errorf("错误码应为 -32602，实际 %d", jrErr.Code)
		}
		if !strings.Contains(jrErr.Message, "参数解析错误") {
			t.Errorf("错误消息应包含 '参数解析错误'，实际: %s", jrErr.Message)
		}
	})
}

func TestStreamableHTTPHandler(t *testing.T) {
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
	httpServer, err := server.StartHTTPServer(addr, false)
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

	t.Run("Stateless直接调用serial_list工具", func(t *testing.T) {
		reqBody := `{
			"jsonrpc": "2.0",
			"method": "tools/call",
			"params": {
				"name": "serial_list"
			},
			"id": 1
		}`

		req, err := http.NewRequest("POST", "http://"+addr+"/mcp", strings.NewReader(reqBody))
		if err != nil {
			t.Fatalf("创建请求失败: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST /mcp 失败: %v", err)
		}
		defer resp.Body.Close()

		// 验证状态码 200（Stateless 模式，无需 session ID）
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("预期状态码 200，实际: %d，响应: %s", resp.StatusCode, string(body))
		}

		// 验证 Content-Type 为 JSON
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			t.Errorf("Content-Type 预期包含 'application/json'，实际: %s", contentType)
		}

		// 验证响应体包含 result
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("读取响应体失败: %v", err)
		}

		bodyStr := string(body)
		if !strings.Contains(bodyStr, "result") {
			t.Errorf("响应应包含 'result'，实际: %s", bodyStr)
		}
	})

	t.Run("Health端点仍然工作", func(t *testing.T) {
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

		var health struct {
			Status     string `json:"status"`
			Service    string `json:"service"`
			Role       string `json:"role"`
			Side       string `json:"side"`
			SideDetail string `json:"sideDetail"`
		}
		if err := json.Unmarshal(body, &health); err != nil {
			t.Fatalf("解析 health 响应失败: %v（响应: %s）", err, string(body))
		}
		if health.Status != "ok" || health.Service != "serialhub" || health.Role != "master" {
			t.Errorf("health 字段异常: %+v", health)
		}
		if health.Side != instance.LocalSide() {
			t.Errorf("side 预期 %q，实际 %q", instance.LocalSide(), health.Side)
		}
		if health.SideDetail != instance.SideDetail() {
			t.Errorf("sideDetail 预期 %q，实际 %q", instance.SideDetail(), health.SideDetail)
		}
	})
}

func TestToolHandlers(t *testing.T) {
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
	httpServer, err := server.StartHTTPServer(addr, false)
	if err != nil {
		t.Fatalf("启动 HTTP 服务器失败: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
	})

	time.Sleep(100 * time.Millisecond)

	callTool := func(name string, arguments map[string]interface{}) (map[string]interface{}, error) {
		reqBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": name,
			},
			"id": 1,
		}
		if arguments != nil {
			reqBody["params"] = map[string]interface{}{
				"name":      name,
				"arguments": arguments,
			}
		}

		jsonBody, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "http://"+addr+"/mcp", strings.NewReader(string(jsonBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		return result, nil
	}

	t.Run("serial_connect工具", func(t *testing.T) {
		result, err := callTool("serial_connect", map[string]interface{}{"port": getTestPort()})
		if err != nil {
			t.Fatalf("调用 serial_connect 失败: %v", err)
		}
		if result["result"] == nil && result["error"] == nil {
			t.Error("响应应包含 result 或 error")
		}
	})

	t.Run("serial_status工具", func(t *testing.T) {
		result, err := callTool("serial_status", nil)
		if err != nil {
			t.Fatalf("调用 serial_status 失败: %v", err)
		}
		if result["result"] == nil {
			t.Error("响应应包含 result")
		}
	})

	t.Run("serial_disconnect工具", func(t *testing.T) {
		result, err := callTool("serial_disconnect", nil)
		if err != nil {
			t.Fatalf("调用 serial_disconnect 失败: %v", err)
		}
		if result["result"] == nil && result["error"] == nil {
			t.Error("响应应包含 result 或 error")
		}
	})

	t.Run("serial_write工具", func(t *testing.T) {
		result, err := callTool("serial_write", map[string]interface{}{"data": "test"})
		if err != nil {
			t.Fatalf("调用 serial_write 失败: %v", err)
		}
		if result["result"] == nil && result["error"] == nil {
			t.Error("响应应包含 result 或 error")
		}
	})
}

// runStdioConnectionToolsListProbe 建立一对内存传输，用给定握手状态启动
// RunStdioConnection，然后不经 initialize 直接发一条 tools/list，
// 返回响应（用于断言状态恢复是否生效）。
func runStdioConnectionToolsListProbe(t *testing.T, state *mcpsdk.ServerSessionState) *jsonrpc.Response {
	t.Helper()
	srv, err := NewMCPServer(newTestSerialManager(t), buffer.NewDataBuffer())
	if err != nil {
		t.Fatalf("创建 MCP 服务失败: %v", err)
	}
	if err := srv.RegisterTools(); err != nil {
		t.Fatalf("注册工具失败: %v", err)
	}

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverConn, err := serverTransport.Connect(ctx)
	if err != nil {
		t.Fatalf("建立服务端连接失败: %v", err)
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- srv.RunStdioConnection(ctx, &StdioHandoff{Conn: serverConn, State: state})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-serveDone:
		case <-time.After(2 * time.Second):
		}
	})

	clientConn, err := clientTransport.Connect(ctx)
	if err != nil {
		t.Fatalf("建立客户端连接失败: %v", err)
	}
	req, err := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	if err != nil {
		t.Fatalf("构造 tools/list 请求失败: %v", err)
	}
	if err := clientConn.Write(ctx, req); err != nil {
		t.Fatalf("发送 tools/list 失败: %v", err)
	}
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()
	msg, err := clientConn.Read(readCtx)
	if err != nil {
		t.Fatalf("读取 tools/list 响应失败: %v", err)
	}
	resp, ok := msg.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("期望 Response, 实际 %T", msg)
	}
	return resp
}

// TestRunStdioConnection_RestoresSessionState：原地接管必须复用代理捕获的
// 握手状态——客户端已在旧主上完成 initialize，接管后不会重发 initialize；
// 若新会话状态为空，tools/list 会被 SDK 以「未完成初始化」拒绝。
func TestRunStdioConnection_RestoresSessionState(t *testing.T) {
	resp := runStdioConnectionToolsListProbe(t, &mcpsdk.ServerSessionState{
		InitializeParams:  &mcpsdk.InitializeParams{ProtocolVersion: "2025-06-18"},
		InitializedParams: &mcpsdk.InitializedParams{},
	})
	if resp.Error != nil {
		t.Fatalf("恢复握手状态后 tools/list 不应报错，实际: %v", resp.Error)
	}
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("解析 tools/list 结果失败: %v", err)
	}
	if len(result.Tools) == 0 {
		t.Fatal("tools/list 应返回已注册工具")
	}
}

// TestRunStdioConnection_RejectsToolsListWithoutState：反向对照——不恢复
// 握手状态时 tools/list 必被拒绝，证明上一测试确实覆盖了状态恢复路径。
func TestRunStdioConnection_RejectsToolsListWithoutState(t *testing.T) {
	resp := runStdioConnectionToolsListProbe(t, nil)
	if resp.Error == nil {
		t.Fatal("未恢复握手状态时 tools/list 应被拒绝")
	}
}

// TestRunStdioConnection_ResendsUndeliveredResponse 原地接管时必须先把旧代理
// 没写出去的下行响应补发给客户端（客户端仍在等这条请求的响应）。
func TestRunStdioConnection_ResendsUndeliveredResponse(t *testing.T) {
	srv, err := NewMCPServer(newTestSerialManager(t), buffer.NewDataBuffer())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.RegisterTools(); err != nil {
		t.Fatal(err)
	}
	undelivered, err := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":42,"result":{"tools":[]}}`))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	serverConn, err := serverTransport.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- srv.RunStdioConnection(ctx, &StdioHandoff{
			Conn: serverConn,
			State: &mcpsdk.ServerSessionState{
				InitializeParams:  &mcpsdk.InitializeParams{ProtocolVersion: "2025-06-18"},
				InitializedParams: &mcpsdk.InitializedParams{},
			},
			Undelivered: undelivered,
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-serveDone:
		case <-time.After(2 * time.Second):
		}
	})

	clientConn, err := clientTransport.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()
	msg, err := clientConn.Read(readCtx)
	if err != nil {
		t.Fatalf("读取补发响应失败: %v", err)
	}
	resp, ok := msg.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("期望补发 Response, 实际 %T", msg)
	}
	raw, err := jsonrpc.EncodeMessage(resp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"id":42`) {
		t.Fatalf("补发的应是 id=42 那条响应, 实际 %s", raw)
	}
}
