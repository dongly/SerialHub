// Package mcp provides MCP server implementation with serial port tools.
package mcp

import (
	"context"
	"encoding/json"
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
			Instructions: "SerialHub MCP server provides serial port operation tools. Use serial_list to discover ports, serial_connect to open a port, then serial_write/serial_read to communicate.",
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
		Description: "List all available serial ports on the system",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialList)

	// Register serial_connect tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_connect",
		Description: "Connect to a specified serial port",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"port": map[string]any{
					"type":        "string",
					"description": "Serial port name (e.g., COM4, /dev/ttyUSB0)",
				},
				"baudRate": map[string]any{
					"type":        "integer",
					"description": "Baud rate (default: 115200)",
				},
			},
			"required": []string{"port"},
		},
	}, s.handleSerialConnect)

	// Register serial_disconnect tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_disconnect",
		Description: "Disconnect from the current serial port",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialDisconnect)

	// Register serial_write tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_write",
		Description: "Write data to the serial port",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type":        "string",
					"description": "Data to send",
				},
				"addNewline": map[string]any{
					"type":        "boolean",
					"description": "Whether to automatically append newline (default: true)",
				},
			},
			"required": []string{"data"},
		},
	}, s.handleSerialWrite)

	// Register serial_read tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_read",
		Description: "Read data from the serial port (blocking, waits for data)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in milliseconds (0 = wait indefinitely)",
				},
				"maxSize": map[string]any{
					"type":        "integer",
					"description": "Maximum bytes to read (default: 4096)",
				},
			},
		},
	}, s.handleSerialRead)

	// Register serial_status tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_status",
		Description: "Get serial port connection status",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialStatus)

	logrus.Infoln("[SerialHub] MCP server registered 6 tools")
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
	if req.Params == nil || req.Params.Arguments == nil {
		return nil
	}

	data, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
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
