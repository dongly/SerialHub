package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
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
	origStdio, origMaster := stdioTransportFactory, masterTransportFactory
	pr, pw := io.Pipe()
	conn := newStubConn()
	stdioTransportFactory = func() mcpsdk.Transport {
		return &mcpsdk.IOTransport{Reader: pr, Writer: nopWriteCloser{io.Discard}}
	}
	masterTransportFactory = func(string) mcpsdk.Transport {
		return &stubTransport{conn: conn}
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
		reason ProxyExitReason
		err    error
		conn   mcpsdk.Connection
	}
	outcome := make(chan proxyOutcome, 1)
	go func() {
		reason, err, conn := RunStdioProxy(ctx, ts.URL, nil)
		outcome <- proxyOutcome{reason, err, conn}
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
		if r.err == nil || !strings.Contains(r.err.Error(), "健康探测") {
			t.Fatalf("err = %v, 期望包含「健康探测」", r.err)
		}
		if r.conn == nil {
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
	reason, err, conn := RunStdioProxy(context.Background(), "http://127.0.0.1:1", nil)
	if reason != ProxyMasterLost {
		t.Fatalf("reason = %q, 期望 masterLost", reason)
	}
	if err == nil {
		t.Fatal("期望返回连接/探活错误")
	}
	if conn == nil {
		t.Fatal("masterLost 时必须交出存续的 stdio 连接（原地接管复用）")
	}
}

// TestRunStdioProxy_StdioClosed：MCP 客户端关闭 stdio 属正常退出，
// 必须返回 stdioClosed（上层不得据此接管）。
func TestRunStdioProxy_StdioClosed(t *testing.T) {
	origStdio, origMaster := stdioTransportFactory, masterTransportFactory
	pr, pw := io.Pipe()
	_ = pw
	stdioTransportFactory = func() mcpsdk.Transport {
		return &mcpsdk.IOTransport{Reader: pr, Writer: nopWriteCloser{io.Discard}}
	}
	masterTransportFactory = func(string) mcpsdk.Transport {
		return &stubTransport{conn: newStubConn()}
	}
	t.Cleanup(func() {
		stdioTransportFactory, masterTransportFactory = origStdio, origMaster
		pr.Close()
	})
	pw.Close() // MCP 客户端断开 → Read 立即 EOF

	reason, err, conn := RunStdioProxy(context.Background(), "http://127.0.0.1:1", nil)
	if reason != ProxyStdioClosed {
		t.Fatalf("reason = %q, 期望 stdioClosed", reason)
	}
	if err == nil {
		t.Fatal("期望返回 stdio 读结束错误")
	}
	if conn != nil {
		t.Fatal("stdioClosed 时连接已关闭，不得交出（避免接管误用）")
	}
}
