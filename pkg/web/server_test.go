package web

import (
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

func TestWebSocketServer_SingleConnection(t *testing.T) {
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

	if srv.ClientCount() != 1 {
		t.Fatalf("期望 ClientCount=1（踢掉旧连接）, 实际=%d", srv.ClientCount())
	}

	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn1.ReadMessage()
	if err == nil {
		t.Fatal("旧连接应该已被关闭")
	}

	count := srv.Broadcast([]byte("to new client"))
	if count != 1 {
		t.Fatalf("期望 Broadcast 返回 1, 实际=%d", count)
	}

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn2.ReadMessage()
	if err != nil {
		t.Fatalf("新客户端读取失败: %v", err)
	}
	if string(data) != "to new client" {
		t.Fatalf("新客户端期望收到 %q, 实际收到 %q", "to new client", string(data))
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

func TestWebSocketServer_Start(t *testing.T) {
	srv, err := NewWebSocketServer("127.0.0.1", 8080)
	if err != nil {
		t.Fatalf("创建服务器失败: %v", err)
	}
	defer srv.Stop()

	if err := srv.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
}
