// Package federation 实现主从联邦：主实例聚合双侧串口，从实例贡献本侧串口。
//
// 角色与发现见 discover.go；主侧管理见 manager.go；从侧客户端见 client.go。
// 传输层：主侧 /federation WebSocket 端点，从侧主动外连，
// 协议为 JSON-RPC 2.0 双向（请求/响应/notification），数据载荷 base64 编码。
package federation

// Side 标识实例所在的操作系统侧。
const (
	SideWindows = "windows"
	SideWSL     = "wsl"
)

// Request 是联邦通道上的 JSON-RPC 2.0 请求或通知（ID==0 视为通知）。
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Response 是联邦通道上的 JSON-RPC 2.0 响应。
type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      int64     `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// RPCError 是 JSON-RPC 错误对象。
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error 使 RPCError 实现 error 接口。
func (e *RPCError) Error() string { return e.Message }

// 联邦协议方法名。
const (
	// MethodRegister 从→主：注册本侧身份与端口列表（请求）。
	MethodRegister = "federation/register"
	// MethodSerialData 从→主：串口数据上行（notification）。
	MethodSerialData = "serial/data"

	// MethodSerialOpen 主→从：请求从侧打开串口。
	MethodSerialOpen = "serial/open"
	// MethodSerialWrite 主→从：请求从侧写串口。
	MethodSerialWrite = "serial/write"
	// MethodSerialClose 主→从：请求从侧关闭串口。
	MethodSerialClose = "serial/close"
)

// RegisterParams 是 federation/register 的参数。
type RegisterParams struct {
	// OS 从实例所在侧："windows" 或 "wsl"。
	OS string `json:"os"`
	// Ports 从实例本侧可用串口名（裸名，如 "COM3"、"/dev/ttyUSB1"）。
	Ports []string `json:"ports"`
}

// DataParams 是 serial/data 的参数。
type DataParams struct {
	// Port 从侧裸端口名。
	Port string `json:"port"`
	// Data 收到的串口数据，base64 编码。
	Data string `json:"data"`
}

// SerialParams 是主→从 serial/* 请求的参数。
type SerialParams struct {
	// Port 从侧裸端口名。
	Port string `json:"port,omitempty"`
	// BaudRate serial/open 的波特率（0 = 默认 115200）。
	BaudRate int `json:"baudRate,omitempty"`
	// Data serial/write 的数据，base64 编码。
	Data string `json:"data,omitempty"`
}

// SerialResult 是从侧 serial/open|write|close 的响应，与 MCP ToolResult 对齐。
type SerialResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// PortInfo 描述联邦视图中的一个串口（主侧 serial_list 聚合用）。
type PortInfo struct {
	// Name 对外端口名：本侧为裸名，联邦侧为 "side:port" 全名（MCP connect 直接使用）。
	Name string `json:"name"`
	// Origin "local"（主实例本侧）或 "federated"（从实例上报）。
	Origin string `json:"origin"`
	// Side 端口所在侧："windows" 或 "wsl"。
	Side string `json:"side"`
	// Port 裸端口名（联邦侧去掉 "side:" 前缀后的名字）。
	Port string `json:"port"`
}
