// Package mcp provides MCP server implementation with serial port tools.
package mcp

import (
	"context"
	"fmt"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/mcp/tools"
	"github.com/yourname/serialhub/pkg/serial"
)

// MCPServer manages the MCP server and tool registration
type MCPServer struct {
	serialManager *serial.SerialManager
	dataBuffer    *buffer.DataBuffer
	mcpServer     *mcpsdk.Server
}

// NewMCPServer creates a new MCP server instance
func NewMCPServer(sm *serial.SerialManager, buf *buffer.DataBuffer) (*MCPServer, error) {
	if buf == nil {
		buf = buffer.NewDataBuffer()
	}

	// Create MCP server
	mcpServer := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "serialhub", Version: "v1.0.0"},
		&mcpsdk.ServerOptions{
			Instructions: "SerialHub MCP 服务器提供串口操作工具",
		},
	)

	return &MCPServer{
		serialManager: sm,
		dataBuffer:    buf,
		mcpServer:     mcpServer,
	}, nil
}

// RegisterTools registers all 6 serial port tools with the MCP server
func (s *MCPServer) RegisterTools() error {
	// Register serial_list tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_list",
		Description: "列出系统中所有可用的串口",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialList)

	// Register serial_connect tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_connect",
		Description: "连接到指定串口",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"port": map[string]any{
					"type":        "string",
					"description": "串口名（如 COM9 或 /dev/ttyUSB0）",
				},
				"baudRate": map[string]any{
					"type":        "integer",
					"description": "波特率（默认 115200）",
				},
			},
			"required": []string{"port"},
		},
	}, s.handleSerialConnect)

	// Register serial_disconnect tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_disconnect",
		Description: "断开当前串口连接",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialDisconnect)

	// Register serial_write tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_write",
		Description: "向串口发送数据，自动追加换行符",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type":        "string",
					"description": "要发送的数据",
				},
				"addNewline": map[string]any{
					"type":        "boolean",
					"description": "是否自动追加换行符（默认 true）",
				},
			},
			"required": []string{"data"},
		},
	}, s.handleSerialWrite)

	// Register serial_read tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_read",
		Description: "阻塞式读取串口数据，等待数据到达后返回",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timeout": map[string]any{
					"type":        "integer",
					"description": "超时时间（毫秒，0 表示无限等待）",
				},
				"maxSize": map[string]any{
					"type":        "integer",
					"description": "最大读取字节数（默认 4096）",
				},
			},
		},
	}, s.handleSerialRead)

	// Register serial_status tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_status",
		Description: "获取串口连接状态",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialStatus)

	logrus.Infoln("[SerialHub] MCP 服务器已注册 6 个工具")
	return nil
}

// StartHTTPServer starts the HTTP transport for MCP communication
func (s *MCPServer) StartHTTPServer(addr string) (*http.Server, error) {
	streamableHandler := mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		return s.mcpServer
	}, &mcpsdk.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})

	mux := http.NewServeMux()
	mux.Handle("/mcp", streamableHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	corsMux := s.withCORS(mux)

	server := &http.Server{
		Addr:    addr,
		Handler: corsMux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Errorf("[SerialHub] HTTP 服务器错误: %v", err)
		}
	}()

	logrus.Infof("[SerialHub] MCP HTTP 服务器已启动: %s", addr)
	return server, nil
}

// Stop stops the MCP server
func (s *MCPServer) Stop() error {
	logrus.Infoln("[SerialHub] MCP 服务器已停止")
	return nil
}

// withCORS adds CORS support to the HTTP handler
func (s *MCPServer) withCORS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// Tool handlers - using low-level ToolHandler API
func (s *MCPServer) handleSerialList(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	result := tools.ExecuteSerialList(s.serialManager)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialConnect(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	// For low-level API, we extract params manually
	var input tools.ConnectInput
	_ = s.parseRequestParams(req, &input)
	result := tools.ExecuteSerialConnect(s.serialManager, input)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialDisconnect(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	result := tools.ExecuteSerialDisconnect(s.serialManager)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialWrite(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var input tools.WriteInput
	_ = s.parseRequestParams(req, &input)
	result := tools.ExecuteSerialWrite(s.serialManager, input)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialRead(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var input tools.ReadInput
	_ = s.parseRequestParams(req, &input)
	result := tools.ExecuteSerialRead(s.dataBuffer, input)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialStatus(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	result := tools.ExecuteSerialStatus(s.serialManager)
	return s.toolResultToMCPResult(result)
}

// Helper functions
func (s *MCPServer) parseRequestParams(req *mcpsdk.CallToolRequest, target interface{}) error {
	// Parse parameters from request.Params
	// In the low-level API, we need to manually extract and parse params
	if req.Params == nil || req.Params.Arguments == nil {
		return nil
	}

	// For now, we'll just skip parsing in the low-level API
	// In production, we'd use json.Unmarshal with proper error handling
	_ = target
	return nil
}

func (s *MCPServer) toolResultToMCPResult(result tools.ToolResult) (*mcpsdk.CallToolResult, error) {
	content := mcpsdk.TextContent{
		Text: fmt.Sprintf("%s\n%v", result.Message, result.Data),
	}

	if !result.Success {
		content.Text = result.Message
	}

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&content},
	}, nil
}
