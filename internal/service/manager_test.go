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
	testutil.AssertEqual(t, filepath.Join(os.TempDir(), "serialhub_last_serial.json"), sm.lastSerialPath)
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

	// 清理
	_ = sm.ClearStatus()
}

func TestServiceManager_ReadStatus(t *testing.T) {
	sm := NewServiceManager()

	// 清理
	_ = sm.ClearStatus()

	// 先写入再读取
	err := sm.WriteStatus(5000)
	testutil.AssertNoError(t, err)

	status, err := sm.ReadStatus()
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "running", status["status"])
	testutil.AssertEqual(t, float64(5000), status["mcpPort"].(float64))

	// 清理
	_ = sm.ClearStatus()
}

func TestServiceManager_ReadStatus_NotExist(t *testing.T) {
	sm := NewServiceManager()

	// 清理
	_ = sm.ClearStatus()

	// 读取不存在的文件
	_, err := sm.ReadStatus()
	testutil.AssertError(t, err)
}

func TestServiceManager_ClearStatus(t *testing.T) {
	sm := NewServiceManager()

	// 写入状态
	err := sm.WriteStatus(5000)
	testutil.AssertNoError(t, err)

	// 清除
	err = sm.ClearStatus()
	testutil.AssertNoError(t, err)

	// 验证文件不存在
	_, err = os.Stat(sm.statusPath)
	testutil.AssertEqual(t, true, os.IsNotExist(err))

	// 清除不存在的文件不应报错
	err = sm.ClearStatus()
	testutil.AssertNoError(t, err)
}

func TestServiceManager_SaveLastSerial(t *testing.T) {
	sm := NewServiceManager()

	// 清理
	_ = sm.ClearLastSerial()

	cfg := &LastSerialConfig{
		Port:     "COM9",
		BaudRate: 115200,
		DataBits: 8,
		Parity:   "none",
		StopBits: 1,
	}

	err := sm.SaveLastSerial(cfg)
	testutil.AssertNoError(t, err)

	// 验证文件存在
	_, err = os.Stat(sm.lastSerialPath)
	testutil.AssertNoError(t, err)

	// 清理
	_ = sm.ClearLastSerial()
}

func TestServiceManager_LoadLastSerial(t *testing.T) {
	sm := NewServiceManager()

	// 清理
	_ = sm.ClearLastSerial()

	// 先保存再加载
	cfg := &LastSerialConfig{
		Port:     "COM9",
		BaudRate: 115200,
		DataBits: 8,
		Parity:   "none",
		StopBits: 1,
	}

	err := sm.SaveLastSerial(cfg)
	testutil.AssertNoError(t, err)

	loaded, err := sm.LoadLastSerial()
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "COM9", loaded.Port)
	testutil.AssertEqual(t, 115200, loaded.BaudRate)
	testutil.AssertEqual(t, 8, loaded.DataBits)
	testutil.AssertEqual(t, "none", loaded.Parity)
	testutil.AssertEqual(t, float32(1), loaded.StopBits)

	// 清理
	_ = sm.ClearLastSerial()
}

func TestServiceManager_LoadLastSerial_NotExist(t *testing.T) {
	sm := NewServiceManager()

	// 清理
	_ = sm.ClearLastSerial()

	// 加载不存在的配置
	loaded, err := sm.LoadLastSerial()
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, loaded == nil)
}

func TestServiceManager_ClearLastSerial(t *testing.T) {
	sm := NewServiceManager()

	// 保存配置
	cfg := &LastSerialConfig{Port: "COM9"}
	err := sm.SaveLastSerial(cfg)
	testutil.AssertNoError(t, err)

	// 清除
	err = sm.ClearLastSerial()
	testutil.AssertNoError(t, err)

	// 验证文件不存在
	_, err = os.Stat(sm.lastSerialPath)
	testutil.AssertEqual(t, true, os.IsNotExist(err))

	// 清除不存在的文件不应报错
	err = sm.ClearLastSerial()
	testutil.AssertNoError(t, err)
}

func TestLastSerialConfig_WithDifferentStopBits(t *testing.T) {
	sm := NewServiceManager()

	// 清理
	_ = sm.ClearLastSerial()

	// 测试 1.5 停止位
	cfg := &LastSerialConfig{
		Port:     "COM9",
		BaudRate: 115200,
		DataBits: 8,
		Parity:   "none",
		StopBits: 1.5,
	}

	err := sm.SaveLastSerial(cfg)
	testutil.AssertNoError(t, err)

	loaded, err := sm.LoadLastSerial()
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, float32(1.5), loaded.StopBits)

	// 清理
	_ = sm.ClearLastSerial()
}
