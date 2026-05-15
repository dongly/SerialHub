package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var testDialer = websocket.Dialer{}

func dialWS(ts *httptest.Server) (*websocket.Conn, error) {
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := testDialer.Dial(wsURL, nil)
	return conn, err
}

func newTestServer(t *testing.T) (*WebSocketServer, *httptest.Server) {
	t.Helper()

	srv, err := NewWebSocketServer("127.0.0.1", 0)
	if err != nil {
		t.Fatalf("创建 WebSocketServer 失败: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.HandleWebSocket)
	ts := httptest.NewServer(mux)
	t.Cleanup(func() { ts.Close() })

	return srv, ts
}

func readWelcome(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取欢迎消息失败: %v", err)
	}
	if !strings.HasPrefix(string(msg), "Connected to SerialHub") {
		t.Fatalf("期望欢迎消息，收到: %q", string(msg))
	}
}

func TestWebSocketServer_DataChan(t *testing.T) {
	srv, ts := newTestServer(t)
	defer srv.Stop()

	ch := srv.DataChan()
	if ch == nil {
		t.Fatal("DataChan() 返回 nil")
	}

	conn, err := dialWS(ts)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer conn.Close()

	readWelcome(t, conn)

	testMsg := "hello serial"
	err = conn.WriteMessage(websocket.TextMessage, []byte(testMsg))
	if err != nil {
		t.Fatalf("写入消息失败: %v", err)
	}

	select {
	case data := <-ch:
		if string(data) != testMsg {
			t.Fatalf("期望收到 %q, 实际收到 %q", testMsg, string(data))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待数据超时")
	}
}

func TestWebSocketServer_Broadcast(t *testing.T) {
	srv, ts := newTestServer(t)
	defer srv.Stop()

	conn, err := dialWS(ts)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer conn.Close()

	readWelcome(t, conn)

	if srv.ClientCount() != 1 {
		t.Fatalf("期望 ClientCount=1, 实际=%d", srv.ClientCount())
	}

	count := srv.Broadcast([]byte("from serial"))
	if count != 1 {
		t.Fatalf("期望 Broadcast 返回 1, 实际=%d", count)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}

	if string(data) != "from serial" {
		t.Fatalf("期望收到 %q, 实际收到 %q", "from serial", string(data))
	}

	count = srv.Broadcast([]byte("another"))
	if count != 1 {
		t.Fatalf("期望 Broadcast 返回 1, 实际=%d", count)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取第二条消息失败: %v", err)
	}
	if string(data) != "another" {
		t.Fatalf("期望收到 %q, 实际收到 %q", "another", string(data))
	}
}

func TestWebSocketServer_Broadcast_NoClient(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Stop()

	count := srv.Broadcast([]byte("no client"))
	if count != 0 {
		t.Fatalf("期望 Broadcast 返回 0, 实际=%d", count)
	}
}

func TestWebSocketServer_MultipleConnections(t *testing.T) {
	srv, ts := newTestServer(t)
	defer srv.Stop()

	conn1, err := dialWS(ts)
	if err != nil {
		t.Fatalf("第一个连接失败: %v", err)
	}
	defer conn1.Close()

	readWelcome(t, conn1)

	if srv.ClientCount() != 1 {
		t.Fatalf("期望 ClientCount=1, 实际=%d", srv.ClientCount())
	}

	conn2, err := dialWS(ts)
	if err != nil {
		t.Fatalf("第二个连接失败: %v", err)
	}
	defer conn2.Close()

	readWelcome(t, conn2)

	time.Sleep(100 * time.Millisecond)

	// 多客户端模式：两个连接都应该存在
	if srv.ClientCount() != 2 {
		t.Fatalf("期望 ClientCount=2, 实际=%d", srv.ClientCount())
	}

	// 广播消息应该发送到两个客户端
	count := srv.Broadcast([]byte("to all clients"))
	if count != 2 {
		t.Fatalf("期望 Broadcast 返回 2, 实际=%d", count)
	}

	// 验证两个客户端都收到消息
	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data1, err := conn1.ReadMessage()
	if err != nil {
		t.Fatalf("客户端1读取失败: %v", err)
	}
	if string(data1) != "to all clients" {
		t.Fatalf("客户端1期望收到 %q, 实际收到 %q", "to all clients", string(data1))
	}

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data2, err := conn2.ReadMessage()
	if err != nil {
		t.Fatalf("客户端2读取失败: %v", err)
	}
	if string(data2) != "to all clients" {
		t.Fatalf("客户端2期望收到 %q, 实际收到 %q", "to all clients", string(data2))
	}
}

func TestWebSocketServer_Stop(t *testing.T) {
	srv, ts := newTestServer(t)

	conn, err := dialWS(ts)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}

	if srv.ClientCount() != 0 {
		t.Fatalf("Stop 后 ClientCount 应为 0, 实际=%d", srv.ClientCount())
	}
}

func TestWebSocketServer_NewServer_InvalidPort(t *testing.T) {
	_, err := NewWebSocketServer("127.0.0.1", -1)
	if err == nil {
		t.Fatal("期望负端口号返回错误")
	}

	_, err = NewWebSocketServer("127.0.0.1", 70000)
	if err == nil {
		t.Fatal("期望超大端口号返回错误")
	}
}

func TestWebSocketServer_Start_NoOp(t *testing.T) {
	srv, err := NewWebSocketServer("127.0.0.1", 8080)
	if err != nil {
		t.Fatalf("创建服务器失败: %v", err)
	}
	defer srv.Stop()

	if err := srv.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
}

func TestWebSocketServer_DoubleStop(t *testing.T) {
	srv, err := NewWebSocketServer("127.0.0.1", 8080)
	if err != nil {
		t.Fatalf("创建服务器失败: %v", err)
	}

	if err := srv.Stop(); err != nil {
		t.Fatalf("第一次 Stop 失败: %v", err)
	}

	// 第二次 Stop 不应 panic
	if err := srv.Stop(); err != nil {
		t.Fatalf("第二次 Stop 失败: %v", err)
	}
}

func TestTerminalRoute(t *testing.T) {
	srv, err := NewWebSocketServer("127.0.0.1", 0)
	if err != nil {
		t.Fatalf("创建 WebSocketServer 失败: %v", err)
	}
	defer srv.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/terminal", func(w http.ResponseWriter, r *http.Request) {
		data, err := StaticFiles.ReadFile("static/terminal.html")
		if err != nil {
			http.Error(w, "Terminal page not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
	mux.HandleFunc("/ws", srv.HandleWebSocket)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/terminal")
	if err != nil {
		t.Fatalf("请求 /terminal 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望状态码 200, 实际=%d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Fatalf("期望 Content-Type 包含 text/html, 实际=%s", contentType)
	}

	body := make([]byte, 8192)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])

	if !strings.Contains(bodyStr, "xterm.min.js") {
		t.Fatal("响应内容应包含 xterm.min.js")
	}
}

func TestHealthRoute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("请求 /health 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望状态码 200, 实际=%d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Fatalf("期望 Content-Type 包含 application/json, 实际=%s", contentType)
	}

	body := make([]byte, 32)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])

	if !strings.Contains(bodyStr, `{"status":"ok"}`) {
		t.Fatalf("期望响应包含 {\"status\":\"ok\"}, 实际=%s", bodyStr)
	}
}

func TestStaticRoute(t *testing.T) {
	mux := http.NewServeMux()
	staticFS, err := fs.Sub(StaticFiles, "static")
	if err != nil {
		t.Fatalf("获取静态文件系统失败: %v", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/static/xterm.min.js")
	if err != nil {
		t.Fatalf("请求 /static/xterm.min.js 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望状态码 200, 实际=%d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "javascript") && !strings.Contains(contentType, "application") {
		t.Logf("注意: Content-Type=%s", contentType)
	}
}
