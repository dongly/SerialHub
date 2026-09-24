package serial

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dongly/serialhub/internal/testutil"
)

func TestReadLoop_NoRetryOnError(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}

	mockPort := testutil.NewMockSerialPort(nil)
	mockPort.ReadErr = fmt.Errorf("模拟读取错误")
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	var errorCount int32
	sm.SetEventHandler(func(event Event) {
		if event.Type == EventError {
			atomic.AddInt32(&errorCount, 1)
		}
	})

	sm.wg.Add(1)
	go sm.readLoop()

	time.Sleep(500 * time.Millisecond)

	sm.cancel()
	sm.wg.Wait()

	time.Sleep(100 * time.Millisecond)

	finalCount := atomic.LoadInt32(&errorCount)
	if finalCount != 1 {
		t.Errorf("错误事件次数 = %d, want 1（不应该重试）", finalCount)
	}
}

func TestDisconnect_NoRepeatedErrors(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	defer sm.Close()

	var disconnectCount int32
	sm.SetEventHandler(func(event Event) {
		if event.Type == EventDisconnected {
			atomic.AddInt32(&disconnectCount, 1)
		}
	})

	mockPort := testutil.NewMockSerialPort([]byte("some data"))
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	sm.wg.Add(1)
	go sm.readLoop()

	time.Sleep(50 * time.Millisecond)

	err = sm.Disconnect()
	if err != nil {
		t.Errorf("Disconnect() failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop 未能在断开连接后退出")
	}

	time.Sleep(200 * time.Millisecond)

	finalDisconnectCount := atomic.LoadInt32(&disconnectCount)
	if finalDisconnectCount < 1 {
		t.Errorf("断开连接事件次数 = %d, want >= 1", finalDisconnectCount)
	}
}

func TestReadLoop_PortClosedDuringRead(t *testing.T) {
	cfg := &Config{
		Port:     "MOCK1",
		BaudRate: 115200,
	}

	sm, err := NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}

	mockPort := &blockingMockPort{}
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	var errorCount int32
	sm.SetEventHandler(func(event Event) {
		if event.Type == EventError {
			atomic.AddInt32(&errorCount, 1)
		}
	})

	sm.wg.Add(1)
	go sm.readLoop()

	time.Sleep(100 * time.Millisecond)

	mockPort.Close()

	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop 未能在端口关闭后退出")
	}

	sm.cancel()

	time.Sleep(100 * time.Millisecond)

	finalCount := atomic.LoadInt32(&errorCount)
	if finalCount != 1 {
		t.Errorf("错误事件次数 = %d, want 1", finalCount)
	}
}

type blockingMockPort struct {
	closed bool
	mu     sync.Mutex
}

func (m *blockingMockPort) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return 0, fmt.Errorf("Port has been closed")
	}
	m.mu.Unlock()

	time.Sleep(100 * time.Millisecond)
	return 0, fmt.Errorf("Port has been closed")
}

func (m *blockingMockPort) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (m *blockingMockPort) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *blockingMockPort) SetReadTimeout(timeout time.Duration) error {
	return nil
}

func (m *blockingMockPort) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}
