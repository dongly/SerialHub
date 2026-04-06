package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yourname/serialhub/internal/testutil"
)

func TestNewServiceManager(t *testing.T) {
	sm := NewServiceManager()
	testutil.AssertNotNil(t, sm)
	testutil.AssertEqual(t, filepath.Join(os.TempDir(), "serialhub_status.json"), sm.statusPath)
	testutil.AssertEqual(t, filepath.Join(os.TempDir(), "serialhub.pid"), sm.pidPath)
}

func TestServiceManager_WriteStatus(t *testing.T) {
	sm := NewServiceManager()

	// 清理
	_ = sm.ClearStatus()

	// 写入状态
	err := sm.WriteStatus(5000)
	testutil.AssertNoError(t, err)

	// 验证文件存在
	_, err = os.Stat(sm.statusPath)
	testutil.AssertNoError(t, err)

	// 读取状态
	status, err := sm.ReadStatus()
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, float64(5000), status["mcpPort"].(float64))
	testutil.AssertEqual(t, "running", status["status"])

	// 清理
	_ = sm.ClearStatus()
}

func TestServiceManager_ClearStatus(t *testing.T) {
	sm := NewServiceManager()

	// 先写入状态
	err := sm.WriteStatus(5000)
	testutil.AssertNoError(t, err)

	// 清除状态
	err = sm.ClearStatus()
	testutil.AssertNoError(t, err)

	// 验证文件不存在
	_, err = os.Stat(sm.statusPath)
	testutil.AssertEqual(t, true, os.IsNotExist(err))

	// 清除不存在的文件不应报错
	err = sm.ClearStatus()
	testutil.AssertNoError(t, err)
}

func TestServiceManager_ReadStatus_NotExist(t *testing.T) {
	sm := NewServiceManager()

	// 清理
	_ = sm.ClearStatus()

	// 读取不存在的状态
	_, err := sm.ReadStatus()
	testutil.AssertEqual(t, true, err != nil)
}

func TestServiceManager_ReadStatus_InvalidJSON(t *testing.T) {
	sm := NewServiceManager()

	// 写入无效的 JSON
	err := os.WriteFile(sm.statusPath, []byte("invalid json"), 0644)
	testutil.AssertNoError(t, err)

	// 读取应该失败
	_, err = sm.ReadStatus()
	testutil.AssertEqual(t, true, err != nil)

	// 清理
	_ = sm.ClearStatus()
}

func TestServiceManager_WriteStatus_MultiplePorts(t *testing.T) {
	sm := NewServiceManager()
	_ = sm.ClearStatus()

	// 写入不同端口的状态
	ports := []int{5000, 8080, 9000}
	for _, port := range ports {
		err := sm.WriteStatus(port)
		testutil.AssertNoError(t, err)

		status, err := sm.ReadStatus()
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, float64(port), status["mcpPort"].(float64))
	}

	_ = sm.ClearStatus()
}

func TestServiceManager_WriteStatus_ContainsPID(t *testing.T) {
	sm := NewServiceManager()
	_ = sm.ClearStatus()

	err := sm.WriteStatus(5000)
	testutil.AssertNoError(t, err)

	status, err := sm.ReadStatus()
	testutil.AssertNoError(t, err)

	// 验证包含 PID
	pid, ok := status["pid"].(float64)
	testutil.AssertEqual(t, true, ok)
	testutil.AssertEqual(t, float64(os.Getpid()), pid)

	_ = sm.ClearStatus()
}

// TestEnsureSingleInstance 测试单实例功能
func TestEnsureSingleInstance(t *testing.T) {
	// 先释放可能存在的锁
	ReleaseSingleInstance()

	// 第一次应该成功
	err := EnsureSingleInstance()
	testutil.AssertNoError(t, err)

	// 再次调用应该成功（幂等）
	err = EnsureSingleInstance()
	testutil.AssertNoError(t, err)

	// 释放锁
	ReleaseSingleInstance()
}

// TestReleaseSingleInstance 测试释放单实例
func TestReleaseSingleInstance(t *testing.T) {
	// 确保没有锁
	ReleaseSingleInstance()

	// 获取锁
	err := EnsureSingleInstance()
	testutil.AssertNoError(t, err)

	// 释放锁
	ReleaseSingleInstance()

	// 再次获取锁应该成功
	err = EnsureSingleInstance()
	testutil.AssertNoError(t, err)

	// 清理
	ReleaseSingleInstance()
}

// TestReleaseSingleInstance_NoLock 测试没有锁时释放不 panic
func TestReleaseSingleInstance_NoLock(t *testing.T) {
	// 确保 instanceLock 为 nil
	instanceLock = nil

	// 不应该 panic
	ReleaseSingleInstance()
}

// TestSingleInstanceLock_TryLockUnlock 测试单实例锁的获取和释放
func TestSingleInstanceLock_TryLockUnlock(t *testing.T) {
	// 先释放可能存在的锁
	ReleaseSingleInstance()

	lock := NewSingleInstanceLock()
	testutil.AssertNotNil(t, lock)
	testutil.AssertEqual(t, `Global\SerialHub_Single_Instance`, lock.mutexName)

	// 获取锁
	err := lock.TryLock()
	testutil.AssertNoError(t, err)

	// 释放锁
	err = lock.Unlock()
	testutil.AssertNoError(t, err)
}

// TestSingleInstanceLock_DoubleLock 测试重复获取锁
func TestSingleInstanceLock_DoubleLock(t *testing.T) {
	// 先释放可能存在的锁
	ReleaseSingleInstance()

	lock1 := NewSingleInstanceLock()
	err := lock1.TryLock()
	testutil.AssertNoError(t, err)

	// 第二个锁应该失败
	lock2 := NewSingleInstanceLock()
	err = lock2.TryLock()
	testutil.AssertError(t, err)

	// 清理
	lock1.Unlock()
}

// TestSingleInstanceLock_UnlockWithoutLock 测试未获取锁时释放
func TestSingleInstanceLock_UnlockWithoutLock(t *testing.T) {
	lock := NewSingleInstanceLock()
	// 不应该 panic
	err := lock.Unlock()
	testutil.AssertNoError(t, err)
}
