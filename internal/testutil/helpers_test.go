package testutil

import (
	"errors"
	"testing"
	"time"
)

func TestAssertContains(t *testing.T) {
	AssertContains(t, "hello world", "world")
}

func TestAssertNotContains(t *testing.T) {
	AssertNotContains(t, "hello", "world")
}

func TestAssertEqual(t *testing.T) {
	AssertEqual(t, 42, 42)
	AssertEqual(t, "hello", "hello")
}

func TestAssertNotEqual(t *testing.T) {
	AssertNotEqual(t, 42, 43)
}

func TestAssertNil(t *testing.T) {
	var ptr *string
	AssertNil(t, ptr)
	
	// 测试 nil slice
	var slice []int
	AssertNil(t, slice)
	
	// 测试 nil map
	var m map[string]int
	AssertNil(t, m)
}

func TestAssertNotNil(t *testing.T) {
	s := "hello"
	AssertNotNil(t, s)
	
	// 测试非 nil pointer
	ptr := "test"
	AssertNotNil(t, &ptr)
}

func TestAssertError(t *testing.T) {
	testErr := errors.New("test error")
	AssertError(t, testErr)
}

func TestAssertNoError(t *testing.T) {
	err := error(nil)
	AssertNoError(t, err)
}

func TestAssertBytesEqual(t *testing.T) {
	AssertBytesEqual(t, []byte{1, 2, 3}, []byte{1, 2, 3})
}

func TestWaitForChannel_Success(t *testing.T) {
	ch := make(chan string)
	go func() {
		time.Sleep(10 * time.Millisecond)
		ch <- "hello"
	}()
	
	result := WaitForChannel(t, ch, time.Second)
	AssertEqual(t, "hello", result)
}

func TestNewTempDir(t *testing.T) {
	dir := NewTempDir(t)
	
	AssertNotEqual(t, "", dir)
	AssertFileExists(t, dir)
}

func TestNewTempFile(t *testing.T) {
	f := NewTempFile(t)
	
	AssertNotNil(t, f)
	AssertNotEqual(t, "", f.Name())
	
	err := f.Close()
	AssertNoError(t, err)
}

func TestWriteTempFile(t *testing.T) {
	content := []byte("test content")
	path := WriteTempFile(t, content)
	
	AssertFileExists(t, path)
	data := ReadFile(t, path)
	AssertBytesEqual(t, content, data)
}

func TestReadFile(t *testing.T) {
	path := WriteTempFile(t, []byte("file content"))
	
	data := ReadFile(t, path)
	AssertEqual(t, "file content", string(data))
}

func TestFileExists(t *testing.T) {
	path := WriteTempFile(t, []byte("test"))
	
	AssertEqual(t, true, FileExists(t, path))
	AssertEqual(t, false, FileExists(t, "nonexistent-file.txt"))
}

func TestAssertFileExists(t *testing.T) {
	path := WriteTempFile(t, []byte("test"))
	
	AssertFileExists(t, path)
}

func TestAssertFileNotExists(t *testing.T) {
	AssertFileNotExists(t, "nonexistent-file.txt")
}

func TestAssertFileContains(t *testing.T) {
	path := WriteTempFile(t, []byte("hello world"))
	
	AssertFileContains(t, path, "hello")
	AssertFileContains(t, path, "world")
}

func TestTempFileInDir(t *testing.T) {
	dir := NewTempDir(t)
	
	path := TempFileInDir(t, dir, "test-*.tmp")
	
	AssertFileExists(t, path)
	AssertContains(t, path, dir)
	AssertContains(t, path, "test-")
}

func TestJoinPath(t *testing.T) {
	result := JoinPath("a", "b", "c")
	AssertNotEqual(t, "", result)
	AssertContains(t, result, "a")
}

func TestMustGetwd(t *testing.T) {
	wd := MustGetwd(t)
	AssertNotEqual(t, "", wd)
}
