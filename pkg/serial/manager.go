// Package serial manages serial port connections.
package serial

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	serial "go.bug.st/serial"

	"github.com/sirupsen/logrus"
)

// Port defines the serial port interface for dependency injection
type Port interface {
	io.Reader
	io.Writer
	io.Closer
	SetReadTimeout(timeout time.Duration) error
}

// SerialManager manages serial port connections
type SerialManager struct {
	config     *Config
	port       Port
	dataChan   chan []byte
	errChan    chan error
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *logrus.Logger
}

// NewSerialManager creates a new serial manager
func NewSerialManager(cfg *Config) (*SerialManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}
	if cfg.Port == "" {
		return nil, fmt.Errorf("端口不能为空")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &SerialManager{
		config:   cfg.Clone(),
		dataChan: make(chan []byte, 256),
		errChan:  make(chan error, 16),
		ctx:      ctx,
		cancel:   cancel,
		logger:   logrus.New(),
	}, nil
}

// Connect connects to the serial port
func (sm *SerialManager) Connect() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.port != nil {
		return fmt.Errorf("串口已连接: %s", sm.config.Port)
	}

	mode, err := sm.config.ToMode()
	if err != nil {
		return fmt.Errorf("配置转换失败: %w", err)
	}

	port, err := serial.Open(sm.config.Port, mode)
	if err != nil {
		return fmt.Errorf("打开串口失败: %w", err)
	}

	sm.port = port
	sm.logger.Infof("[SerialHub] 串口已连接: %s", sm.config.String())

	// Start read loop
	go sm.readLoop()

	return nil
}

// Disconnect disconnects from the serial port
func (sm *SerialManager) Disconnect() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.port == nil {
		return fmt.Errorf("串口未连接")
	}

	err := sm.port.Close()
	sm.port = nil
	sm.logger.Infof("[SerialHub] 串口已断开: %s", sm.config.Port)

	return err
}

// Write writes data to the serial port
func (sm *SerialManager) Write(data []byte) (int, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.port == nil {
		return 0, fmt.Errorf("串口未连接")
	}

	n, err := sm.port.Write(data)
	if err != nil {
		sm.errChan <- fmt.Errorf("写入错误: %w", err)
		return n, fmt.Errorf("写入失败: %w", err)
	}

	return n, nil
}

// WriteLine writes a line of data to the serial port (adds newline automatically)
func (sm *SerialManager) WriteLine(line string) error {
	data := []byte(line + "\n")
	_, err := sm.Write(data)
	return err
}

// ListPorts lists all available serial ports
func (sm *SerialManager) ListPorts() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("获取串口列表失败: %w", err)
	}
	return ports, nil
}

// IsConnected checks if the serial port is connected
func (sm *SerialManager) IsConnected() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.port != nil
}

// CurrentPort returns the current connected port name
func (sm *SerialManager) CurrentPort() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.port == nil {
		return ""
	}
	return sm.config.Port
}

// GetConfig returns a copy of the current configuration
func (sm *SerialManager) GetConfig() *Config {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.config.Clone()
}

// UpdateConfig updates the configuration (must be called when not connected)
func (sm *SerialManager) UpdateConfig(cfg *Config) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.port != nil {
		return fmt.Errorf("串口已连接，无法更新配置")
	}

	if cfg == nil {
		return fmt.Errorf("配置不能为空")
	}
	if cfg.Port == "" {
		return fmt.Errorf("端口不能为空")
	}

	sm.config = cfg.Clone()
	return nil
}

// DataChan returns the data channel
func (sm *SerialManager) DataChan() <-chan []byte {
	return sm.dataChan
}

// ErrChan returns the error channel
func (sm *SerialManager) ErrChan() <-chan error {
	return sm.errChan
}

// Close closes the manager and cleans up resources
func (sm *SerialManager) Close() error {
	sm.cancel()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	var err error
	if sm.port != nil {
		err = sm.port.Close()
		sm.port = nil
	}

	close(sm.dataChan)
	close(sm.errChan)

	return err
}

// readLoop continuously reads data from the serial port
func (sm *SerialManager) readLoop() {
	buf := make([]byte, 1024)
	
	for {
		select {
		case <-sm.ctx.Done():
			return
		default:
			sm.mu.RLock()
			port := sm.port
			sm.mu.RUnlock()

			if port == nil {
				// Wait for connection
				continue
			}

			n, err := port.Read(buf)
			if err != nil {
				if err == io.EOF {
					// Connection closed
					sm.errChan <- fmt.Errorf("串口连接已关闭")
					return
				}
				sm.errChan <- fmt.Errorf("读取错误: %w", err)
				continue
			}

			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case sm.dataChan <- data:
					// Data sent
				case <-sm.ctx.Done():
					return
				default:
					// Channel full, drop data
					sm.logger.Warnln("[SerialHub] 数据通道已满，丢弃数据")
				}
			}
		}
	}
}