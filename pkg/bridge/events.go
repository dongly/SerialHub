package bridge

// SerialDataEvent 表示从串口接收的数据事件
type SerialDataEvent struct {
	Data []byte
}

// WebSocketDataEvent 表示从 WebSocket 客户端接收的数据事件
type WebSocketDataEvent struct {
	Data []byte
}

// ForwardEvent 表示转发数据的目标
type ForwardEvent struct {
	Source string // "serial" 或 "ws"
	Data   []byte
}
