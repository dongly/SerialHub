// Package web provides WebSocket server functionality for SerialHub.
package web

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
)

// WebSocketClient 表示一个 WebSocket 客户端连接。
type WebSocketClient struct {
	id        string
	conn      *websocket.Conn
	server    *WebSocketServer
	writeChan chan []byte
	stopChan  chan struct{}
	mu        sync.Mutex
	closed    bool
}

func newWebSocketClient(id string, conn *websocket.Conn, server *WebSocketServer) *WebSocketClient {
	return &WebSocketClient{
		id:        id,
		conn:      conn,
		server:    server,
		writeChan: make(chan []byte, 256),
		stopChan:  make(chan struct{}),
	}
}

func (c *WebSocketClient) RemoteAddr() string {
	if c.conn == nil {
		return ""
	}
	return c.conn.RemoteAddr().String()
}

func (c *WebSocketClient) Send(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return false
	}

	select {
	case c.writeChan <- data:
		return true
	default:
		return false
	}
}

func (c *WebSocketClient) Start() {
	go c.readLoop()
	go c.writeLoop()
}

func (c *WebSocketClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}
	c.closed = true
	close(c.stopChan)

	if c.conn != nil {
		c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(writeWait),
		)
		c.conn.Close()
	}

	go func() {
		for range c.writeChan {
		}
	}()
}

func (c *WebSocketClient) Closed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *WebSocketClient) readLoop() {
	defer func() {
		c.server.removeClient(c)
		c.conn.Close()
		logrus.Infof("[SerialHub] WebSocket 客户端已断开: %s (%s)", c.id, c.RemoteAddr())
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logrus.Warnf("[SerialHub] WebSocket 读取错误: %v", err)
			}
			return
		}

		if len(message) > 0 {
			targetChan := c.server.dataChan
			if c.isCommand(message) {
				targetChan = c.server.cmdChan
			}
			select {
			case targetChan <- message:
			default:
				logrus.Warnln("[SerialHub] 通道已满，丢弃数据")
			}
		}
	}
}

func (c *WebSocketClient) isCommand(message []byte) bool {
	if len(message) > 0 {
		return message[0] == '{'
	}
	return false
}

func (c *WebSocketClient) writeLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case <-c.stopChan:
			return
		case data, ok := <-c.writeChan:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}

			// JSON 消息以文本发送，串口数据以二进制发送
			msgType := websocket.BinaryMessage
			if c.isJSON(data) {
				msgType = websocket.TextMessage
			}

			if err := c.conn.WriteMessage(msgType, data); err != nil {
				logrus.Warnf("[SerialHub] WebSocket 写入错误: %v", err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *WebSocketClient) isJSON(data []byte) bool {
	if len(data) > 0 && data[0] == '{' {
		return true
	}
	return false
}
