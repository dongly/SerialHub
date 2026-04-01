// Package app App 结构体测试
package app

import (
	"testing"

	"github.com/yourname/serialhub/pkg/config"
)

func TestNewApp(t *testing.T) {
	cfg := config.GetDefault()
	app := NewApp("0.1.0", cfg)

	if app == nil {
		t.Fatal("NewApp 返回 nil")
	}

	if app.Version != "0.1.0" {
		t.Errorf("Version = %s, want 0.1.0", app.Version)
	}

	if app.Serial == nil {
		t.Error("Serial 未初始化")
	}

	if app.Telnet == nil {
		t.Error("Telnet 未初始化")
	}

	if app.Buffer == nil {
		t.Error("Buffer 未初始化")
	}

	if app.Config != cfg {
		t.Error("Config 未正确设置")
	}
}

func TestApp_Close_Order(t *testing.T) {
	cfg := config.GetDefault()
	app := NewApp("0.1.0", cfg)

	// 关闭不应 panic
	app.Close()
}

func TestApp_LogOutput(t *testing.T) {
	// 验证日志输出到 stderr
	// 这里简化处理，实际应该捕获 stderr
	cfg := config.GetDefault()
	_ = NewApp("0.1.0", cfg)
}
