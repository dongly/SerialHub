package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/dongly/serialhub/internal/i18n"
)

// RunStdioProxy 以 stdio 透明代理模式运行：本进程不提供任何服务，
// 将 stdin/stdout 上的 MCP 消息逐条转发给已运行主实例的 HTTP /mcp 端点。
//
// 适用场景：MCP 客户端（如 OpenCode）以 local/stdio 方式拉起本进程，
// 而主实例已在运行（instance lock 发现命中）。协议握手（initialize）被透明转发，
// 无会话语义依赖主实例的 Stateless 模式。
//
// 返回值 reason 区分退出原因：MCP 客户端关闭 stdio（ProxyStdioClosed，
// 正常结束）或与主实例失联（ProxyMasterLost，调用方可尝试接管为主）。
type ProxyExitReason string

const (
	// ProxyStdioClosed 表示 MCP 客户端侧的 stdio 已关闭，属正常退出。
	ProxyStdioClosed ProxyExitReason = "stdioClosed"
	// ProxyMasterLost 表示与主实例的连接断开或建立失败，可尝试接管。
	ProxyMasterLost ProxyExitReason = "masterLost"
)

// proxyResult 携带单个转发 goroutine 的退出原因与错误。
type proxyResult struct {
	reason ProxyExitReason
	err    error
}

// StdioHandoff 是代理切换身份时移交给下一阶段的完整 stdio 状态。
// Pending 是旧代理已完整读出、但尚未成功转发的一条消息；接管为主或
// 重新连接新主时必须先处理它，避免请求在身份切换窗口丢失。
type StdioHandoff struct {
	Conn    mcpsdk.Connection
	State   *mcpsdk.ServerSessionState
	Pending jsonrpc.Message
}

type sessionStateTracker struct {
	mu    sync.Mutex
	state mcpsdk.ServerSessionState
}

func newSessionStateTracker(state *mcpsdk.ServerSessionState) *sessionStateTracker {
	t := &sessionStateTracker{}
	if state != nil {
		t.state = *state
	}
	return t
}

func (t *sessionStateTracker) record(msg jsonrpc.Message) {
	req, ok := msg.(*jsonrpc.Request)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch req.Method {
	case "initialize":
		var params mcpsdk.InitializeParams
		if json.Unmarshal(req.Params, &params) == nil {
			t.state.InitializeParams = &params
		}
	case "notifications/initialized":
		var params mcpsdk.InitializedParams
		if len(req.Params) == 0 || json.Unmarshal(req.Params, &params) == nil {
			t.state.InitializedParams = &params
		}
	case "logging/setLevel":
		var params mcpsdk.SetLoggingLevelParams
		if json.Unmarshal(req.Params, &params) == nil {
			t.state.LogLevel = params.Level
		}
	}
}

func (t *sessionStateTracker) snapshot() *mcpsdk.ServerSessionState {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.state
	return &state
}

// 探活节奏与判定阈值；包级变量便于测试缩短周期。
var (
	masterProbeInterval = 2 * time.Second
	masterProbeFailures = 2
)

// stdioTransportFactory / masterTransportFactory 构造两侧传输；
// 包级变量便于测试注入（默认分别使用真实 stdin/stdout 与 Streamable HTTP）。
var (
	stdioTransportFactory  = func() mcpsdk.Transport { return &mcpsdk.StdioTransport{} }
	masterTransportFactory = func(endpoint string) mcpsdk.Transport {
		return &mcpsdk.StreamableClientTransport{Endpoint: endpoint}
	}
)

// stdioGone 判定 stdio 端是否被 MCP 客户端真正关闭：客户端关闭 stdin（EOF）、
// 管道已关闭，或写回时收到 broken pipe（Linux 为 syscall.EPIPE；Windows 由 os
// 包映射为 io.ErrClosedPipe）。不含代理自身取消——那是内部收尾，不是客户端离开。
func stdioGone(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, syscall.EPIPE)
}

