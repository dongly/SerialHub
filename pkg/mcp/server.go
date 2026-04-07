// Package mcp provides MCP server implementation with serial port tools.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/mcp/tools"
	"github.com/yourname/serialhub/pkg/serial"
	"github.com/yourname/serialhub/pkg/web"
)

// MCPServer manages the MCP server and tool registration
type MCPServer struct {
	serialManager *serial.SerialManager
	dataBuffer    *buffer.DataBuffer
	mcpServer     *mcpsdk.Server
	wsServer      *web.WebSocketServer
}

// NewMCPServer creates a new MCP server instance
func NewMCPServer(sm *serial.SerialManager, buf *buffer.DataBuffer, wsSrv ...*web.WebSocketServer) (*MCPServer, error) {
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

	s := &MCPServer{
		serialManager: sm,
		dataBuffer:    buf,
		mcpServer:     mcpServer,
	}

	if len(wsSrv) > 0 && wsSrv[0] != nil {
		s.wsServer = wsSrv[0]
	}

	return s, nil
}

// RegisterTools registers all 6 serial port tools with the MCP server
func (s *MCPServer) RegisterTools() error {
	// Register serial_list tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_list",
		Description: "List all available serial ports on the system. Returns a list of port names (e.g., COM1, COM4, /dev/ttyUSB0) that can be used with serial_connect.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialList)

	// Register serial_connect tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_connect",
		Description: "Connect to a specified serial port with configurable baud rate, data bits, parity, and stop bits. Must be called before serial_write or serial_read. Returns success message with connection details or error if port is unavailable.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"port": map[string]any{
					"type":        "string",
					"description": "Serial port name (e.g., COM4, /dev/ttyUSB0). Use serial_list to discover available ports.",
				},
				"baudRate": map[string]any{
					"type":        "integer",
					"description": "Baud rate for communication. Common values: 9600, 19200, 38400, 57600, 115200. Default: 115200.",
				},
				"dataBits": map[string]any{
					"type":        "integer",
					"description": "Number of data bits per frame. Options: 7, 8. Default: 8.",
				},
				"parity": map[string]any{
					"type":        "string",
					"description": "Parity checking mode. Options: 'none', 'even', 'odd'. Default: 'none'.",
				},
				"stopBits": map[string]any{
					"type":        "number",
					"description": "Number of stop bits. Options: 1, 1.5, 2. Default: 1.",
				},
			},
			"required": []string{"port"},
		},
	}, s.handleSerialConnect)

	// Register serial_disconnect tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_disconnect",
		Description: "Disconnect from the currently connected serial port. Releases the port and cleans up resources. Safe to call even if not connected (no-op). Returns success message or error if disconnect fails.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialDisconnect)

	// Register serial_write tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_write",
		Description: "Write data to the connected serial port. Data is sent as bytes to the device. The written data will also be forwarded to any connected WebSocket clients. Returns success message with bytes written or error if not connected.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type":        "string",
					"description": "Data to send to the serial port. Can be any text or binary data represented as string.",
				},
				"addNewline": map[string]any{
					"type":        "boolean",
					"description": "If true, automatically appends a newline (\\n) to the data. Useful for devices expecting line-terminated commands. Default: true.",
				},
			},
			"required": []string{"data"},
		},
	}, s.handleSerialWrite)

	// Register serial_read tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_read",
		Description: "Read data from the connected serial port. Blocks until data arrives or timeout expires. Data received from the serial port is also forwarded to connected WebSocket clients. Returns received data as string with byte count, or empty if timeout.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Maximum time to wait for data in milliseconds. Use 0 to wait indefinitely until data arrives. Default: 0 (indefinite).",
				},
				"maxSize": map[string]any{
					"type":        "integer",
					"description": "Maximum number of bytes to read in one call. Prevents buffer overflow. Default: 4096.",
				},
			},
		},
	}, s.handleSerialRead)

	// Register serial_status tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_status",
		Description: "Get the current serial port connection status. Returns whether connected, port name, baud rate, and other configuration details. Use this to check if serial_connect succeeded or to verify current settings.",
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

	mux.HandleFunc("/terminal", func(w http.ResponseWriter, r *http.Request) {
		data, err := web.StaticFiles.ReadFile("static/terminal.html")
		if err != nil {
			http.Error(w, "Terminal page not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if s.wsServer != nil {
			s.wsServer.HandleWebSocket(w, r)
		} else {
			http.Error(w, "WebSocket server not available", http.StatusServiceUnavailable)
		}
	})

	staticFS, err := fs.Sub(web.StaticFiles, "static")
	if err != nil {
		logrus.Warnf("[SerialHub] 静态文件系统初始化失败: %v", err)
	} else {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}

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
