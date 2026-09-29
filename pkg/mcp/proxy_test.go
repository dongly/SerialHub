package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dongly/serialhub/internal/i18n"
)

// stubConn 模拟一条对端无响应的 MCP 连接：Read 阻塞到 ctx 取消或
// Close，Write 恒成功——用于把「主实例侧」从测试中隔离，聚焦探活逻辑。
type stubConn struct{ done chan struct{} }

func newStubConn() *stubConn { return &stubConn{done: make(chan struct{})} }

func (c *stubConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, errors.New("连接已关闭")
	}
}

func (c *stubConn) Write(context.Context, jsonrpc.Message) error { return nil }

func (c *stubConn) Close() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	return nil
}

func (c *stubConn) SessionID() string { return "" }

// stubTransport 固定返回预置连接。
type stubTransport struct{ conn mcpsdk.Connection }

func (tr *stubTransport) Connect(context.Context) (mcpsdk.Connection, error) {
	return tr.conn, nil
}

// nopWriteCloser 让 io.Discard 满足 IOTransport.Writer 的 io.WriteCloser。
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// injectStubTransports 把两侧传输替换为测试替身：stdio 侧读一条永不
// 关闭的管道（模拟 MCP 客户端保持连接），主实例侧返回 stubConn。
// 返回写端，调用方 Close 它即可模拟 MCP 客户端断开。
func injectStubTransports(t *testing.T) *io.PipeWriter {
	t.Helper()
	return injectStubTransportsWith(t, newStubConn())
}

// injectStubTransportsWith 是 injectStubTransports 的通用版：主实例侧改用
// 调用方提供的连接（如写入必失败的 failWriteConn），用于构造特殊场景。
func injectStubTransportsWith(t *testing.T, masterConn mcpsdk.Connection) *io.PipeWriter {
	t.Helper()
	origStdio, origMaster := stdioTransportFactory, masterTransportFactory
	pr, pw := io.Pipe()
	stdioTransportFactory = func() mcpsdk.Transport {
		return &mcpsdk.IOTransport{Reader: pr, Writer: nopWriteCloser{io.Discard}}
	}
	masterTransportFactory = func(string) mcpsdk.Transport {
		return &stubTransport{conn: masterConn}
	}
	t.Cleanup(func() {
		stdioTransportFactory, masterTransportFactory = origStdio, origMaster
		pr.Close()
	})
	return pw
}

