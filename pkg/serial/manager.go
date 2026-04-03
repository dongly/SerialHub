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
	"github.com/yourname/serialhub/internal/service"
)

// EventType 定义串口事件类型
type EventType string

const (
	EventConnected    EventType = "connected"
	EventDisconnected EventType = "disconnected"
	EventError        EventType = "error"
)

// Event 表示串口状态事件
type Event struct {
	Type    EventType
	Port    string
	Message string
}

// EventHandler 是串口事件处理函数类型
type EventHandler func(event Event)

// Port defines the serial port interface for dependency injection
type Port interface {
	io.Reader
	io.Writer
	io.Closer
	SetReadTimeout(timeout time.Duration) error
}

// SerialManager manages serial port connections
type SerialManager struct {
	config       *Config
	port         Port
	dataChan     chan []byte
	errChan      chan error
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	logger       *logrus.Logger
	eventHandler EventHandler
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

	logrus.Debugf("[SerialHub] 尝试连接串口: %s", sm.config.String())

	mode, err := sm.config.ToMode()
	if err != nil {
		logrus.Errorf("[SerialHub] 串口配置转换失败: %v", err)
		return fmt.Errorf("配置转换失败: %w", err)
	}

	logrus.Debugf("[SerialHub] 串口模式: BaudRate=%d, DataBits=%d, Parity=%v, StopBits=%v",
		mode.BaudRate, mode.DataBits, mode.Parity, mode.StopBits)

	port, err := serial.Open(sm.config.Port, mode)
	if err != nil {
		logrus.Errorf("[SerialHub] 打开串口失败 %s: %v", sm.config.Port, err)
		return fmt.Errorf("打开串口失败: %w", err)
	}

	sm.port = port
	logrus.Infof("[SerialHub] 串口已连接: %s", sm.config.String())

	svc := service.NewServiceManager()
	if err := svc.SaveLastSerial(&service.LastSerialConfig{
		Port:     sm.config.Port,
		BaudRate: sm.config.BaudRate,
		DataBits: sm.config.DataBits,
		Parity:   sm.config.Parity,
		StopBits: sm.config.StopBits,
	}); err != nil {
		logrus.Warnf("[SerialHub] 保存串口配置失败: %v", err)
	}

	go sm.readLoop()

	sm.emitEvent(Event{
		Type: EventConnected,
		Port: sm.config.Port,
	})

	return nil
}

// Disconnect disconnects from the serial port
func (sm *SerialManager) Disconnect() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.port == nil {
		return fmt.Errorf("串口未连接")
	}

	portName := sm.config.Port
	logrus.Debugf("[SerialHub] 正在断开串口: %s", portName)

	err := sm.port.Close()
	sm.port = nil
	logrus.Infof("[SerialHub] 串口已断开: %s", portName)

	sm.emitEvent(Event{
		Type: EventDisconnected,
		Port: portName,
	})

	return err
}

// Write writes data to the serial port
func (sm *SerialManager) Write(data []byte) (int, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.port == nil {
		return 0, fmt.Errorf("串口未连接")
	}

	logrus.Debugf("[SerialHub] 串口写入 %d 字节: %q", len(data), string(data))

	n, err := sm.port.Write(data)
	if err != nil {
		logrus.Errorf("[SerialHub] 串口写入失败: %v", err)
		sm.errChan <- fmt.Errorf("写入错误: %w", err)
		return n, fmt.Errorf("写入失败: %w", err)
	}

	logrus.Debugf("[SerialHub] 串口写入成功: %d 字节", n)
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

// SetEventHandler 设置串口事件处理函数
func (sm *SerialManager) SetEventHandler(handler EventHandler) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.eventHandler = handler
}

// emitEvent 触发事件
// 注意：调用者可能持有 sm.mu 写锁，因此这里不能再获取锁。
// eventHandler 在启动时一次性设置，之后只读，无需加锁。
func (sm *SerialManager) emitEvent(event Event) {
	handler := sm.eventHandler
	if handler == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("[SerialHub] 事件处理 panic: %v", r)
			}
		}()
		handler(event)
	}()
}
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
	logrus.Debug("[SerialHub] 串口读取循环已启动")

	for {
		select {
		case <-sm.ctx.Done():
			logrus.Debug("[SerialHub] 串口读取循环已停止")
			return
		default:
			sm.mu.RLock()
			port := sm.port
			sm.mu.RUnlock()

			if port == nil {
				continue
			}

			n, err := port.Read(buf)
			if err != nil {
				if err == io.EOF {
					logrus.Warn("[SerialHub] 串口连接已关闭 (EOF)")
					sm.errChan <- fmt.Errorf("串口连接已关闭")
					sm.emitEvent(Event{
						Type:    EventDisconnected,
						Port:    sm.CurrentPort(),
						Message: "连接已关闭",
					})
					return
				}
				errMsg := fmt.Errorf("读取错误: %w", err)
				logrus.Errorf("[SerialHub] 串口读取错误: %v", err)
				sm.errChan <- errMsg
				sm.emitEvent(Event{
					Type:    EventError,
					Port:    sm.CurrentPort(),
					Message: errMsg.Error(),
				})
				continue
			}

			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				logrus.Debugf("[SerialHub] 串口读取 %d 字节: %q", n, string(data))
				select {
				case sm.dataChan <- data:
				case <-sm.ctx.Done():
					return
				default:
					logrus.Warnln("[SerialHub] 数据通道已满，丢弃数据")
				}
			}
		}
	}
}
