package testutil

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestMockConn_Read_Success(t *testing.T) {
	conn := NewMockConn()
	conn.ReadData = []byte("hello from client")
	buf := make([]byte, 100)
	
	n, err := conn.Read(buf)
	AssertNoError(t, err)
	AssertEqual(t, 17, n)
	AssertEqual(t, "hello from client", string(buf[:n]))
}

func TestMockConn_Read_EOF(t *testing.T) {
	conn := NewMockConn()
	conn.ReadData = []byte("test")
	buf := make([]byte, 100)
	
	// 第一次读取
	_, err := conn.Read(buf)
	AssertNoError(t, err)
	
	// 第二次读取应该返回 EOF
	_, err = conn.Read(buf)
	AssertEqual(t, io.EOF, err)
}

func TestMockConn_Read_Partial(t *testing.T) {
	conn := NewMockConn()
	conn.ReadData = []byte("hello world")
	buf := make([]byte, 5)
	
	n, err := conn.Read(buf)
	AssertNoError(t, err)
	AssertEqual(t, 5, n)
	AssertEqual(t, "hello", string(buf))
}

func TestMockConn_Read_Error(t *testing.T) {
	conn := NewMockConn()
	testErr := errors.New("读取错误")
	conn.ReadErr = testErr
	
	buf := make([]byte, 100)
	_, err := conn.Read(buf)
	AssertError(t, err)
	AssertEqual(t, testErr, err)
}

func TestMockConn_Read_Closed(t *testing.T) {
	conn := NewMockConn()
	conn.ReadData = []byte("test")
	conn.Close()
	
	buf := make([]byte, 100)
	_, err := conn.Read(buf)
	AssertEqual(t, io.ErrClosedPipe, err)
}

func TestMockConn_Write(t *testing.T) {
	conn := NewMockConn()
	
	n, err := conn.Write([]byte("hello"))
	AssertNoError(t, err)
	AssertEqual(t, 5, n)
	
	data := conn.GetWriteData()
	AssertEqual(t, "hello", string(data))
}

func TestMockConn_Write_Error(t *testing.T) {
	conn := NewMockConn()
	testErr := errors.New("写入错误")
	conn.WriteErr = testErr
	
	_, err := conn.Write([]byte("test"))
	AssertError(t, err)
	AssertEqual(t, testErr, err)
}

func TestMockConn_Write_Closed(t *testing.T) {
	conn := NewMockConn()
	conn.Close()
	
	_, err := conn.Write([]byte("test"))
	AssertEqual(t, io.ErrClosedPipe, err)
}

func TestMockConn_Close(t *testing.T) {
	conn := NewMockConn()
	
	AssertEqual(t, false, conn.IsClosed())
	AssertNoError(t, conn.Close())
	AssertEqual(t, true, conn.IsClosed())
}

func TestMockConn_Close_Idempotent(t *testing.T) {
	conn := NewMockConn()
	
	AssertNoError(t, conn.Close())
	AssertNoError(t, conn.Close())
	AssertEqual(t, true, conn.IsClosed())
}

func TestMockConn_Close_ClosesChannel(t *testing.T) {
	conn := NewMockConn()
	
	// Channel 应该正常工作
	select {
	case <-conn.ClosedChan():
		t.Fatal("Channel 不应该立即关闭")
	default:
	}
	
	conn.Close()
	
	// Channel 应该已关闭
	select {
	case <-conn.ClosedChan():
		// 成功
	default:
		t.Fatal("Channel 应该已关闭")
	}
}

func TestMockConn_LocalAddr(t *testing.T) {
	conn := NewMockConn()
	
	addr := conn.LocalAddr()
	AssertNotNil(t, addr)
	AssertEqual(t, "tcp", addr.Network())
	AssertEqual(t, "127.0.0.1:2323", addr.String())
}

func TestMockConn_RemoteAddr(t *testing.T) {
	conn := NewMockConn()
	
	addr := conn.RemoteAddr()
	AssertNotNil(t, addr)
	AssertEqual(t, "tcp", addr.Network())
	AssertEqual(t, "127.0.0.1:54321", addr.String())
}

func TestMockConn_SetDeadline(t *testing.T) {
	conn := NewMockConn()
	
	err := conn.SetDeadline(time.Now().Add(time.Second))
	AssertNoError(t, err)
}

func TestMockConn_SetReadDeadline(t *testing.T) {
	conn := NewMockConn()
	
	err := conn.SetReadDeadline(time.Now().Add(time.Second))
	AssertNoError(t, err)
}

func TestMockConn_SetWriteDeadline(t *testing.T) {
	conn := NewMockConn()
	
	err := conn.SetWriteDeadline(time.Now().Add(time.Second))
	AssertNoError(t, err)
}

func TestMockConn_Reset(t *testing.T) {
	conn := NewMockConn()
	conn.ReadData = []byte("original")
	
	// 写入一些数据
	conn.Write([]byte("written"))
	
	// 重置
	conn.Reset([]byte("new"))
	
	// 检查状态
	data := conn.GetWriteData()
	AssertEqual(t, 0, len(data))
	AssertEqual(t, false, conn.IsClosed())
	
	buf := make([]byte, 100)
	n, err := conn.Read(buf)
	AssertNoError(t, err)
	AssertEqual(t, 3, n)
	AssertEqual(t, "new", string(buf[:n]))
	
	// Channel 应该是新的，不是关闭的
	select {
	case <-conn.ClosedChan():
		t.Fatal("Channel 不应该关闭")
	default:
	}
}

func TestMockConn_ConcurrentReadWrite(t *testing.T) {
	conn := NewMockConn()
	conn.ReadData = []byte("data")
	
	done := make(chan bool)
	
	// 并发读取
	go func() {
		buf := make([]byte, 100)
		conn.Read(buf)
		done <- true
	}()
	
	// 并发写入
	go func() {
		conn.Write([]byte("test"))
		done <- true
	}()
	
	// 等待两个操作完成
	<-done
	<-done
	
	AssertEqual(t, 4, len(conn.GetWriteData()))
}

func TestMockAddr(t *testing.T) {
	addr := NewMockAddr("192.168.1.1:8080")
	
	AssertEqual(t, "tcp", addr.Network())
	AssertEqual(t, "192.168.1.1:8080", addr.String())
}

func TestMockAddr_NetConnInterface(t *testing.T) {
	conn := NewMockConn()
	
	// 验证 MockConn 实现了 net.Conn 接口
	var _ net.Conn = conn
	
	AssertNotNil(t, conn.LocalAddr())
	AssertNotNil(t, conn.RemoteAddr())
}
