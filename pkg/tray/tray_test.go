// Package tray 系统托盘测试
package tray

import (
	"testing"

	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/serial"
)

func TestTrayState(t *testing.T) {
	// 测试状态切换
	states := []TrayState{TrayIdle, TrayConnected, TrayError}
	for _, state := range states {
		if state != TrayIdle && state != TrayConnected && state != TrayError {
			t.Errorf("无效的状态: %s", state)
		}
	}
}

func TestNewTrayManager(t *testing.T) {
	serialMgr := serial.NewSerialManager(serial.DefaultConfig())
	cfg := config.GetDefault()

	tray := NewTrayManager(serialMgr, cfg, 2323, 5000, "0.1.0")
	if tray == nil {
		t.Fatal("NewTrayManager 返回 nil")
	}

	if tray.version != "0.1.0" {
		t.Errorf("version = %s, want 0.1.0", tray.version)
	}
	if tray.telnetPort != 2323 {
		t.Errorf("telnetPort = %d, want 2323", tray.telnetPort)
	}
	if tray.mcpPort != 5000 {
		t.Errorf("mcpPort = %d, want 5000", tray.mcpPort)
	}
	if tray.state != TrayIdle {
		t.Errorf("初始状态 = %s, want idle", tray.state)
	}
}

func TestUpdateState(t *testing.T) {
	serialMgr := serial.NewSerialManager(serial.DefaultConfig())
	cfg := config.GetDefault()
	tray := NewTrayManager(serialMgr, cfg, 2323, 5000, "0.1.0")

	tray.UpdateState(TrayConnected)
	if tray.state != TrayConnected {
		t.Errorf("UpdateState 后 state = %s, want connected", tray.state)
	}

	tray.UpdateState(TrayError)
	if tray.state != TrayError {
		t.Errorf("UpdateState 后 state = %s, want error", tray.state)
	}
}
