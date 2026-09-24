// Package mcp provides MCP server implementation with serial port tools.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"github.com/dongly/serialhub/internal/buffer"
	"github.com/dongly/serialhub/internal/federation"
	"github.com/dongly/serialhub/pkg/mcp/tools"
	"github.com/dongly/serialhub/pkg/serial"
	"github.com/dongly/serialhub/pkg/version"
	"github.com/dongly/serialhub/pkg/web"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// FedRouter 是主实例的联邦路由能力，由 internal/federation.Manager 实现。
// 从实例上报的端口在 MCP 工具层以此接口路由到远侧执行。
type FedRouter interface {
	LocalSide() string
	WorkerCount() int
	FederatedPorts() []federation.PortInfo
	ActiveFederatedPort() string
	IsFederated(portName string) bool
	Open(portName string, baudRate int) federation.SerialResult
	Write(portName string, data []byte) federation.SerialResult
	Close(portName string) federation.SerialResult
}

// MCPServer manages the MCP server and tool registration
type MCPServer struct {
	serialManager *serial.SerialManager
	dataBuffer    *buffer.DataBuffer
	mcpServer     *mcpsdk.Server
	wsServer      *web.WebSocketServer
	federation    FedRouter
	federationWS  http.Handler
}

// SetFederation 注入联邦路由器（主实例），并将联邦 WebSocket 端点
// （从实例外连的 /federation）交由 StartHTTPServer 挂载。
func (s *MCPServer) SetFederation(fr FedRouter, wsHandler http.Handler) {
	s.federation = fr
	s.federationWS = wsHandler
}

// NewMCPServer creates a new MCP server instance
func NewMCPServer(sm *serial.SerialManager, buf *buffer.DataBuffer, wsSrv ...*web.WebSocketServer) (*MCPServer, error) {
	if buf == nil {
		buf = buffer.NewDataBuffer()
	}

	// Create MCP server
	mcpServer := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: version.Name, Version: version.Version},
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

