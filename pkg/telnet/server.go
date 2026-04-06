// Package telnet provides Telnet server functionality.
package telnet

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// TelnetServer manages Telnet server and client connections.
type TelnetServer struct {
	host          string
	port          int
	listener      net.Listener
	clients       map[string]*TelnetClient
	dataChan      chan []byte
	stopChan      chan struct{}
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	getSerialInfo func() string
}

// NewTelnetServer creates a new Telnet server.
func NewTelnetServer(host string, port int, getSerialInfo ...func() string) (*TelnetServer, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("端口号必须在 0-65535 范围内")
	}

	ctx, cancel := context.WithCancel(context.Background())

	ts := &TelnetServer{
		host:     host,
		port:     port,
		clients:  make(map[string]*TelnetClient),
		dataChan: make(chan []byte, 256),
		stopChan: make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	if len(getSerialInfo) > 0 && getSerialInfo[0] != nil {
		ts.getSerialInfo = getSerialInfo[0]
	}

	return ts, nil
}

// Start starts the Telnet server.
func (ts *TelnetServer) Start() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.listener != nil {
		return fmt.Errorf("服务器已在运行")
	}

	addr := fmt.Sprintf("%s:%d", ts.host, ts.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("启动 Telnet 服务器失败: %w", err)
	}

	ts.listener = listener
	logrus.Infof("[SerialHub] Telnet 服务器已启动: %s", addr)

	// Start accept loop
	go ts.acceptLoop()

	return nil
}

// Stop stops the Telnet server.
func (ts *TelnetServer) Stop() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.listener == nil {
		return fmt.Errorf("服务器未运行")
	}

	// Cancel context
	ts.cancel()

	// Close listener
	if ts.listener != nil {
		ts.listener.Close()
		ts.listener = nil
	}

	// Disconnect all clients
	for _, client := range ts.clients {
		client.Stop()
	}
	ts.clients = nil

	// Close stop channel
	close(ts.stopChan)

	logrus.Infof("[SerialHub] Telnet 服务器已停止: %s:%d", ts.host, ts.port)

	return nil
}

// Broadcast sends data to all connected clients.
func (ts *TelnetServer) Broadcast(data []byte) int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if ts.clients == nil {
		return 0
	}

	count := 0
	for _, client := range ts.clients {
		if client.Send(data) {
			count++
		}
	}

	return count
}

// SendToClient sends data to a specific client.
func (ts *TelnetServer) SendToClient(clientID string, data []byte) error {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if ts.clients == nil {
		return fmt.Errorf("没有连接的客户端")
	}

	client, exists := ts.clients[clientID]
	if !exists {
		return fmt.Errorf("客户端不存在: %s", clientID)
	}

	if !client.Send(data) {
		return fmt.Errorf("发送数据失败: 客户端已关闭或缓冲区已满")
	}

	return nil
}

// DisconnectClient disconnects a specific client.
func (ts *TelnetServer) DisconnectClient(clientID string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.clients == nil {
		return fmt.Errorf("没有连接的客户端")
	}

	client, exists := ts.clients[clientID]
	if !exists {
		return fmt.Errorf("客户端不存在: %s", clientID)
	}

	client.Stop()
	delete(ts.clients, clientID)

	logrus.Infof("[SerialHub] Telnet 客户端已断开: %s (%s)", clientID, client.RemoteAddr())

	return nil
}

// DataChan returns the data channel for receiving data from clients.
func (ts *TelnetServer) DataChan() <-chan []byte {
	return ts.dataChan
}

// ClientCount returns the number of connected clients.
func (ts *TelnetServer) ClientCount() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if ts.clients == nil {
		return 0
	}

	return len(ts.clients)
}

// GetClients returns a list of all connected client IDs.
func (ts *TelnetServer) GetClients() []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if ts.clients == nil {
		return []string{}
	}

	clientIDs := make([]string, 0, len(ts.clients))
	for id := range ts.clients {
		clientIDs = append(clientIDs, id)
	}

	return clientIDs
}

// IsRunning returns whether the server is running.
func (ts *TelnetServer) IsRunning() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.listener != nil
}

// acceptLoop accepts new client connections.
func (ts *TelnetServer) acceptLoop() {
	for {
		select {
		case <-ts.ctx.Done():
			return
		default:
			conn, err := ts.listener.Accept()
			if err != nil {
				select {
				case <-ts.ctx.Done():
					// Server is stopping, this is expected
					return
				default:
					logrus.Errorf("[SerialHub] Telnet 接受连接失败: %v", err)
					continue
				}
			}

			// Create new client
			clientID := uuid.New().String()
			client := newTelnetClient(clientID, conn, ts)

			// Send IAC WILL ECHO to disable local echo
			conn.Write([]byte{IAC, WILL, ECHO})

			// Add to clients map
			ts.mu.Lock()
			if ts.clients == nil {
				ts.clients = make(map[string]*TelnetClient)
			}
			ts.clients[clientID] = client
			ts.mu.Unlock()

			// Send welcome message
			welcomeMsg := "Connected to SerialHub"
			if ts.getSerialInfo != nil {
				serialInfo := ts.getSerialInfo()
				if serialInfo != "" {
					welcomeMsg += " - Serial: " + serialInfo
				}
			}
			welcomeMsg += "\r\n"
			client.Send([]byte(welcomeMsg))

			// Start client
			client.Start()

			logrus.Infof("[SerialHub] Telnet 客户端已连接: %s (%s)", clientID, conn.RemoteAddr())
		}
	}
}

// Close closes the server and cleans up resources.
func (ts *TelnetServer) Close() error {
	// Close data channel
	if ts.dataChan != nil {
		close(ts.dataChan)
		ts.dataChan = nil
	}

	return ts.Stop()
}
