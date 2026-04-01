package buffer

import (
	"sync"
	"testing"
)

func TestBuffer_Append(t *testing.T) {
	buf := NewDataBuffer()

	buf.Append([]byte("hello"))
	if buf.Length() != 5 {
		t.Errorf("期望长度 5，实际 %d", buf.Length())
	}

	buf.Append([]byte(" world"))
	if buf.Length() != 11 {
		t.Errorf("期望长度 11，实际 %d", buf.Length())
	}
}

func TestBuffer_Read(t *testing.T) {
	buf := NewDataBuffer()

	// 测试空缓冲区
	data := buf.Read(10)
	if data != nil {
		t.Errorf("空缓冲区读取应返回 nil")
	}

	// 添加数据
	buf.Append([]byte("hello world"))

	// 读取部分
	data = buf.Read(5)
	if string(data) != "hello" {
		t.Errorf("读取 'hello'，实际 '%s'", string(data))
	}
	if buf.Length() != 6 {
		t.Errorf("读取后长度应为 6，实际 %d", buf.Length())
	}

	// 读取剩余
	data = buf.Read(10)
	if string(data) != " world" {
		t.Errorf("读取 ' world'，实际 '%s'", string(data))
	}
	if buf.Length() != 0 {
		t.Errorf("全部读取后长度应为 0，实际 %d", buf.Length())
	}
}

func TestBuffer_Read_Partial(t *testing.T) {
	buf := NewDataBuffer()
	buf.Append([]byte("hello"))

	// 请求大于实际数据量
	data := buf.Read(100)
	if string(data) != "hello" {
		t.Errorf("读取 'hello'，实际 '%s'", string(data))
	}
	if buf.Length() != 0 {
		t.Errorf("读取后长度应为 0，实际 %d", buf.Length())
	}

	// 再次测试空缓冲区
	data = buf.Read(10)
	if data != nil {
		t.Errorf("空缓冲区读取应返回 nil")
	}
}

func TestBuffer_Peek(t *testing.T) {
	buf := NewDataBuffer()

	// 测试空缓冲区
	data := buf.Peek(10)
	if data != nil {
		t.Errorf("空缓冲区查看应返回 nil")
	}

	// 添加数据
	buf.Append([]byte("hello world"))

	// 查看数据
	data = buf.Peek(5)
	if string(data) != "hello" {
		t.Errorf("查看 'hello'，实际 '%s'", string(data))
	}
	if buf.Length() != 11 {
		t.Errorf("查看后长度不应变化，应为 11，实际 %d", buf.Length())
	}

	// 再次查看
	data = buf.Peek(100)
	if string(data) != "hello world" {
		t.Errorf("查看 'hello world'，实际 '%s'", string(data))
	}
	if buf.Length() != 11 {
		t.Errorf("查看后长度不应变化，应为 11，实际 %d", buf.Length())
	}
}

func TestBuffer_Clear(t *testing.T) {
	buf := NewDataBuffer()
	buf.Append([]byte("hello world"))

	buf.Clear()
	if buf.Length() != 0 {
		t.Errorf("清空后长度应为 0，实际 %d", buf.Length())
	}

	// 验证可以继续使用
	buf.Append([]byte("new data"))
	if buf.Length() != 8 {
		t.Errorf("清空后添加数据长度应为 8，实际 %d", buf.Length())
	}
}

func TestBuffer_Overflow(t *testing.T) {
	maxSize := 10
	buf := NewDataBuffer(maxSize)

	// 填满缓冲区
	buf.Append([]byte("0123456789"))
	if buf.Length() != 10 {
		t.Errorf("填满后长度应为 10，实际 %d", buf.Length())
	}

	// 超过 maxSize，应丢弃旧数据
	buf.Append([]byte("abc"))
	if buf.Length() != 10 {
		t.Errorf("溢出后长度应为 10，实际 %d", buf.Length())
	}

	data := buf.Read(100)
	// 丢弃 3 字节 (0,1,2)，保留最新 10 字节
	if string(data) != "3456789abc" {
		t.Errorf("溢出后应保留最新数据 '3456789abc'，实际 '%s'", string(data))
	}

	// 测试完全覆盖
	buf = NewDataBuffer(5)
	buf.Append([]byte("01234"))
	buf.Append([]byte("6789")) // 长度 4，不超过 maxSize
	if buf.Length() != 5 {
		t.Errorf("添加后长度应为 5，实际 %d", buf.Length())
	}

	buf.Append([]byte("ABCDE")) // 长度 5，总长度 10，溢出 5
	if buf.Length() != 5 {
		t.Errorf("溢出后长度应为 5，实际 %d", buf.Length())
	}

	data = buf.Read(100)
	if string(data) != "ABCDE" {
		t.Errorf("完全覆盖后应为 'ABCDE'，实际 '%s'", string(data))
	}
}

func TestBuffer_ConcurrentAccess(t *testing.T) {
	buf := NewDataBuffer(1000)

	var wg sync.WaitGroup
	numGoroutines := 10
	numAppends := 100

	// 并发写入
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numAppends; j++ {
				data := []byte{byte(id % 10)}
				buf.Append(data)
			}
		}(i)
	}

	wg.Wait()

	expectedLen := numGoroutines * numAppends
	if expectedLen > 1000 {
		expectedLen = 1000 // 溢出后最多保留 maxSize
	}

	actualLen := buf.Length()
	if actualLen != expectedLen {
		t.Errorf("并发写入后长度应为 %d，实际 %d", expectedLen, actualLen)
	}
}

func TestBuffer_EmptyRead(t *testing.T) {
	buf := NewDataBuffer()

	// 空缓冲区读取
	data := buf.Read(10)
	if data != nil {
		t.Errorf("空缓冲区读取应返回 nil，实际长度 %d", len(data))
	}

	// 空缓冲区查看
	data = buf.Peek(10)
	if data != nil {
		t.Errorf("空缓冲区查看应返回 nil，实际长度 %d", len(data))
	}

	// 空数据追加
	buf.Append([]byte{})
	if buf.Length() != 0 {
		t.Errorf("追加空数据后长度应为 0，实际 %d", buf.Length())
	}

	// nil 数据追加
	buf.Append(nil)
	if buf.Length() != 0 {
		t.Errorf("追加 nil 后长度应为 0，实际 %d", buf.Length())
	}
}

func TestBuffer_NewDataBuffer(t *testing.T) {
	// 默认大小
	buf1 := NewDataBuffer()
	if buf1.maxSize != 65536 {
		t.Errorf("默认大小应为 65536，实际 %d", buf1.maxSize)
	}

	// 自定义大小
	buf2 := NewDataBuffer(1024)
	if buf2.maxSize != 1024 {
		t.Errorf("自定义大小应为 1024，实际 %d", buf2.maxSize)
	}

	// 零大小
	buf3 := NewDataBuffer(0)
	if buf3.maxSize != 65536 {
		t.Errorf("零大小应使用默认 65536，实际 %d", buf3.maxSize)
	}
}