// stdioChannelClosed 判定 stdio 侧错误是否属正常退出（无需作为错误上报）：
// 客户端已关闭，或代理自身已取消。
func stdioChannelClosed(ctx context.Context, err error) bool {
	return stdioGone(err) || ctx.Err() != nil
}

// resolveProxyExit 决定代理最终的退出语义：客户端已关闭 stdio 时，即使
// 「主失联」的结果先到，也必须按正常退出处理——客户端已经不在了，此时
// 接管成一个没有客户端的主实例毫无意义，还会白占端口与串口。
func resolveProxyExit(r proxyResult, clientGone bool) (ProxyExitReason, error) {
	if clientGone {
		return ProxyStdioClosed, nil
	}
	return r.reason, r.err
}

// RunStdioProxy 以 stdio 透明代理模式运行，返回退出原因、错误与 stdio 连接。
//
// stdioConn 参数：nil 时自行连接 stdin/stdout；非 nil 时复用调用方已持有的
// 连接（上轮代理交回的，避免二次连接产生第二个 stdin reader）。
//
// MasterLost 时返回的连接仍在存续（未关闭）：调用方可原地接管为主实例，
// 继续使用同一 reader goroutine；其余退出原因返回 nil 连接。
func RunStdioProxy(ctx context.Context, masterURL string, handoff *StdioHandoff) (ProxyExitReason, error, *StdioHandoff) {
	endpoint := masterURL + "/mcp"
	var stdioConn mcpsdk.Connection
	tracker := newSessionStateTracker(nil)
	var pending jsonrpc.Message
	if handoff != nil {
		stdioConn = handoff.Conn
		tracker = newSessionStateTracker(handoff.State)
		pending = handoff.Pending
	}

	if stdioConn == nil {
		stdio := stdioTransportFactory()
		conn, err := stdio.Connect(ctx)
		if err != nil {
			return ProxyStdioClosed, fmt.Errorf(i18n.ServeErrors.ProxyStdioInitFailed, err), nil
		}
		stdioConn = conn
	}

	client := masterTransportFactory(endpoint)
	clientConn, err := client.Connect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.ServeErrors.MasterUnreachableHint, endpoint)
		return ProxyMasterLost, fmt.Errorf(i18n.ServeErrors.ProxyMasterConnectFailed, err), &StdioHandoff{Conn: stdioConn, State: tracker.snapshot(), Pending: pending}
	}
	if pending != nil {
		if err := clientConn.Write(ctx, pending); err != nil {
			_ = clientConn.Close()
			return ProxyMasterLost, fmt.Errorf(i18n.ServeErrors.ProxyForwardFailed, err), &StdioHandoff{Conn: stdioConn, State: tracker.snapshot(), Pending: pending}
		}
		tracker.record(pending)
		pending = nil
	}

	logrus.Infof("[SerialHub] stdio 代理模式：转发到 %s", endpoint)

	// innerCtx 控制转发与探活 goroutine：判定失联后立即停止转发，但不关闭
	// stdioConn——它的底层 reader goroutine 正是接管方要复用的（关了会断
	// os.Stdout 写端，且再无 reader 消费 stdin）。
	innerCtx, innerCancel := context.WithCancel(ctx)
	defer innerCancel()

	// 双向拷贝 MCP 消息，任一侧结束即退出。
	// stdio 侧错误＝客户端关闭；主实例侧错误＝主失联（可接管）。
	errCh := make(chan proxyResult, 4)
	var forwarders sync.WaitGroup
	var pendingMu sync.Mutex

	// clientGone 记录 MCP 客户端是否已真正关闭 stdio（与主失联并发时优先正常退出）。
	var clientGone atomic.Bool

	// Streamable 非流式短连接在空闲时没有活跃 TCP 连接：主实例退出后
	// clientConn.Read 会一直阻塞等数据，感知不到失联。定期探测主实例
	// /health，连续失败即判定失联（上层据此接管为主）。
	probe := time.NewTicker(masterProbeInterval)
	defer probe.Stop()
	healthURL := strings.TrimSuffix(masterURL, "/mcp") + "/health"
	probeClient := &http.Client{Timeout: 2 * time.Second}
	go func() {
		failures := 0
		for {
			select {
			case <-innerCtx.Done():
				return
			case <-probe.C:
				resp, err := probeClient.Get(healthURL)
				if err == nil {
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						failures = 0
						continue
					}
				}
				failures++
				status := ""
				if resp != nil {
					status = resp.Status
				}
				logrus.Debugf("[SerialHub] 主实例健康探测失败（%d/%d）: err=%v status=%s", failures, masterProbeFailures, err, status)
				if failures >= masterProbeFailures {
					if err != nil {
						errCh <- proxyResult{ProxyMasterLost, fmt.Errorf(i18n.ServeErrors.ProxyHealthFailed, err)}
					} else {
						errCh <- proxyResult{ProxyMasterLost, fmt.Errorf(i18n.ServeErrors.ProxyHealthStatus, status)}
					}
					return
				}
			}
		}
	}()

	forwarders.Add(1)
	go func() {
		defer forwarders.Done()
		for {
			msg, err := stdioConn.Read(innerCtx)
			if err != nil {
				// 客户端关闭 stdin（EOF）或代理自身取消属正常退出，
				// 不作为错误上报；仅真实读异常保留错误链。
				if stdioGone(err) {
					clientGone.Store(true)
				}
				if stdioChannelClosed(innerCtx, err) {
					errCh <- proxyResult{ProxyStdioClosed, nil}
				} else {
					errCh <- proxyResult{ProxyStdioClosed, fmt.Errorf(i18n.ServeErrors.ProxyStdioReadEnded, err)}
				}
				return
			}
			if err := clientConn.Write(innerCtx, msg); err != nil {
				pendingMu.Lock()
				pending = msg
				pendingMu.Unlock()
				errCh <- proxyResult{ProxyMasterLost, fmt.Errorf(i18n.ServeErrors.ProxyForwardFailed, err)}
				return
			}
			tracker.record(msg)
		}
	}()

	forwarders.Add(1)
	go func() {
		defer forwarders.Done()
		for {
			msg, err := clientConn.Read(innerCtx)
			if err != nil {
				errCh <- proxyResult{ProxyMasterLost, fmt.Errorf(i18n.ServeErrors.ProxyMasterReadEnded, err)}
				return
			}
			if err := stdioConn.Write(innerCtx, msg); err != nil {
				// 同上：客户端侧通道已结束属正常退出。
				if stdioGone(err) {
					clientGone.Store(true)
				}
				if stdioChannelClosed(innerCtx, err) {
					errCh <- proxyResult{ProxyStdioClosed, nil}
				} else {
					errCh <- proxyResult{ProxyStdioClosed, fmt.Errorf(i18n.ServeErrors.ProxyStdioWriteFailed, err)}
				}
				return
			}
		}
	}()

	select {
	case r := <-errCh:
		// 先过交接屏障再定案：等待期间客户端可能已关闭 stdio，
		// 那时按正常退出处理，绝不交出连接去接管。
		innerCancel()
		_ = clientConn.Close()
		forwarders.Wait()
		reason, err := resolveProxyExit(r, clientGone.Load())
		logrus.Infof("[SerialHub] stdio 代理退出（%s）: %v", reason, err)
		if reason == ProxyMasterLost {
			pendingMu.Lock()
			deferred := pending
			pendingMu.Unlock()
			return reason, err, &StdioHandoff{Conn: stdioConn, State: tracker.snapshot(), Pending: deferred}
		}
		return reason, err, nil
	case <-ctx.Done():
		logrus.Info("[SerialHub] stdio 代理退出：上下文取消")
		innerCancel()
		_ = clientConn.Close()
		forwarders.Wait()
		return ProxyStdioClosed, nil, nil
	}
}
