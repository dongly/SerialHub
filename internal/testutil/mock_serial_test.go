package testutil

import (
	"errors"
	"io"
	"testing"
	"time"

	serial "go.bug.st/serial"
)

func TestMockSerialPort_Read_Success(t *testing.T) {
	port := NewMockSerialPort([]byte("hello world"))
	buf := make([]byte, 100)
	
	n, err := port.Read(buf)
	AssertNoError(t, err)
	AssertEqual(t, 11, n)
	AssertEqual(t, "hello world", string(buf[:n]))
}

func TestMockSerialPort_Read_EOF(t *testing.T) {
	port := NewMockSerialPort([]byte("test"))
	buf := make([]byte, 100)
	
	// 第一次读取
	_, err := port.Read(buf)
	AssertNoError(t, err)
	
	// 第二次读取应该返回 EOF
	_, err = port.Read(buf)
	AssertEqual(t, io.EOF, err)
}

func TestMockSerialPort_Read_Partial(t *testing.T) {
	port := NewMockSerialPort([]byte("hello world"))
	buf := make([]byte, 5)
	
	n, err := port.Read(buf)
	AssertNoError(t, err)
	AssertEqual(t, 5, n)
	AssertEqual(t, "hello", string(buf))
}

func TestMockSerialPort_Read_Error(t *testing.T) {
	port := NewMockSerialPort([]byte("test"))
	testErr := errors.New("读取错误")
	port.ReadErr = testErr
	
	buf := make([]byte, 100)
	_, err := port.Read(buf)
	AssertError(t, err)
	AssertEqual(t, testErr, err)
}

func TestMockSerialPort_Read_Closed(t *testing.T) {
	port := NewMockSerialPort([]byte("test"))
	port.Close()
	
	buf := make([]byte, 100)
	_, err := port.Read(buf)
	AssertEqual(t, io.ErrClosedPipe, err)
}

func TestMockSerialPort_Write(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	n, err := port.Write([]byte("hello"))
	AssertNoError(t, err)
	AssertEqual(t, 5, n)
	
	data := port.GetWriteData()
	AssertEqual(t, "hello", string(data))
}

func TestMockSerialPort_Write_Error(t *testing.T) {
	port := NewMockSerialPort(nil)
	testErr := errors.New("写入错误")
	port.WriteErr = testErr
	
	_, err := port.Write([]byte("test"))
	AssertError(t, err)
	AssertEqual(t, testErr, err)
}

func TestMockSerialPort_Write_Closed(t *testing.T) {
	port := NewMockSerialPort(nil)
	port.Close()
	
	_, err := port.Write([]byte("test"))
	AssertEqual(t, io.ErrClosedPipe, err)
}

func TestMockSerialPort_Close(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	AssertEqual(t, false, port.IsClosed())
	AssertNoError(t, port.Close())
	AssertEqual(t, true, port.IsClosed())
}

func TestMockSerialPort_Close_Idempotent(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	AssertNoError(t, port.Close())
	AssertNoError(t, port.Close())
	AssertEqual(t, true, port.IsClosed())
}

func TestMockSerialPort_SetMode(t *testing.T) {
	port := NewMockSerialPort(nil)
	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
	}
	
	err := port.SetMode(mode)
	AssertNoError(t, err)
}

func TestMockSerialPort_Drain(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	err := port.Drain()
	AssertNoError(t, err)
}

func TestMockSerialPort_ResetInputBuffer(t *testing.T) {
	port := NewMockSerialPort([]byte("test data"))
	buf := make([]byte, 100)
	
	// 读取部分数据
	port.Read(buf)
	
	// 重置输入缓冲区
	err := port.ResetInputBuffer()
	AssertNoError(t, err)
	
	// 再次读取应该返回 EOF
	_, err = port.Read(buf)
	AssertEqual(t, io.EOF, err)
}

func TestMockSerialPort_ResetOutputBuffer(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	port.Write([]byte("test data"))
	port.ResetOutputBuffer()
	
	data := port.GetWriteData()
	AssertEqual(t, 0, len(data))
}

func TestMockSerialPort_SetDTR(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	err := port.SetDTR(true)
	AssertNoError(t, err)
	
	err = port.SetDTR(false)
	AssertNoError(t, err)
}

func TestMockSerialPort_SetRTS(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	err := port.SetRTS(true)
	AssertNoError(t, err)
	
	err = port.SetRTS(false)
	AssertNoError(t, err)
}

func TestMockSerialPort_GetModemStatusBits(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	bits, err := port.GetModemStatusBits()
	AssertNoError(t, err)
	AssertNotNil(t, bits)
}

func TestMockSerialPort_SetReadTimeout(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	err := port.SetReadTimeout(time.Second)
	AssertNoError(t, err)
	
	err = port.SetReadTimeout(0)
	AssertNoError(t, err)
}

func TestMockSerialPort_Break(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	err := port.Break(time.Millisecond * 100)
	AssertNoError(t, err)
}

func TestMockSerialPort_Reset(t *testing.T) {
	port := NewMockSerialPort([]byte("original"))
	
	// 写入一些数据
	port.Write([]byte("written"))
	
	// 重置
	port.Reset([]byte("new"))
	
	// 检查状态
	data := port.GetWriteData()
	AssertEqual(t, 0, len(data))
	AssertEqual(t, false, port.IsClosed())
	
	buf := make([]byte, 100)
	n, err := port.Read(buf)
	AssertNoError(t, err)
	AssertEqual(t, 3, n)
	AssertEqual(t, "new", string(buf[:n]))
}

func TestMockSerialPort_GetWriteData_Copy(t *testing.T) {
	port := NewMockSerialPort(nil)
	
	port.Write([]byte("test"))
	data1 := port.GetWriteData()
	data2 := port.GetWriteData()
	
	// 修改第一个副本不应该影响第二个
	data1[0] = 'X'
	AssertEqual(t, byte('t'), data2[0])
}
