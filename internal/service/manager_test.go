// Package service 服务状态管理测试
package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadStatus(t *testing.T) {
	sm := NewServiceManager()

	// 写入状态
	if err := sm.WriteStatus(5000); err != nil {
		t.Fatalf("WriteStatus failed: %v", err)
	}

	// 读取状态
	status, err := sm.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus failed: %v", err)
	}

	if status.Port != 5000 {
		t.Errorf("Port = %d, want 5000", status.Port)
	}

	// 清理
	sm.ClearStatus()
}

func TestReadStatus_NoFile(t *testing.T) {
	sm := NewServiceManager()

	// 确保文件不存在
	sm.ClearStatus()

	status, err := sm.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus failed: %v", err)
	}

	if status.Running {
		t.Error("Running should be false when no status file")
	}
}

func TestClearStatus(t *testing.T) {
	sm := NewServiceManager()

	// 写入状态
	sm.WriteStatus(5000)

	// 清理
	if err := sm.ClearStatus(); err != nil {
		t.Fatalf("ClearStatus failed: %v", err)
	}

	// 验证文件不存在
	if _, err := os.Stat(sm.pidFile); !os.IsNotExist(err) {
		t.Error("PID file should be removed")
	}
	if _, err := os.Stat(sm.portFile); !os.IsNotExist(err) {
		t.Error("Port file should be removed")
	}
}

func TestStaleCleanup(t *testing.T) {
	sm := NewServiceManager()

	// 写入无效的 PID（999999 不可能存在）
	os.MkdirAll(sm.runtimeDir, 0755)
	os.WriteFile(sm.pidFile, []byte("999999"), 0644)
	os.WriteFile(sm.portFile, []byte("5000"), 0644)

	// 读取状态应该自动清理
	status, err := sm.ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus failed: %v", err)
	}

	if status.Running {
		t.Error("Running should be false for stale process")
	}
}

func TestCheckHealth_Failure(t *testing.T) {
	sm := NewServiceManager()

	// 检查不存在的端口
	healthy, err := sm.CheckHealth(99999)
	if err != nil {
		t.Fatalf("CheckHealth failed: %v", err)
	}

	if healthy {
		t.Error("Health check should fail for non-existent port")
	}
}
