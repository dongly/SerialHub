package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/dongly/serialhub/pkg/web"
)

// blockingWriter 模拟日志输出链卡死：Write 永远阻塞直到 release 关闭。
type blockingWriter struct {
	release chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.release
	return len(p), nil
}

// TestFlushBoundedLog_ReturnsOnScheduleWhenBlocked 验证停机收尾的有界性：
// 日志 writer 卡死（stdout 背压/文件锁等价物）时，flushBoundedLog 不得
// 永久阻塞控制线程，应在 ~500ms 超时后返回（防止未来把同步日志移回
// 控制线程导致强退路径到不了 os.Exit）。
func TestFlushBoundedLog_ReturnsOnScheduleWhenBlocked(t *testing.T) {
	release := make(chan struct{})
	blocked := &blockingWriter{release: release}

	std := logrus.StandardLogger()
	origOut := std.Out
	logrus.SetOutput(blocked)
	defer func() {
		// 先释放卡住的 Write（其持有 logger 锁），再恢复输出
		close(release)
		logrus.SetOutput(origOut)
	}()

	start := time.Now()
	flushBoundedLog(logrus.InfoLevel, "停机通知（应被阻塞但有界返回）")
	elapsed := time.Since(start)

	if elapsed >= time.Second {
		t.Errorf("日志输出阻塞时 flushBoundedLog 应在 ~500ms 返回，实际 %s", elapsed)
	}
	if elapsed < 450*time.Millisecond {
		t.Errorf("flushBoundedLog 过早返回（%s）：应等到 500ms 超时才放弃", elapsed)
	}
}

// TestGracefulShutdown_NotifiesWebClients：实例停机前应向已连接的
// Web 终端广播 serverStopping 系统事件，客户端据此预告连接即将断开。
func TestGracefulShutdown_NotifiesWebClients(t *testing.T) {
	wsSrv, err := web.NewWebSocketServer("127.0.0.1", 0)
	if err != nil {
		t.Fatalf("创建 WebSocketServer 失败: %v", err)
	}
	defer wsSrv.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsSrv.HandleWebSocket)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer conn.Close()

	// 读掉欢迎消息（纯文本帧），停机广播才会被读到
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, welcome, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取欢迎消息失败: %v", err)
	}
	if !strings.Contains(string(welcome), "Connected to SerialHub") {
		t.Fatalf("意外的欢迎消息: %q", welcome)
	}

	done := make(chan struct{})
	go func() {
		gracefulShutdown(nil, &runningServices{wsSrv: wsSrv})
		close(done)
	}()

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var msg struct {
		Type string `json:"type"`
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("读取停机预告失败: %v", err)
	}
	if msg.Type != "system_event" || msg.Data.Code != "serverStopping" {
		t.Fatalf("意外的停机预告: %+v", msg)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("gracefulShutdown 未在期限内返回")
	}
}

// timedBlockWriter 卡住 Write 一段时间后放行：制造日志阻塞窗口以验证
// 「停机预告先于停机日志写出」，又不至于触发 gracefulShutdown 的 3s
// 超时强退（该分支 os.Exit，无法在测试进程内触达）。
type timedBlockWriter struct{ block time.Duration }

func (w *timedBlockWriter) Write(p []byte) (int, error) {
	time.Sleep(w.block)
	return len(p), nil
}

// TestGracefulShutdown_NotifiesDespiteBlockedLogger：日志输出链阻塞
// （stdout 背压/文件锁等价物）时，停机预告仍必须先于日志写出——已连接
// 终端在阻塞窗口内就收到 serverStopping，而不是等日志放行之后。
// 注意：WS 接入路径（HandleWebSocket）含同步日志，客户端须在阻塞前
// 完成接入，否则接入本身会被拖慢（与本测试目标无关）。
func TestGracefulShutdown_NotifiesDespiteBlockedLogger(t *testing.T) {
	wsSrv, err := web.NewWebSocketServer("127.0.0.1", 0)
	if err != nil {
		t.Fatalf("创建 WebSocketServer 失败: %v", err)
	}
	defer wsSrv.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsSrv.HandleWebSocket)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer conn.Close()

	// 先完成接入并读掉欢迎消息（此时日志尚畅通）
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("读取欢迎消息失败: %v", err)
	}

	// 每条日志阻塞 1.2s：若预告被排在停机日志之后，客户端读取耗时
	// 必然 ≥1.2s；正确实现下预告先于日志发出，远早于该窗口。
	const block = 1200 * time.Millisecond
	std := logrus.StandardLogger()
	origOut := std.Out
	logrus.SetOutput(&timedBlockWriter{block: block})
	defer logrus.SetOutput(origOut)

	start := time.Now()
	done := make(chan struct{})
	go func() {
		gracefulShutdown(nil, &runningServices{wsSrv: wsSrv})
		close(done)
	}()

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var msg struct {
		Type string `json:"type"`
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("日志阻塞时读取停机预告失败: %v", err)
	}
	if msg.Type != "system_event" || msg.Data.Code != "serverStopping" {
		t.Fatalf("意外的停机预告: %+v", msg)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("预告到达过晚（%v）：疑似被排到了停机日志之后", elapsed)
	}

	// 日志放行后停机 goroutine 正常收尾（无超时强退）
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("gracefulShutdown 未在期限内返回")
	}
}
