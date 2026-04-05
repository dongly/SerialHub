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
