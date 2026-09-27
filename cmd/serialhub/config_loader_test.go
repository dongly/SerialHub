package main

import (
	"path/filepath"
	"testing"

	"github.com/dongly/serialhub/pkg/config"
)

// TestLoadConfig回填全局端口 验证配置文件指定的端口传导到全局 mcpPort
// （联邦发现与 HTTP 监听均使用该值，而非停留在 flag 默认值）。
func TestLoadConfig回填全局端口(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	// 准备：配置文件指定端口 1234
	base := config.GetDefault()
	base.MCP.HTTPPort = 1234
	if err := config.Save(cfgPath, base); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	// 隔离并复位全局 flag 变量为默认值状态（模拟未传 CLI 参数）
	oldPort, oldHost, oldCfgPath := mcpPort, host, configPath
	mcpPort, host, configPath = config.DefaultHTTPPort, "127.0.0.1", cfgPath
	defer func() { mcpPort, host, configPath = oldPort, oldHost, oldCfgPath }()

	loadConfig()

	if mcpPort != 1234 {
		t.Errorf("配置文件指定 1234 后全局 mcpPort = %d, 期望 1234（发现/监听应使用实际生效端口）", mcpPort)
	}
}

// TestLoadConfig命令行端口优先 验证 CLI 显式指定的端口覆盖配置文件并回填全局。
func TestLoadConfig命令行端口优先(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	base := config.GetDefault()
	base.MCP.HTTPPort = 1234
	if err := config.Save(cfgPath, base); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	oldPort, oldHost, oldCfgPath := mcpPort, host, configPath
	mcpPort, host, configPath = 5555, "127.0.0.1", cfgPath
	defer func() { mcpPort, host, configPath = oldPort, oldHost, oldCfgPath }()

	loadConfig()

	if mcpPort != 5555 {
		t.Errorf("CLI 指定 5555 时全局 mcpPort = %d, 期望 5555（CLI 优先于配置文件）", mcpPort)
	}
}
