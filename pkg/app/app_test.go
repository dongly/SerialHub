package app

import (
	"testing"

	"github.com/yourname/serialhub/pkg/config"
)

func TestNewApp_NoPort(t *testing.T) {
	cfg := config.GetDefault()
	app, err := NewApp("0.1.0", cfg)

	if err != nil {
		t.Fatalf("NewApp 返回错误: %v", err)
	}
	if app == nil {
		t.Fatal("NewApp 返回 nil")
	}
	if app.Version != "0.1.0" {
		t.Errorf("Version = %s, want 0.1.0", app.Version)
	}
	if app.Serial != nil {
		t.Error("无串口配置时 Serial 应为 nil")
	}
	if app.Buffer == nil {
		t.Error("Buffer 未初始化")
	}
	if app.Config != cfg {
		t.Error("Config 未正确设置")
	}
}

func TestNewApp_WithPort(t *testing.T) {
	cfg := config.GetDefault()
	cfg.Serial.Port = "COM_NONEXISTENT"
	app, err := NewApp("0.1.0", cfg)

	if err != nil {
		t.Fatalf("NewApp 不应在创建阶段报错: %v", err)
	}
	if app.Serial == nil {
		t.Fatal("有串口配置时 Serial 不应为 nil")
	}
}

func TestApp_Close(t *testing.T) {
	cfg := config.GetDefault()
	app, _ := NewApp("0.1.0", cfg)
	app.Close()
}
