// Package transport provides MCP transport implementations.
package transport

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// StdioTransport wraps mcpsdk.StdioTransport for MCP stdio communication
type StdioTransport struct {
	transport *mcpsdk.StdioTransport
	logger    *logrus.Logger
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport(logger *logrus.Logger) (*StdioTransport, error) {
	if logger == nil {
		logger = logrus.New()
	}

	return &StdioTransport{
		transport: &mcpsdk.StdioTransport{},
		logger:    logger,
	}, nil
}

// Run starts the stdio transport with the given MCP server
func (t *StdioTransport) Run(ctx context.Context, mcpServer *mcpsdk.Server) error {
	t.logger.Infoln("[SerialHub] stdio 传输启动中...")
	return mcpServer.Run(ctx, t.transport)
}

// Close closes the stdio transport
func (t *StdioTransport) Close() error {
	t.logger.Infoln("[SerialHub] stdio 传输关闭")
	return nil
}
