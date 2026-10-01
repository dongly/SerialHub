// Package buffer provides data buffering utilities.
package buffer

import (
	"sync"
	"time"
)

// DataBuffer 提供线程安全的数据缓冲区，支持溢出时丢弃旧数据
type DataBuffer struct {
	mu      sync.Mutex
	buffer  []byte
	maxSize int
	// observers 数据到达观察者（非破坏性：仅供订阅方扫描新数据，
	// 不影响 Read/Peek 消费语义）。回调在 Append 临界区内同步执行：
	// 到达时刻捕获与观察者通知原子完成，订阅方的时序判定不被
	// 回调调度延迟干扰。回调内不得调用 DataBuffer 方法（重入死锁）。
	observers []bufferObserver
	nextObsID uint64
}

// bufferObserver 带唯一标识的观察者：函数值不可比较，
// 取消订阅按 id 精确移除。
type bufferObserver struct {
	id uint64
	fn func(data []byte, at time.Time)
}

// NewDataBuffer 创建新的数据缓冲区，maxSize 默认 65536 字节
func NewDataBuffer(maxSize ...int) *DataBuffer {
	size := 65536 // 默认 64KB
	if len(maxSize) > 0 && maxSize[0] > 0 {
		size = maxSize[0]
	}
	return &DataBuffer{
		buffer:  make([]byte, 0),
		maxSize: size,
	}
}

// Append 向缓冲区添加数据，超过 maxSize 时丢弃旧数据
func (b *DataBuffer) Append(data []byte) {
	if len(data) == 0 {
		return
	}

	b.mu.Lock()
	// 溢出时丢弃旧数据，保留最新的 maxSize 字节
	if newLen := len(b.buffer) + len(data); newLen > b.maxSize {
		discard := newLen - b.maxSize
		if discard >= len(b.buffer) {
			// 丢弃所有旧数据
			b.buffer = make([]byte, len(data))
			copy(b.buffer, data)
		} else {
			// 部分丢弃
			b.buffer = b.buffer[discard:]
			b.buffer = append(b.buffer, data...)
		}
	} else {
		// 正常添加
		b.buffer = append(b.buffer, data...)
	}
	// 到达时刻捕获与观察者通知同处 Append 临界区：二者原子完成，
	// 消除"解锁后回调被调度延迟"导致订阅方（如脚本超时判定）漏数据的窗口。
	// 契约：回调必须快速返回，且不得调用 DataBuffer 任何方法（重入死锁）。
	arrivedAt := time.Now()
	if len(b.observers) > 0 {
		// 传递副本：观察者不得持有引用（data 为调用方所有）
		chunk := make([]byte, len(data))
		copy(chunk, data)
		for _, o := range b.observers {
			o.fn(chunk, arrivedAt)
		}
	}
	b.mu.Unlock()
}

// Subscribe 注册数据到达观察者，返回取消订阅函数（幂等）。
// 观察者在 Append 临界区内被同步调用，收到该次追加的数据副本及其到达时刻
// （捕获与通知原子完成）。回调契约：必须快速返回，不得调用 DataBuffer 的
// 任何方法（重入死锁），不得阻塞。与 serial_read/WebSocket 的消费互不影响（tee 语义）。
func (b *DataBuffer) Subscribe(fn func(data []byte, at time.Time)) func() {
	b.mu.Lock()
	b.nextObsID++
	id := b.nextObsID
	b.observers = append(b.observers, bufferObserver{id: id, fn: fn})
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, o := range b.observers {
			if o.id == id {
				b.observers = append(b.observers[:i], b.observers[i+1:]...)
				break
			}
		}
	}
}

// Read 从缓冲区读取最多 maxSize 字节，并从缓冲区移除
func (b *DataBuffer) Read(maxSize int) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.buffer) == 0 {
		return nil
	}

	readSize := maxSize
	if readSize <= 0 || readSize > len(b.buffer) {
		readSize = len(b.buffer)
	}

	data := make([]byte, readSize)
	copy(data, b.buffer[:readSize])
	b.buffer = b.buffer[readSize:]

	return data
}

// Peek 从缓冲区查看最多 maxSize 字节，不从缓冲区移除
func (b *DataBuffer) Peek(maxSize int) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.buffer) == 0 {
		return nil
	}

	peekSize := maxSize
	if peekSize <= 0 || peekSize > len(b.buffer) {
		peekSize = len(b.buffer)
	}

	data := make([]byte, peekSize)
	copy(data, b.buffer[:peekSize])

	return data
}

// Clear 清空缓冲区
func (b *DataBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer = make([]byte, 0)
}

// Length 返回缓冲区当前数据长度
func (b *DataBuffer) Length() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.buffer)
}
