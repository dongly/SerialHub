package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dongly/serialhub/internal/i18n"
	"github.com/dongly/serialhub/internal/testutil"
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
		return &mcpsdk.IOTransport{Reader: pr, Writer: testutil.NopWriteCloser{Writer: io.Discard}}
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

// proxyOutcome 汇总一次 RunStdioProxy 的返回，便于在 goroutine 中传递。
type proxyOutcome struct {
	reason  ProxyExitReason
	err     error
	handoff *StdioHandoff
}

// startProxyAsync 异步启动 RunStdioProxy，返回接收结果的 channel。
func startProxyAsync(ctx context.Context, masterURL string, handoff *StdioHandoff) <-chan proxyOutcome {
	out := make(chan proxyOutcome, 1)
	go func() {
		reason, h, err := RunStdioProxy(ctx, masterURL, handoff)
		out <- proxyOutcome{reason, err, h}
	}()
	return out
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
	outcome := startProxyAsync(ctx, ts.URL, nil)

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
	reason, handoff, err := RunStdioProxy(context.Background(), "http://127.0.0.1:1", nil)
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

	reason, handoff, err := RunStdioProxy(context.Background(), "http://127.0.0.1:1", nil)
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
	outcome := startProxyAsync(ctx, ts.URL, nil)

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
	outcome := startProxyAsync(ctx, "http://127.0.0.1:1", nil)

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

// emitOnceConn 主实例侧替身：内嵌 stubConn，只覆写 Read——首次返回预置消息
// （模拟主实例主动推送），之后委托底层阻塞到取消/关闭。用于构造
// 「向 stdio 写回时客户端已断开」的场景。
type emitOnceConn struct {
	*stubConn
	msg jsonrpc.Message
}

func (c *emitOnceConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	if c.msg != nil {
		m := c.msg
		c.msg = nil
		return m, nil
	}
	return c.stubConn.Read(ctx)
}

// TestRunStdioProxy_BrokenPipeIsNormalExit 用真实 OS 管道验证：客户端关闭读端后
// 写回得到系统级 broken pipe（Linux 为 syscall.EPIPE），必须判定为正常退出——
// 不带错误、也不交出可接管的连接。此处 innerCtx 尚未取消，因此 err==nil 的
// 断言真正约束了 EPIPE 被识别，而非被「自身取消」掩盖。
func TestRunStdioProxy_BrokenPipeIsNormalExit(t *testing.T) {
	origStdio, origMaster := stdioTransportFactory, masterTransportFactory
	t.Cleanup(func() { stdioTransportFactory, masterTransportFactory = origStdio, origMaster })

	// stdio 读端：永不写入也永不关闭，使 client→master goroutine 稳定阻塞
	pipeR, pipeW := io.Pipe()
	// stdio 写端：真实 OS 管道，读端立即关闭 → 写回必得 broken pipe
	prd, pwd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := prd.Close(); err != nil {
		t.Fatal(err)
	}
	msg, err := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	stdioTransportFactory = func() mcpsdk.Transport {
		return &mcpsdk.IOTransport{Reader: pipeR, Writer: pwd}
	}
	masterTransportFactory = func(string) mcpsdk.Transport {
		return &stubTransport{conn: &emitOnceConn{stubConn: newStubConn(), msg: msg}}
	}
	t.Cleanup(func() {
		_ = pipeR.Close()
		_ = pipeW.Close()
		_ = pwd.Close()
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	select {
	case r := <-startProxyAsync(ctx, ts.URL, nil):
		if r.reason != ProxyStdioClosed {
			t.Fatalf("reason = %q, 期望 stdioClosed", r.reason)
		}
		if r.err != nil {
			t.Fatalf("客户端 broken pipe 属正常退出，不应带错误，实际 %v", r.err)
		}
		if r.handoff != nil {
			t.Fatal("客户端已断开，不得交出可接管的 stdio 连接")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("写回 broken pipe 后应在超时前判定为正常退出")
	}
}

// TestResolveProxyExit_PrefersClientClose 表驱动验证退出语义优先级：
// 客户端已关闭 stdio 时一律正常退出，即使「主失联」的结果先到也不接管。
func TestResolveProxyExit_PrefersClientClose(t *testing.T) {
	lostErr := errors.New("主实例读取结束")

	if reason, err := resolveProxyExit(proxyResult{ProxyMasterLost, lostErr}, true); reason != ProxyStdioClosed || err != nil {
		t.Fatalf("clientGone=true 应正常退出，实际 reason=%v err=%v", reason, err)
	}
	if reason, err := resolveProxyExit(proxyResult{ProxyMasterLost, lostErr}, false); reason != ProxyMasterLost || err != lostErr {
		t.Fatalf("clientGone=false 应保留可接管语义，实际 reason=%v err=%v", reason, err)
	}
	if reason, err := resolveProxyExit(proxyResult{ProxyStdioClosed, nil}, false); reason != ProxyStdioClosed || err != nil {
		t.Fatalf("stdio 正常结束应原样透传，实际 reason=%v err=%v", reason, err)
	}
}

// blockingWriteConn 内嵌 stubConn，只覆写 Write：阻塞到 ctx 取消后返回错误。
// 用于模拟「对端接受了连接却不去读消息」造成的写阻塞。
type blockingWriteConn struct{ *stubConn }

func newBlockingWriteConn() *blockingWriteConn {
	return &blockingWriteConn{stubConn: newStubConn()}
}

func (c *blockingWriteConn) Write(ctx context.Context, _ jsonrpc.Message) error {
	<-ctx.Done()
	return ctx.Err()
}

// withMasterConn 只替换主实例侧传输（stdio 侧由调用方经 handoff.Conn 提供），
// 并在测试结束时还原。
func withMasterConn(t *testing.T, conn mcpsdk.Connection) {
	t.Helper()
	orig := masterTransportFactory
	masterTransportFactory = func(string) mcpsdk.Transport {
		return &stubTransport{conn: conn}
	}
	t.Cleanup(func() { masterTransportFactory = orig })
}

// TestRunStdioProxy_PendingReplayIsProbeProtected 交回消息的重放必须在探活
// （及超时）约束之内：旧实现在探活启动前用无超时 ctx 同步重放，主实例若接受
// 连接却不读请求，代理会永久卡住——既不有界重试也不正常退出。
func TestRunStdioProxy_PendingReplayIsProbeProtected(t *testing.T) {
	origInterval, origFailures := masterProbeInterval, masterProbeFailures
	masterProbeInterval, masterProbeFailures = 60*time.Millisecond, 2
	t.Cleanup(func() {
		masterProbeInterval, masterProbeFailures = origInterval, origFailures
	})

	withMasterConn(t, newBlockingWriteConn())

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := ts.URL
	ts.Close() // 主实例失联，探活应在 60ms×2 内判定

	req, err := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	handoff := &StdioHandoff{Conn: newStubConn(), Pending: req}

	select {
	case r := <-startProxyAsync(context.Background(), url, handoff):
		if r.reason != ProxyMasterLost {
			t.Fatalf("reason = %q, 期望 masterLost", r.reason)
		}
		if r.handoff == nil || r.handoff.Conn == nil {
			t.Fatal("masterLost 必须交出连接供接管")
		}
		if r.handoff.Pending == nil {
			t.Fatal("重放未成功时必须保留 pending，交后续阶段继续携带")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("重放阻塞未受探活/超时约束：代理永久卡住")
	}
}

// TestRunStdioProxy_KeepsUndeliveredResponse 从主实例读到但未写回客户端的下行
// 响应必须留住并在接管后补发：否则客户端会一直等这条请求的响应（如 initialize）。
func TestRunStdioProxy_KeepsUndeliveredResponse(t *testing.T) {
	origInterval, origFailures := masterProbeInterval, masterProbeFailures
	masterProbeInterval, masterProbeFailures = 60*time.Millisecond, 2
	t.Cleanup(func() {
		masterProbeInterval, masterProbeFailures = origInterval, origFailures
	})

	resp, err := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":9,"result":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	withMasterConn(t, &emitOnceConn{stubConn: newStubConn(), msg: resp})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := ts.URL
	ts.Close()

	// stdio 侧写阻塞到取消：客户端仍在线（Read 阻塞），只是这条响应没出去。
	handoff := &StdioHandoff{Conn: newBlockingWriteConn()}

	select {
	case r := <-startProxyAsync(context.Background(), url, handoff):
		if r.reason != ProxyMasterLost {
			t.Fatalf("reason = %q, 期望 masterLost", r.reason)
		}
		if r.handoff == nil || r.handoff.Undelivered == nil {
			t.Fatal("必须保留未写回客户端的下行响应以供补发")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("代理未在有限时间内退出")
	}
}

// recordingConn 记录写入的消息（主实例侧替身）：用于断言 Undelivered 补发。
type recordingConn struct {
	*stubConn

	mu  sync.Mutex
	got []jsonrpc.Message
}

func (c *recordingConn) Write(_ context.Context, m jsonrpc.Message) error {
	c.mu.Lock()
	c.got = append(c.got, m)
	c.mu.Unlock()
	return nil
}

func (c *recordingConn) messages() []jsonrpc.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]jsonrpc.Message(nil), c.got...)
}

// TestRunStdioProxy_ResendsUndeliveredWhenReProxying 接管撞上他进程抢先成主
// 而退回代理模式时，上轮未写回客户端的下行响应必须在新一轮连接建立后立即
// 补发（否则客户端该请求会悬挂至自身超时）。
func TestRunStdioProxy_ResendsUndeliveredWhenReProxying(t *testing.T) {
	master := &recordingConn{stubConn: newStubConn()}
	orig := masterTransportFactory
	masterTransportFactory = func(string) mcpsdk.Transport {
		return &stubTransport{conn: master}
	}
	t.Cleanup(func() { masterTransportFactory = orig })

	resp, err := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":11,"result":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	handoff := &StdioHandoff{Conn: newStubConn(), Undelivered: resp}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := ts.URL
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 主实例健康 → 代理不退出；给补发留出时间窗口后检查主实例收到的消息。
	select {
	case r := <-startProxyAsync(ctx, url, handoff):
		t.Fatalf("主实例健康时代理不应退出, 实际 reason=%q err=%v", r.reason, r.err)
	case <-time.After(200 * time.Millisecond):
	}
	got := master.messages()
	if len(got) == 0 {
		t.Fatal("未观察到任何补发消息")
	}
	raw, err := jsonrpc.EncodeMessage(got[len(got)-1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"id":11`) {
		t.Fatalf("补发的应是 id=11 的下行响应, 实际 %s", raw)
	}
}
