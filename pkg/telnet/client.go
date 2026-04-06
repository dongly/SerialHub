// Package telnet provides Telnet server functionality.
package telnet

import (
	"bufio"
	"io"
	"net"
	"sync"

	"github.com/sirupsen/logrus"
)

const (
	IAC  byte = 255
	WILL byte = 251
	WONT byte = 252
	DO   byte = 253
	DONT byte = 254
	ECHO byte = 1
)

// TelnetClient represents a connected Telnet client.
type TelnetClient struct {
	id        string
	conn      net.Conn
	server    *TelnetServer
	writeChan chan []byte
	stopChan  chan struct{}
	mu        sync.Mutex
	closed    bool
}

// newTelnetClient creates a new Telnet client.
func newTelnetClient(id string, conn net.Conn, server *TelnetServer) *TelnetClient {
	return &TelnetClient{
		id:        id,
		conn:      conn,
		server:    server,
		writeChan: make(chan []byte, 1024),
		stopChan:  make(chan struct{}),
		closed:    false,
	}
}

// ID returns the client ID.
func (tc *TelnetClient) ID() string {
	return tc.id
}

// RemoteAddr returns the remote address of the client.
func (tc *TelnetClient) RemoteAddr() string {
	return tc.conn.RemoteAddr().String()
}

// Start starts the client's read loop and write loop.
func (tc *TelnetClient) Start() {
	go tc.readLoop()
	go tc.writeLoop()
}

// Stop stops the client and closes the connection.
func (tc *TelnetClient) Stop() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.closed {
		return
	}

	tc.closed = true
	close(tc.stopChan)

	// Close connection
	if tc.conn != nil {
		tc.conn.Close()
	}

	// Drain write channel
	go func() {
		for range tc.writeChan {
		}
	}()
}

// Closed returns whether the client is closed.
func (tc *TelnetClient) Closed() bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.closed
}

// Send sends data to the client asynchronously.
func (tc *TelnetClient) Send(data []byte) bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.closed {
		return false
	}

	select {
	case tc.writeChan <- data:
		return true
	default:
		// Channel full, drop data
		return false
	}
}

// readLoop continuously reads data from the client connection.
func (tc *TelnetClient) readLoop() {
	reader := bufio.NewReader(tc.conn)

	for {
		select {
		case <-tc.stopChan:
			return
		default:
			data, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					logrus.Warnf("[SerialHub] Telnet 客户端读取错误: %v", err)
				}
				tc.server.DisconnectClient(tc.id)
				return
			}

			if len(data) > 0 {
				// Send data to server's data channel
				tc.server.dataChan <- data
			}
		}
	}
}

// writeLoop writes data to the client connection.
func (tc *TelnetClient) writeLoop() {
	for {
		select {
		case <-tc.stopChan:
			return
		case data := <-tc.writeChan:
			if tc.conn != nil {
				_, err := tc.conn.Write(data)
				if err != nil {
					logrus.Warnf("[SerialHub] Telnet 客户端写入错误: %v", err)
					tc.server.DisconnectClient(tc.id)
					return
				}
			}
		}
	}
}