// TestRunStdioProxy_MasterLostByHealthProbe：非流式短连接空闲时无活跃 TCP，
// 代理必须靠 /health 探活感知主实例退出并返回 masterLost（可接管）。
func TestRunStdioProxy_MasterLostByHealthProbe(t *testing.T) {
	origInterval, origFailures := masterProbeInterval, masterProbeFailures
	masterProbeInterval, masterProbeFailures = 60*time.Millisecond, 2
	t.Cleanup(func() { masterProbeInterval, masterProbeFailures = origInterval, origFailures })

	injectStubTransports(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type proxyOutcome struct {
		reason  ProxyExitReason
		err     error
		handoff *StdioHandoff
	}
	outcome := make(chan proxyOutcome, 1)
	go func() {
		reason, err, handoff := RunStdioProxy(ctx, ts.URL, nil)
		outcome <- proxyOutcome{reason, err, handoff}
	}()

	// 主实例健在：至少数个探活周期内不得误报退出
	select {
	case r := <-outcome:
		t.Fatalf("主实例健在时代理不应退出: reason=%v err=%v", r.reason, r.err)
	case <-time.After(300 * time.Millisecond):
	}

	ts.Close() // 模拟主实例死亡

	select {
	case r := <-outcome:
		if r.reason != ProxyMasterLost {
			t.Fatalf("reason = %q, 期望 masterLost", r.reason)
		}
		// 错误文案已本地化（i18n），断言时取词条 %w 之前的前缀，
		// 避免 SERIALHUB_LANG=en 时中文子串断言误报。
		healthPrefix := strings.SplitN(i18n.ServeErrors.ProxyHealthFailed, "%w", 2)[0]
		if r.err == nil || !strings.Contains(r.err.Error(), healthPrefix) {
			t.Fatalf("err = %v, 期望包含「%s」", r.err, healthPrefix)
		}
		if r.handoff == nil || r.handoff.Conn == nil {
			t.Fatal("masterLost 时必须交出存续的 stdio 连接（原地接管复用）")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("主实例死亡后 3 秒内未判定失联")
	}
}

// TestRunStdioProxy_MasterConnectFailed：连不上主实例时返回 masterLost，
// 让上层有机会接管（而不是当成普通错误退出）。
func TestRunStdioProxy_MasterConnectFailed(t *testing.T) {
	origInterval, origFailures := masterProbeInterval, masterProbeFailures
	masterProbeInterval, masterProbeFailures = 50*time.Millisecond, 2
	t.Cleanup(func() { masterProbeInterval, masterProbeFailures = origInterval, origFailures })

	injectStubTransports(t)

	// 端口 1 几乎不可能有服务；无论 Connect 立即失败还是探活兜底，
	// 都必须返回 masterLost。
	reason, err, handoff := RunStdioProxy(context.Background(), "http://127.0.0.1:1", nil)
	if reason != ProxyMasterLost {
		t.Fatalf("reason = %q, 期望 masterLost", reason)
	}
	if err == nil {
		t.Fatal("期望返回连接/探活错误")
	}
	if handoff == nil || handoff.Conn == nil {
		t.Fatal("masterLost 时必须交出存续的 stdio 连接（原地接管复用）")
	}
}

// TestRunStdioProxy_StdioClosed：MCP 客户端关闭 stdio 属正常退出，
// 必须返回 stdioClosed 且不带错误（上层不得据此接管）。
func TestRunStdioProxy_StdioClosed(t *testing.T) {
	pw := injectStubTransports(t)
	pw.Close() // MCP 客户端断开 → Read 立即 EOF

	reason, err, handoff := RunStdioProxy(context.Background(), "http://127.0.0.1:1", nil)
	if reason != ProxyStdioClosed {
		t.Fatalf("reason = %q, 期望 stdioClosed", reason)
	}
	if err != nil {
		t.Fatalf("stdio 正常关闭不应作为错误退出, 实际 %v", err)
	}
	if handoff != nil {
		t.Fatal("stdioClosed 时连接已关闭，不得交出（避免接管误用）")
	}
}

// failWriteConn 复用 stubConn 的读/关生命周期，只把 Write 改为恒失败——
// 用于触发「已完整读出但转发失败」的 pending 保留路径。
type failWriteConn struct{ *stubConn }

func newFailWriteConn() *failWriteConn { return &failWriteConn{stubConn: newStubConn()} }

func (c *failWriteConn) Write(context.Context, jsonrpc.Message) error {
	return errors.New("主实例写入失败")
}

// TestSessionStateTracker_RecordsHandshake：代理必须从转发的消息中捕获
// initialize / notifications/initialized / logging/setLevel，供接管后的
// 主服务恢复会话（客户端在接管后不会重新 initialize）。
func TestSessionStateTracker_RecordsHandshake(t *testing.T) {
	tr := newSessionStateTracker(nil)

	initParams, err := json.Marshal(mcpsdk.InitializeParams{
		ProtocolVersion: "2025-06-18",
		ClientInfo:      &mcpsdk.Implementation{Name: "test", Version: "v1"},
	})
	if err != nil {
		t.Fatalf("序列化 InitializeParams 失败: %v", err)
	}
	tr.record(&jsonrpc.Request{Method: "initialize", Params: initParams})
	if st := tr.snapshot(); st.InitializeParams == nil {
		t.Fatal("必须捕获 initialize 参数")
	} else if st.InitializeParams.ProtocolVersion != "2025-06-18" {
		t.Fatalf("ProtocolVersion = %q, 期望 2025-06-18", st.InitializeParams.ProtocolVersion)
	}

	// initialized 通知允许不带 params
	tr.record(&jsonrpc.Request{Method: "notifications/initialized"})
	if tr.snapshot().InitializedParams == nil {
		t.Fatal("必须捕获 notifications/initialized（无 params 也应接受）")
	}

	tr.record(&jsonrpc.Request{Method: "logging/setLevel", Params: json.RawMessage(`{"level":"debug"}`)})
	if lv := tr.snapshot().LogLevel; lv != mcpsdk.LoggingLevel("debug") {
		t.Fatalf("LogLevel = %q, 期望 debug", lv)
	}

	// 非 Request 消息与无关方法不得污染状态
	tr.record(&jsonrpc.Request{Method: "tools/list"})
	if tr.snapshot().InitializeParams == nil {
		t.Fatal("捕获的状态不应被无关方法覆盖")
	}
}

// TestPendingConnection_ReplaysPendingFirst：接管后的连接必须先返回旧代理
// 已完整读出但未转发成功的那条消息，再委托底层连接，保证请求不丢失且保序。
func TestPendingConnection_ReplaysPendingFirst(t *testing.T) {
	pending := &jsonrpc.Request{Method: "tools/list"}
	inner := newStubConn()
	conn := &pendingConnection{Connection: inner, pending: pending}

	got, err := conn.Read(context.Background())
	if err != nil {
		t.Fatalf("首次 Read 应返回 pending，实际错误 %v", err)
	}
	if got != jsonrpc.Message(pending) {
		t.Fatalf("首次 Read = %v, 期望 pending 消息", got)
	}

	// 第二次 Read 委托底层：ctx 取消即返回错误（证明 pending 已清空且委托生效）
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := conn.Read(ctx); err == nil {
		t.Fatal("pending 清空后应委托底层连接读（此处应返回 ctx 错误）")
	}
}

// TestRunStdioProxy_CapturesSessionState：客户端经代理写入握手消息后主实例
// 失联，代理交出的 handoff 必须携带捕获到的会话状态。
func TestRunStdioProxy_CapturesSessionState(t *testing.T) {
	origInterval, origFailures := masterProbeInterval, masterProbeFailures
	masterProbeInterval, masterProbeFailures = 60*time.Millisecond, 2
	t.Cleanup(func() { masterProbeInterval, masterProbeFailures = origInterval, origFailures })

	pw := injectStubTransports(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type proxyOutcome struct {
		reason  ProxyExitReason
		err     error
		handoff *StdioHandoff
	}
	outcome := make(chan proxyOutcome, 1)
	go func() {
		reason, err, handoff := RunStdioProxy(ctx, ts.URL, nil)
		outcome <- proxyOutcome{reason, err, handoff}
	}()

	// 经 stdio 管道写入 initialize 与 initialized 通知（代理应原样转发给主）
	initParams, _ := json.Marshal(mcpsdk.InitializeParams{ProtocolVersion: "2025-06-18"})
	initMsg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": json.RawMessage(initParams),
	})
	if _, err := pw.Write(append(initMsg, '\n')); err != nil {
		t.Fatalf("写入 initialize 失败: %v", err)
	}
	if _, err := pw.Write([]byte("{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n")); err != nil {
		t.Fatalf("写入 initialized 失败: %v", err)
	}
	time.Sleep(150 * time.Millisecond) // 等转发 goroutine 记录状态

	ts.Close() // 模拟主实例死亡 → MasterLost 交出 handoff

	select {
	case r := <-outcome:
		if r.reason != ProxyMasterLost {
			t.Fatalf("reason = %q, 期望 masterLost", r.reason)
		}
		if r.handoff == nil || r.handoff.State == nil {
			t.Fatal("handoff 必须携带捕获的会话状态")
		}
		if r.handoff.State.InitializeParams == nil {
			t.Fatal("必须捕获 initialize 参数（客户端接管后不会重新 initialize）")
		}
		if r.handoff.State.InitializedParams == nil {
			t.Fatal("必须捕获 notifications/initialized")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("等待 masterLost 超时")
	}
}

// TestRunStdioProxy_KeepsPendingMessage：客户端请求已完整读出但向主实例
// 转发失败时，必须保留为 handoff.Pending（接管后先处理），不得静默丢弃。
func TestRunStdioProxy_KeepsPendingMessage(t *testing.T) {
	pw := injectStubTransportsWith(t, newFailWriteConn())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type proxyOutcome struct {
		reason  ProxyExitReason
		err     error
		handoff *StdioHandoff
	}
	outcome := make(chan proxyOutcome, 1)
	go func() {
		reason, err, handoff := RunStdioProxy(ctx, "http://127.0.0.1:1", nil)
		outcome <- proxyOutcome{reason, err, handoff}
	}()

	time.Sleep(50 * time.Millisecond) // 等代理开始转发
	if _, err := pw.Write([]byte("{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"tools/list\"}\n")); err != nil {
		t.Fatalf("写入请求失败: %v", err)
	}

	select {
	case r := <-outcome:
		if r.reason != ProxyMasterLost {
			t.Fatalf("reason = %q, 期望 masterLost", r.reason)
		}
		if r.handoff == nil || r.handoff.Conn == nil {
			t.Fatal("masterLost 必须交出 stdio 连接")
		}
		if r.handoff.Pending == nil {
			t.Fatal("转发失败前已完整读出的消息必须保留为 Pending")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("等待 masterLost 超时")
	}
}
