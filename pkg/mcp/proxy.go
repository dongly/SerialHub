package mcp

import (
	"context"
	"fmt"
	"os"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// RunStdioProxy 以 stdio 透明代理模式运行：本进程不提供任何服务，
// 将 stdin/stdout 上的 MCP 消息逐条转发给已运行主实例的 HTTP /mcp 端点。
//
// 适用场景：MCP 客户端（如 OpenCode）以 local/stdio 方式拉起本进程，
// 而主实例已在运行（联邦发现命中）。协议握手（initialize）被透明转发，
// 无会话语义依赖主实例的 Stateless 模式。
func RunStdioProxy(ctx context.Context, masterURL string) error {
	endpoint := masterURL + "/mcp"

	stdio := &mcpsdk.StdioTransport{}
	stdioConn, err := stdio.Connect(ctx)
	if err != nil {
		return fmt.Errorf("stdio 传输初始化失败: %w", err)
	}

	client := &mcpsdk.StreamableClientTransport{Endpoint: endpoint}
	clientConn, err := client.Connect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SerialHub 主实例不可达（%s）。\n若主实例使用非默认端口，请改用 remote 模式直连其 URL。\n", endpoint)
		return fmt.Errorf("连接主实例失败: %w", err)
	}

	logrus.Infof("[SerialHub] stdio 代理模式：转发到 %s", endpoint)

	// 双向拷贝 MCP 消息，任一侧结束即退出
	errCh := make(chan error, 2)

	go func() {
		for {
			msg, err := stdioConn.Read(ctx)
			if err != nil {
				errCh <- fmt.Errorf("stdio 读结束: %w", err)
				return
			}
			if err := clientConn.Write(ctx, msg); err != nil {
				errCh <- fmt.Errorf("转发到主实例失败: %w", err)
				return
			}
		}
	}()

	go func() {
		for {
			msg, err := clientConn.Read(ctx)
			if err != nil {
				errCh <- fmt.Errorf("主实例读结束: %w", err)
				return
			}
			if err := stdioConn.Write(ctx, msg); err != nil {
				errCh <- fmt.Errorf("写回 stdio 失败: %w", err)
				return
			}
		}
	}()

	select {
	case err := <-errCh:
		logrus.Infof("[SerialHub] stdio 代理退出: %v", err)
		return nil
	case <-ctx.Done():
		logrus.Info("[SerialHub] stdio 代理退出：上下文取消")
		return nil
	}
}