// RegisterTools registers all 7 serial port tools with the MCP server
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
					"description": "If true, automatically appends a newline (\\n) to the data. Default: true (a newline is appended unless this is explicitly set to false).",
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
					"description": "Maximum time to wait for data in milliseconds. Use 0 to wait indefinitely until data arrives. Default: 1000.",
				},
				"maxSize": map[string]any{
					"type":        "integer",
					"description": "Maximum number of bytes to read in one call. Prevents buffer overflow. Default: 4096.",
				},
			},
		},
	}, s.handleSerialRead)

	// Register serial_clear tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_clear",
		Description: "Clear the read buffer. Discards all buffered serial data not yet consumed by serial_read. Returns the number of bytes cleared.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialClear)

	// Register serial_status tool
	s.mcpServer.AddTool(&mcpsdk.Tool{
		Name:        "serial_status",
		Description: "Get the current serial port connection status. Returns whether connected, port name, baud rate, and other configuration details. Use this to check if serial_connect succeeded or to verify current settings.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, s.handleSerialStatus)

	logrus.Infoln("[SerialHub] MCP server registered 7 tools")
	return nil
}

// StartHTTPServer starts the HTTP transport for MCP communication.
// autoOpenBrowser 控制启动后是否自动打开 xterm web（/terminal）。
func (s *MCPServer) StartHTTPServer(addr string, autoOpenBrowser bool) (*http.Server, error) {
	streamableHandler := mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		return s.mcpServer
	}, &mcpsdk.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})

	mux := http.NewServeMux()
	mux.Handle("/mcp", streamableHandler)
	// 联邦端点：从实例经 WebSocket 外连注册（仅主实例注入后存在）
	if s.federationWS != nil {
		mux.Handle("/federation", s.federationWS)
	}
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// role 字段供实例发现区分主从（从实例反代 /health 返回 role=worker）
		w.Write([]byte(`{"status":"ok","role":"master"}`))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"version":"` + version.FullVersion() + `"}`))
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

	// 先绑定端口再对外提供服务：listen 成功即端口可用，
	// 消除原先 500ms sleep 等待服务器就绪的竞态 hack
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("HTTP 服务器监听失败: %w", err)
	}

	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			logrus.Errorf("[SerialHub] HTTP 服务器错误: %v", err)
		}
	}()

	logrus.Infof("[SerialHub] MCP HTTP 服务器已启动: %s", ln.Addr())

	// 自动打开浏览器（--minimized/--stdio/从实例模式下由调用方关闭）
	if autoOpenBrowser {
		displayAddr := addr
		if strings.HasPrefix(addr, "0.0.0.0:") {
			displayAddr = "127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
		}
		url := "http://" + displayAddr + "/terminal"
		logrus.Infof("[SerialHub] 正在打开浏览器: %s", url)
		go openBrowser(url)
	}

	return server, nil
}

// openBrowser 打开系统默认浏览器。
// Linux/WSL 下按可用性依次降级：xdg-open（桌面 Linux）→
// wslview（WSL + wslu）→ cmd.exe（WSL 互操作，最通用的兜底）。
func openBrowser(url string) {
	if runtime.GOOS == "windows" {
		if err := exec.Command("cmd", "/c", "start", url).Start(); err != nil {
			logrus.Warnf("[SerialHub] 打开浏览器失败: %v", err)
		}
		return
	}
	if runtime.GOOS == "darwin" {
		if err := exec.Command("open", url).Start(); err != nil {
			logrus.Warnf("[SerialHub] 打开浏览器失败: %v", err)
		}
		return
	}

	// Linux/WSL：探测式降级链
	candidates := []struct {
		cmd  string
		args []string
	}{
		{"xdg-open", []string{url}},
		{"wslview", []string{url}},
		// start 的首参数 "" 是窗口标题占位，避免 URL 被当作标题
		{"cmd.exe", []string{"/c", "start", "", url}},
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c.cmd); err != nil {
			continue
		}
		if err := exec.Command(c.cmd, c.args...).Start(); err != nil {
			continue
		}
		return
	}
	logrus.Warn("[SerialHub] 打开浏览器失败: 未找到可用的浏览器启动方式 (xdg-open/wslview/cmd.exe)")
}

// RunStdioTransport 在 stdio 传输上服务同一 SDK Server
// （stdio 主实例模式：与 HTTP /mcp 共用工具注册，随连接断开返回）。
func (s *MCPServer) RunStdioTransport(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcpsdk.StdioTransport{})
}

// Stop stops the MCP server
func (s *MCPServer) Stop() error {
	logrus.Infoln("[SerialHub] MCP 服务器已停止")
	return nil
}

// withCORS adds CORS support to the HTTP handler.
// 注意：Origin/CSRF 校验由 go-sdk StreamableHTTPHandler 内置提供
// （DNS rebinding 保护 + CrossOriginProtection），此处 CORS 头仅影响
// 浏览器预检；Allow-Headers 需覆盖规范定义的 MCP 请求头，否则
// 浏览器端 MCP 客户端预检会失败。
func (s *MCPServer) withCORS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Mcp-Session-Id, Mcp-Protocol-Version, Authorization, Last-Event-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// Tool handlers - using low-level ToolHandler API

// handleSerialList 聚合本侧与联邦（从实例上报）端口。
// 本地端口对外名为裸名（如 COM3、/dev/ttyUSB1）；
// 联邦端口对外名为 "side:port" 全名（如 "wsl:/dev/ttyUSB1"），connect 直接使用。
func (s *MCPServer) handleSerialList(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	local := tools.ExecuteSerialList(s.serialManager)
	if !local.Success {
		return s.toolResultToMCPResult(local)
	}

	side := federation.LocalSide()
	ports := make([]federation.PortInfo, 0, 8)
	m, ok := local.Data.(map[string]any)
	if !ok {
		logrus.Warnf("[SerialHub] 本地端口数据格式异常（期望 map，实际 %T），仅返回联邦端口", local.Data)
	} else if localPorts, ok := m["ports"].([]string); !ok {
		logrus.Warnf("[SerialHub] 本地端口列表类型异常（期望 []string），仅返回联邦端口")
	} else {
		for _, p := range localPorts {
			ports = append(ports, federation.PortInfo{Name: p, Origin: "local", Side: side, Port: p})
		}
	}
	if s.federation != nil {
		ports = append(ports, s.federation.FederatedPorts()...)
	}

	return s.toolResultToMCPResult(tools.ToolResult{
		Success: true,
		Message: fmt.Sprintf("找到 %d 个串口", len(ports)),
		Data:    map[string]any{"ports": ports},
	})
}

func (s *MCPServer) handleSerialConnect(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var input tools.ConnectInput
	if err := s.parseRequestParams(req, &input); err != nil {
		return nil, s.invalidParamsError(err)
	}

	// 联邦端口：路由到从实例执行
	if s.federation != nil && s.federation.IsFederated(input.Port) {
		if s.serialManager.IsConnected() {
			return s.toolResultToMCPResult(tools.ToolResult{Success: false, Message: "本地串口已连接，请先断开后再连接联邦端口"})
		}
		return s.toolResultToMCPResult(fedSerialToToolResult(s.federation.Open(input.Port, input.BaudRate)))
	}

	// 本地端口：联邦口活动时拒绝（全局单活动口模型，数据统一入 DataBuffer）
	if s.federation != nil && s.federation.ActiveFederatedPort() != "" {
		return s.toolResultToMCPResult(tools.ToolResult{
			Success: false,
			Message: fmt.Sprintf("联邦端口 %s 已连接，请先断开", s.federation.ActiveFederatedPort()),
		})
	}

	result := tools.ExecuteSerialConnect(s.serialManager, input)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialDisconnect(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	if s.federation != nil {
		if ap := s.federation.ActiveFederatedPort(); ap != "" {
			return s.toolResultToMCPResult(fedSerialToToolResult(s.federation.Close(ap)))
		}
	}
	result := tools.ExecuteSerialDisconnect(s.serialManager)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialWrite(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var input tools.WriteInput
	if err := s.parseRequestParams(req, &input); err != nil {
		return nil, s.invalidParamsError(err)
	}

	// 联邦口活动：数据路由到从实例写入（addNewline 在主侧展开，从侧只收裸字节）
	if s.federation != nil {
		if ap := s.federation.ActiveFederatedPort(); ap != "" {
			data := input.Data
			addNewline := input.AddNewline == nil || *input.AddNewline
			if addNewline {
				data += "\n"
			}
			return s.toolResultToMCPResult(fedSerialToToolResult(s.federation.Write(ap, []byte(data))))
		}
	}

	result := tools.ExecuteSerialWrite(s.serialManager, input)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialRead(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	var input tools.ReadInput
	if err := s.parseRequestParams(req, &input); err != nil {
		return nil, s.invalidParamsError(err)
	}
	result := tools.ExecuteSerialRead(ctx, s.dataBuffer, input)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialClear(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	result := tools.ExecuteSerialClear(s.dataBuffer)
	return s.toolResultToMCPResult(result)
}

func (s *MCPServer) handleSerialStatus(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	if s.federation != nil {
		if ap := s.federation.ActiveFederatedPort(); ap != "" {
			return s.toolResultToMCPResult(tools.ToolResult{
				Success: true,
				Message: fmt.Sprintf("已连接联邦端口 %s", ap),
				Data:    map[string]any{"connected": true, "port": ap, "origin": "federated"},
			})
		}
	}
	result := tools.ExecuteSerialStatus(s.serialManager)
	return s.toolResultToMCPResult(result)
}

// fedSerialToToolResult 将联邦通道的 SerialResult 转为 tools.ToolResult。
func fedSerialToToolResult(res federation.SerialResult) tools.ToolResult {
	data := res.Data
	if data == nil && res.Success {
		data = map[string]any{}
	}
	return tools.ToolResult{Success: res.Success, Message: res.Message, Data: data}
}

// Helper functions
func (s *MCPServer) invalidParamsError(err error) error {
	return &jsonrpc.Error{
		Code:    jsonrpc.CodeInvalidParams,
		Message: fmt.Sprintf("参数解析错误: %v", err),
	}
}

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

// toolResultToMCPResult 将内部 ToolResult 转为 MCP CallToolResult。
// 依据 MCP 规范（2025-06-18 / server/tools#structured-content）：
// 返回 structuredContent 的工具 SHOULD 同时在 TextContent 中给出
// 序列化后的 JSON（供不支持 structuredContent 的旧客户端回退解析）。
func (s *MCPServer) toolResultToMCPResult(result tools.ToolResult) (*mcpsdk.CallToolResult, error) {
	var text string
	if result.Success {
		// 成功：message 与数据合并为单个 JSON 对象序列化
		var payload map[string]any
		if m, ok := result.Data.(map[string]any); ok {
			payload = make(map[string]any, len(m)+1)
			payload["message"] = result.Message
			for k, v := range m {
				payload[k] = v
			}
		} else if result.Data == nil {
			payload = map[string]any{"message": result.Message}
		} else {
			payload = map[string]any{"message": result.Message, "data": result.Data}
		}
		b, err := json.Marshal(payload)
		if err != nil {
			// 序列化失败兜底退回纯文本
			text = result.Message
		} else {
			text = string(b)
		}
	} else {
		// 失败：规范示例即纯文本错误消息（isError=true）
		text = result.Message
	}

	content := mcpsdk.TextContent{Text: text}

	return &mcpsdk.CallToolResult{
		Content:           []mcpsdk.Content{&content},
		StructuredContent: result.Data,
		IsError:           !result.Success,
	}, nil
}
