// Package buffer provides data buffering utilities.
package buffer

import (
	"sync"
)

// DataBuffer 提供线程安全的数据缓冲区，支持溢出时丢弃旧数据
type DataBuffer struct {
	mu      sync.Mutex
	buffer  []byte
	maxSize int
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
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(data) == 0 {
		return
	}

	newLen := len(b.buffer) + len(data)
	if newLen > b.maxSize {
		// 溢出：丢弃旧数据，保留最新的 maxSize 字节
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
