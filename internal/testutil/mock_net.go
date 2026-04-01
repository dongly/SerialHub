// Package testutil provides mock objects and test helpers for SerialHub testing.
package testutil

import (
	"io"
	"net"
	"sync"
	"time"
)

// MockConn simulates net.Conn behavior for testing.
type MockConn struct {
	ReadData    []byte
	WriteData   []byte
	ReadErr     error
	WriteErr    error
	Closed      bool
	localAddr   net.Addr
	remoteAddr  net.Addr
	mu          sync.Mutex
	readPos     int
	closeChan   chan struct{}
}

// NewMockConn creates a new MockConn.
func NewMockConn() *MockConn {
	return &MockConn{
		localAddr:  &MockAddr{addr: "127.0.0.1:2323"},
		remoteAddr: &MockAddr{addr: "127.0.0.1:54321"},
		closeChan:  make(chan struct{}),
	}
}

// Read reads from the mock connection. Returns io.EOF after all data is consumed.
func (m *MockConn) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Closed {
		return 0, io.ErrClosedPipe
	}

	if m.ReadErr != nil {
		return 0, m.ReadErr
	}

	if m.readPos >= len(m.ReadData) {
		return 0, io.EOF
	}

	n = copy(p, m.ReadData[m.readPos:])
	m.readPos += n
	return n, nil
}

// Write writes data to the mock connection, recording it for verification.
func (m *MockConn) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Closed {
		return 0, io.ErrClosedPipe
	}

	if m.WriteErr != nil {
		return 0, m.WriteErr
	}

	m.WriteData = append(m.WriteData, p...)
	return len(p), nil
}

// Close marks the connection as closed.
func (m *MockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.Closed {
		m.Closed = true
		close(m.closeChan)
	}
	return nil
}

// LocalAddr returns the local address of the connection.
func (m *MockConn) LocalAddr() net.Addr {
	return m.localAddr
}

// RemoteAddr returns the remote address of the connection.
func (m *MockConn) RemoteAddr() net.Addr {
	return m.remoteAddr
}

// SetDeadline sets the read and write deadlines (no-op for mock).
func (m *MockConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline sets the read deadline (no-op for mock).
func (m *MockConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline sets the write deadline (no-op for mock).
func (m *MockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// GetWriteData returns a copy of the written data.
func (m *MockConn) GetWriteData() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	data := make([]byte, len(m.WriteData))
	copy(data, m.WriteData)
	return data
}

// IsClosed returns whether the connection is closed.
func (m *MockConn) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Closed
}

// ClosedChan returns a channel that's closed when the connection closes.
func (m *MockConn) ClosedChan() <-chan struct{} {
	return m.closeChan
}

// SetLocalAddr sets the local address for testing.
func (m *MockConn) SetLocalAddr(addr net.Addr) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.localAddr = addr
}

// SetRemoteAddr sets the remote address for testing.
func (m *MockConn) SetRemoteAddr(addr net.Addr) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remoteAddr = addr
}

// Reset resets the mock connection to initial state.
func (m *MockConn) Reset(readData []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WriteData = nil
	m.ReadData = readData
	m.readPos = 0
	m.Closed = false
	m.ReadErr = nil
	m.WriteErr = nil
	m.closeChan = make(chan struct{})
}

// MockAddr simulates net.Addr for testing.
type MockAddr struct {
	addr string
}

// Network returns the network type.
func (m *MockAddr) Network() string {
	return "tcp"
}

// String returns the address string.
func (m *MockAddr) String() string {
	return m.addr
}

// NewMockAddr creates a new MockAddr.
func NewMockAddr(addr string) *MockAddr {
	return &MockAddr{addr: addr}
}
