// Package testutil provides mock objects and test helpers for SerialHub testing.
package testutil

import (
	"io"
	"sync"
	"time"

	serial "go.bug.st/serial"
)

// MockSerialPort simulates go.bug.st/serial.Port behavior for testing.
type MockSerialPort struct {
	WriteData []byte
	ReadData  []byte
	ReadErr   error
	WriteErr  error
	Closed    bool
	mu        sync.Mutex
	readPos   int
}

// NewMockSerialPort creates a new MockSerialPort with optional read data.
func NewMockSerialPort(readData []byte) *MockSerialPort {
	return &MockSerialPort{
		ReadData: readData,
	}
}

// Read reads from the mock port. Returns io.EOF after all data is consumed.
func (m *MockSerialPort) Read(p []byte) (n int, err error) {
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

// Write writes data to the mock port, recording it for verification.
func (m *MockSerialPort) Write(p []byte) (n int, err error) {
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

// Close marks the port as closed.
func (m *MockSerialPort) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Closed = true
	return nil
}

// SetMode sets the serial port mode (no-op for mock).
func (m *MockSerialPort) SetMode(mode *serial.Mode) error {
	return nil
}

// Drain waits until all data in the buffer are sent (no-op for mock).
func (m *MockSerialPort) Drain() error {
	return nil
}

// ResetInputBuffer purges port read buffer (no-op for mock).
func (m *MockSerialPort) ResetInputBuffer() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readPos = 0
	m.ReadData = nil
	return nil
}

// ResetOutputBuffer purges port write buffer (no-op for mock).
func (m *MockSerialPort) ResetOutputBuffer() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WriteData = nil
	return nil
}

// SetDTR sets the modem status bit DataTerminalReady (no-op for mock).
func (m *MockSerialPort) SetDTR(dtr bool) error {
	return nil
}

// SetRTS sets the modem status bit RequestToSend (no-op for mock).
func (m *MockSerialPort) SetRTS(rts bool) error {
	return nil
}

// GetModemStatusBits returns the modem status bits (no-op for mock).
func (m *MockSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}

// SetReadTimeout sets the timeout for the Read operation (no-op for mock).
func (m *MockSerialPort) SetReadTimeout(t time.Duration) error {
	return nil
}

// Break sends a break for a determined time (no-op for mock).
func (m *MockSerialPort) Break(d time.Duration) error {
	return nil
}

// GetWriteData returns a copy of the written data.
func (m *MockSerialPort) GetWriteData() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	data := make([]byte, len(m.WriteData))
	copy(data, m.WriteData)
	return data
}

// IsClosed returns whether the port is closed.
func (m *MockSerialPort) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Closed
}

// Reset resets the mock port to initial state.
func (m *MockSerialPort) Reset(readData []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WriteData = nil
	m.ReadData = readData
	m.readPos = 0
	m.Closed = false
	m.ReadErr = nil
	m.WriteErr = nil
}
