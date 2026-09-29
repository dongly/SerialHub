// Package web provides WebSocket server functionality for SerialHub.
package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// WebSocketServer 管理 WebSocket 服务器和客户端连接。
// 支持多客户端同时连接。
type WebSocketServer struct {
	host          string
	port          int
	upgrader      websocket.Upgrader
	clients       map[string]*WebSocketClient
	dataChan      chan []byte
	cmdChan       chan []byte
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
			CheckOrigin: checkOrigin,
		},
		clients:  make(map[string]*WebSocketClient),
		dataChan: make(chan []byte, 256),
		cmdChan:  make(chan []byte, 64),
		stopChan: make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	if len(getSerialInfo) > 0 && getSerialInfo[0] != nil {
		s.getSerialInfo = getSerialInfo[0]
	}

	return s, nil
}

// checkOrigin 校验 WebSocket 升级请求的来源主机与端口：
//   - 无 Origin 头（非浏览器客户端：脚本、测试、MCP 客户端）放行；
//   - 浏览器请求要求 Origin 为合法绝对地址（scheme://host[:port]）且
//     host:port 与请求 Host 字面相等，防止第三方网页跨站连接本服务
//     发送 shutdown 等控制命令（CSRF）。
//
// 注意：这是来源校验而非鉴权——不校验 scheme 的具体取值（页面 http 与
// WS ws 天然不同）、不防 DNS rebinding；如需更强边界需另加认证。
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return u.Host == r.Host
}

// DataChan 返回数据通道，用于接收客户端发送的数据。
func (s *WebSocketServer) DataChan() <-chan []byte {
	return s.dataChan
}

// CmdChan 返回命令通道，用于接收客户端发送的控制命令。
func (s *WebSocketServer) CmdChan() <-chan []byte {
	return s.cmdChan
}

// Broadcast 向所有连接的 WebSocket 客户端发送数据。
// 返回成功发送的客户端数量。
func (s *WebSocketServer) Broadcast(data []byte) int {
	s.mu.RLock()
	clients := make([]*WebSocketClient, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.mu.RUnlock()

	count := 0
	for _, client := range clients {
		if client.Send(data) {
			count++
		}
	}
	return count
}

// HandleWebSocket 处理 WebSocket 升级请求。
// 支持多客户端同时连接。
func (s *WebSocketServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.Errorf("[SerialHub] WebSocket 升级失败: %v", err)
		return
	}

	s.mu.Lock()
	// 创建新客户端
	clientID := uuid.New().String()
	client := newWebSocketClient(clientID, conn, s)
	s.clients[clientID] = client
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

// Stop 停止 WebSocket 服务，关闭所有客户端和通道。
func (s *WebSocketServer) Stop() error {
	s.cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭所有客户端
	for _, client := range s.clients {
		client.Stop()
	}
	s.clients = make(map[string]*WebSocketClient)

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
	return len(s.clients)
}

// HasClient 返回是否有客户端连接。
func (s *WebSocketServer) HasClient() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients) > 0
}

// removeClient 从服务器移除指定客户端。
func (s *WebSocketServer) removeClient(c *WebSocketClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, c.id)
}
