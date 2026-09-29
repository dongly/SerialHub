package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
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

// RunStdioProxy 以 stdio 透明代理模式运行，返回退出原因、错误与 stdio 连接。
//
// stdioConn 参数：nil 时自行连接 stdin/stdout；非 nil 时复用调用方已持有的
// 连接（上轮代理交回的，避免二次连接产生第二个 stdin reader）。
//
// MasterLost 时返回的连接仍在存续（未关闭）：调用方可原地接管为主实例，
// 继续使用同一 reader goroutine；其余退出原因返回 nil 连接。
func RunStdioProxy(ctx context.Context, masterURL string, stdioConn mcpsdk.Connection) (ProxyExitReason, error, mcpsdk.Connection) {
	endpoint := masterURL + "/mcp"

	if stdioConn == nil {
		stdio := stdioTransportFactory()
		conn, err := stdio.Connect(ctx)
		if err != nil {
			return ProxyStdioClosed, fmt.Errorf("stdio 传输初始化失败: %w", err), nil
		}
		stdioConn = conn
	}

	client := masterTransportFactory(endpoint)
	clientConn, err := client.Connect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SerialHub 主实例不可达（%s）。\n若主实例使用非默认端口，请改用 remote 模式直连其 URL。\n", endpoint)
		return ProxyMasterLost, fmt.Errorf("连接主实例失败: %w", err), stdioConn
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
				logrus.Debugf("[SerialHub] 主实例健康探测失败（%d/%d）: %v", failures, masterProbeFailures, err)
				if failures >= masterProbeFailures {
					errCh <- proxyResult{ProxyMasterLost, fmt.Errorf("主实例健康探测连续失败: %v", err)}
					return
				}
			}
		}
	}()

	go func() {
		for {
			msg, err := stdioConn.Read(innerCtx)
			if err != nil {
				errCh <- proxyResult{ProxyStdioClosed, fmt.Errorf("stdio 读结束: %w", err)}
				return
			}
			if err := clientConn.Write(innerCtx, msg); err != nil {
				errCh <- proxyResult{ProxyMasterLost, fmt.Errorf("转发到主实例失败: %w", err)}
				return
			}
		}
	}()

	go func() {
		for {
			msg, err := clientConn.Read(innerCtx)
			if err != nil {
				errCh <- proxyResult{ProxyMasterLost, fmt.Errorf("主实例读结束: %w", err)}
				return
			}
			if err := stdioConn.Write(innerCtx, msg); err != nil {
				errCh <- proxyResult{ProxyStdioClosed, fmt.Errorf("写回 stdio 失败: %w", err)}
				return
			}
		}
	}()

	select {
	case r := <-errCh:
		logrus.Infof("[SerialHub] stdio 代理退出（%s）: %v", r.reason, r.err)
		innerCancel()
		_ = clientConn.Close()
		if r.reason == ProxyMasterLost {
			// 交出仍在存续的 stdio 连接：调用方原地接管后复用同一 reader
			return r.reason, r.err, stdioConn
		}
		return r.reason, r.err, nil
	case <-ctx.Done():
		logrus.Info("[SerialHub] stdio 代理退出：上下文取消")
		innerCancel()
		_ = clientConn.Close()
		return ProxyStdioClosed, nil, nil
	}
}
