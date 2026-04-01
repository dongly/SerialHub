// Package testutil provides testing utilities.
package testutil

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// AssertContains checks if a string contains a substring.
func AssertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("期望字符串包含 %q, 但得到 %q", substr, s)
	}
}

// AssertNotContains checks if a string does not contain a substring.
func AssertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("期望字符串不包含 %q, 但得到 %q", substr, s)
	}
}

// AssertEqual checks if two values are equal.
func AssertEqual[T comparable](t *testing.T, expected, actual T) {
	t.Helper()
	if expected != actual {
		t.Errorf("期望 %v, 但得到 %v", expected, actual)
	}
}

// AssertNotEqual checks if two values are not equal.
func AssertNotEqual[T comparable](t *testing.T, expected, actual T) {
	t.Helper()
	if expected == actual {
		t.Errorf("期望值不相等, 但都为 %v", expected)
	}
}

// AssertNil checks if a value is nil.
func AssertNil(t *testing.T, v any) {
	t.Helper()
	if !isNil(v) {
		t.Errorf("期望 nil, 但得到 %v (type: %T)", v, v)
	}
}

// AssertNotNil checks if a value is not nil.
func AssertNotNil(t *testing.T, v any) {
	t.Helper()
	if isNil(v) {
		t.Errorf("期望非 nil, 但得到 nil")
	}
}

// isNil returns true if v is nil (including typed nil pointers).
func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return rv.IsNil()
	}
	return false
}

// AssertError checks if an error is not nil.
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Errorf("期望错误, 但得到 nil")
	}
}

// AssertNoError checks if an error is nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("期望无错误, 但得到 %v", err)
	}
}

// AssertBytesEqual checks if two byte slices are equal.
func AssertBytesEqual(t *testing.T, expected, actual []byte) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("期望字节长度 %d, 但得到 %d", len(expected), len(actual))
		return
	}
	for i := range expected {
		if expected[i] != actual[i] {
			t.Errorf("字节不匹配在位置 %d: 期望 %d, 得到 %d", i, expected[i], actual[i])
		}
	}
}

// WaitForChannel waits for a channel to receive a value or timeout.
func WaitForChannel[T any](t *testing.T, ch <-chan T, timeout time.Duration) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatalf("等待 channel 超时 (%v)", timeout)
		var zero T
		return zero
	}
}

// WaitForChannelOrFail waits for a channel to receive a value and fails on timeout.
func WaitForChannelOrFail[T any](t *testing.T, ch <-chan T, timeout time.Duration) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Helper()
		t.Fatalf("等待 channel 超时 (%v)", timeout)
		var zero T
		return zero
	}
}

// WaitForEmpty waits for a channel to be empty.
func WaitForEmpty[T any](t *testing.T, ch <-chan T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(ch) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待 channel 清空超时 (%v), 仍有 %d 个元素", timeout, len(ch))
}

// NewTempDir creates a new temporary directory for testing.
func NewTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "serialhub-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// NewTempFile creates a new temporary file for testing.
func NewTempFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp("", "serialhub-test-*.tmp")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	t.Cleanup(func() {
		f.Close()
		os.Remove(f.Name())
	})
	return f
}

// WriteTempFile writes content to a temporary file and returns its path.
func WriteTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f := NewTempFile(t)
	if _, err := f.Write(content); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("关闭临时文件失败: %v", err)
	}
	return f.Name()
}

// ReadFile reads the entire content of a file.
func ReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	return data
}

// FileExists checks if a file exists.
func FileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// AssertFileExists checks if a file exists.
func AssertFileExists(t *testing.T, path string) {
	t.Helper()
	if !FileExists(t, path) {
		t.Errorf("期望文件存在: %s", path)
	}
}

// AssertFileNotExists checks if a file does not exist.
func AssertFileNotExists(t *testing.T, path string) {
	t.Helper()
	if FileExists(t, path) {
		t.Errorf("期望文件不存在: %s", path)
	}
}

// AssertFileContains checks if a file contains a substring.
func AssertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	content := string(ReadFile(t, path))
	if !strings.Contains(content, substr) {
		t.Errorf("期望文件 %s 包含 %q, 但内容为 %q", path, substr, content)
	}
}

// TempFileInDir creates a temporary file in the specified directory.
func TempFileInDir(t *testing.T, dir, pattern string) string {
	t.Helper()
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		t.Fatalf("在目录 %s 创建临时文件失败: %v", dir, err)
	}
	t.Cleanup(func() {
		f.Close()
		os.Remove(f.Name())
	})
	return f.Name()
}

// JoinPath joins path elements safely.
func JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}

// MustGetwd returns the current working directory or panics.
func MustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前工作目录失败: %v", err)
	}
	return wd
}
