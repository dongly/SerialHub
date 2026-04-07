// Package web provides WebSocket server functionality for SerialHub.
package web

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// WebSocketServer 管理 WebSocket 服务器和客户端连接。
// 仅支持单客户端连接，新连接会踢掉旧连接。
type WebSocketServer struct {
	host          string
	port          int
	upgrader      websocket.Upgrader
	client        *WebSocketClient
	dataChan      chan []byte
	stopChan      chan struct{}
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	getSerialInfo func() string
}

// NewWebSocketServer 创建新的 WebSocket 服务器。
func NewWebSocketServer(host string, port int, getSerialInfo ...func() string) (*WebSocketServer, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("端口号必须在 0-65535 范围内")
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &WebSocketServer{
		host: host,
		port: port,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		dataChan: make(chan []byte, 256),
		stopChan: make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	if len(getSerialInfo) > 0 && getSerialInfo[0] != nil {
		s.getSerialInfo = getSerialInfo[0]
	}

	return s, nil
}

// DataChan 返回数据通道，用于接收客户端发送的数据。
func (s *WebSocketServer) DataChan() <-chan []byte {
	return s.dataChan
}

// Broadcast 向当前连接的 WebSocket 客户端发送数据。
// 返回成功发送的客户端数量（0 或 1）。
func (s *WebSocketServer) Broadcast(data []byte) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.client == nil {
		return 0
	}

	if s.client.Send(data) {
		return 1
	}
	return 0
}

// HandleWebSocket 处理 WebSocket 升级请求。
// 如果已有连接，先关闭旧连接再接受新连接。
func (s *WebSocketServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.Errorf("[SerialHub] WebSocket 升级失败: %v", err)
		return
	}

	s.mu.Lock()
	// 踢掉旧连接
	if s.client != nil {
		logrus.Infof("[SerialHub] 踢出旧 WebSocket 连接: %s", s.client.RemoteAddr())
		s.client.Stop()
		s.client = nil
	}

	// 创建新客户端
	clientID := uuid.New().String()
	client := newWebSocketClient(clientID, conn, s)
	s.client = client
	s.mu.Unlock()

	logrus.Infof("[SerialHub] WebSocket 客户端已连接: %s (%s)", clientID, conn.RemoteAddr())

	// 发送欢迎消息
	welcomeMsg := "Connected to SerialHub"
	if s.getSerialInfo != nil {
		serialInfo := s.getSerialInfo()
		if serialInfo != "" {
			welcomeMsg += " - Serial: " + serialInfo
		}
	}
	welcomeMsg += "\n"
	client.Send([]byte(welcomeMsg))

	// 启动客户端读写循环
	client.Start()
}

// Start 启动 WebSocket HTTP 服务器。
func (s *WebSocketServer) Start() error {
	return nil
}

// Stop 停止 WebSocket 服务，关闭客户端和通道。
func (s *WebSocketServer) Stop() error {
	s.cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		s.client.Stop()
		s.client = nil
	}

	select {
	case <-s.stopChan:
		// 已关闭
	default:
		close(s.stopChan)
	}

	logrus.Infof("[SerialHub] WebSocket 服务已停止: %s:%d", s.host, s.port)
	return nil
}

// ClientCount 返回当前连接的客户端数量。
func (s *WebSocketServer) ClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.client == nil {
		return 0
	}
	return 1
}

// HasClient 返回是否有客户端连接。
func (s *WebSocketServer) HasClient() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client != nil
}

// removeClient 从服务器移除指定客户端。
func (s *WebSocketServer) removeClient(c *WebSocketClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == c {
		s.client = nil
	}
}
