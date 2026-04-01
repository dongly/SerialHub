// Package bridge provides data bridging functionality between serial, telnet, and MCP.
package bridge

// SerialDataEvent 表示从串口接收的数据事件
type SerialDataEvent struct {
	Data []byte
}

// TelnetDataEvent 表示从 Telnet 客户端接收的数据事件
type TelnetDataEvent struct {
	Data []byte
}

// ForwardEvent 表示转发数据的目标
type ForwardEvent struct {
	Source string // "serial" 或 "telnet"
	Data   []byte
}
