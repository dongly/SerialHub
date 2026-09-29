package mcpsetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// mergeJSON 应保留文件中的其他顶层键与其他服务器条目
func TestMergeJSON_KeepsExistingEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	orig := `{
  "mcpServers": {
    "other": {"url": "http://example.com/mcp"},
    "serialhub": {"url": "http://old/mcp"}
  },
  "别的配置": true
}`
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	called := false
	err := mergeJSON(path, "mcpServers", "serialhub",
		map[string]any{"url": "http://127.0.0.1:5050/mcp"},
		func(p string) bool { called = true; return true })
	if err != nil {
		t.Fatalf("mergeJSON: %v", err)
	}
	if !called {
		t.Fatal("已有条目时应调用 confirm 回调")
	}

	var got map[string]any
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("结果不是有效 JSON: %v", err)
	}
	servers := got["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Fatal("其他服务器条目被删除")
	}
	if got["别的配置"] != true {
		t.Fatal("其他顶层键被删除")
	}
	if servers["serialhub"].(map[string]any)["url"] != "http://127.0.0.1:5050/mcp" {
		t.Fatal("serialhub 条目未更新")
	}
}

// 拒绝覆盖时应返回 ErrEntryExists 且文件不变
func TestMergeJSON_RejectOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	orig := `{"mcp":{"serialhub":{"url":"http://old/mcp"}}}`
	os.WriteFile(path, []byte(orig), 0o644)

	err := mergeJSON(path, "mcp", "serialhub", map[string]any{"url": "new"}, func(string) bool { return false })
	if err != ErrEntryExists {
		t.Fatalf("期望 ErrEntryExists，得到 %v", err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != orig {
		t.Fatal("拒绝覆盖时文件不应被修改")
	}
}

// 文件不存在时应创建并建立嵌套目录
func TestMergeJSON_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "mcp.json")
	err := mergeJSON(path, "servers", "serialhub", map[string]any{"type": "http"}, nil)
	if err != nil {
		t.Fatalf("mergeJSON: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var got map[string]any
	json.Unmarshal(raw, &got)
	if _, ok := got["servers"].(map[string]any)["serialhub"]; !ok {
		t.Fatal("条目未写入")
	}
}

// 各客户端的写入目标与条目结构
func TestDefaultURL(t *testing.T) {
	if got := DefaultURL(); got != "http://127.0.0.1:5050/mcp" {
		t.Errorf("DefaultURL = %s, want http://127.0.0.1:5050/mcp", got)
	}
}

func TestTarget(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := func(rel ...string) string { return filepath.Join(append([]string{home}, rel...)...) }
	cases := []struct {
		name     string
		opts     Options
		path     string
		topKey   string
		checkKey string
		want     string
	}{
		{"opencode项目HTTP", Options{Client: "opencode", Scope: ScopeProject, Mode: ModeHTTP, URL: "http://127.0.0.1:5050/mcp"},
			"opencode.json", "mcp.servers", "type", "remote"},
		{"opencode用户stdio", Options{Client: "opencode", Scope: ScopeUser, Mode: ModeStdio},
			cfg(".config", "opencode", "opencode.json"), "mcp.servers", "type", "local"},
		{"claude项目HTTP", Options{Client: "claude", Scope: ScopeProject, Mode: ModeHTTP, URL: "u"},
			".mcp.json", "mcpServers", "type", "http"},
		{"cursor项目", Options{Client: "cursor", Scope: ScopeProject, Mode: ModeHTTP, URL: "u"},
			filepath.Join(".cursor", "mcp.json"), "mcpServers", "url", "u"},
		{"windsurf用户", Options{Client: "windsurf", Scope: ScopeUser, Mode: ModeHTTP, URL: "u"},
			cfg(".codeium", "windsurf", "mcp_config.json"), "mcpServers", "serverUrl", "u"},
		{"vscode项目stdio", Options{Client: "vscode", Scope: ScopeProject, Mode: ModeStdio},
			filepath.Join(".vscode", "mcp.json"), "servers", "type", "stdio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, topKey, entry, err := target(tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if path != tc.path || topKey != tc.topKey {
				t.Fatalf("path=%q topKey=%q，期望 %q %q", path, topKey, tc.path, tc.topKey)
			}
			if entry[tc.checkKey] != tc.want {
				t.Fatalf("entry[%q]=%v，期望 %v", tc.checkKey, entry[tc.checkKey], tc.want)
			}
		})
	}
}

// 不支持的层级应报错
func TestScopeValidation(t *testing.T) {
	if _, err := Install(Options{Client: "vscode", Scope: ScopeUser, Mode: ModeHTTP, URL: "u"}); err == nil {
		t.Fatal("vscode 用户级应报错")
	}
	if _, err := Install(Options{Client: "windsurf", Scope: ScopeProject, Mode: ModeHTTP, URL: "u"}); err == nil {
		t.Fatal("windsurf 项目级应报错")
	}
	if _, err := Install(Options{Client: "nope"}); err == nil {
		t.Fatal("未知客户端应报错")
	}
}

// OpenCode V2 结构：条目须嵌套在 mcp.servers 下；stdio 的 command 为数组；
// v0.5.0 写入的扁平 mcp.serialhub 残留应被迁移清理
func TestOpenCodeV2Structure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // 项目级 Install 写入当前目录 opencode.json，需隔离
	path := filepath.Join(dir, "opencode.json")
	// 模拟含 V1 扁平残留的既有配置
	orig := `{"mcp":{"serialhub":{"type":"remote","url":"http://old/mcp","enabled":true},"other":{"url":"x"}},"model":"m"}`
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(Options{
		Client: "opencode", Scope: ScopeProject, Mode: ModeHTTP, URL: "http://127.0.0.1:5050/mcp",
		ConfirmOverwrite: func(string) bool { return true },
	}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	mcp := cfg["mcp"].(map[string]any)
	if _, exists := mcp["serialhub"]; exists {
		t.Fatal("V1 扁平残留 mcp.serialhub 未被清理")
	}
	servers := mcp["servers"].(map[string]any)
	entry := servers["serialhub"].(map[string]any)
	if entry["type"] != "remote" || entry["url"] != "http://127.0.0.1:5050/mcp" {
		t.Fatalf("mcp.servers.serialhub 条目错误：%v", entry)
	}
	if _, has := entry["enabled"]; has {
		t.Fatal("V2 无 enabled 字段")
	}
	if mcp["other"] == nil {
		t.Fatal("mcp.other 邻居条目应保留")
	}
	if cfg["model"] != "m" {
		t.Fatal("其他顶层键应保留")
	}
}

// opencode stdio 模式的 command 应为「可执行文件+--stdio」数组（V2 无 args 字段）
func TestOpenCodeStdioCommandArray(t *testing.T) {
	_, _, entry, err := target(Options{Client: "opencode", Scope: ScopeUser, Mode: ModeStdio})
	if err != nil {
		t.Fatal(err)
	}
	cmd, ok := entry["command"].([]string)
	if !ok || len(cmd) != 2 || cmd[1] != "--stdio" {
		t.Fatalf("command 应为 [可执行文件, --stdio] 数组，得到 %v", entry["command"])
	}
	if _, has := entry["args"]; has {
		t.Fatal("V2 无独立 args 字段")
	}
}

// Mode 留空时应默认 stdio 本地模式（Install 填充，v0.6 起默认从 HTTP 改为 stdio）
func TestInstall_DefaultModeIsStdio(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // 项目级 Install 写入当前目录 opencode.json，需隔离
	path := filepath.Join(dir, "opencode.json")
	if _, err := Install(Options{
		Client: "opencode",
		Scope:  ScopeProject,
		URL:    "http://ignored-example:9999/mcp", // stdio 模式应忽略 URL
	}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 opencode.json: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("解析 opencode.json: %v", err)
	}
	mcpObj, ok := cfg["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 mcp 容器: %v", cfg)
	}
	servers, ok := mcpObj["servers"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 mcp.servers 容器: %v", mcpObj)
	}
	entry, ok := servers["serialhub"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 serialhub 条目: %v", servers)
	}
	if got := entry["type"]; got != "local" {
		t.Errorf("type 应为 local，得到 %v", got)
	}
	cmd, ok := entry["command"].([]any) // JSON 反序列化后是 []any 而非 []string
	if !ok || len(cmd) != 2 || cmd[0] != "serialhub" || cmd[1] != "--stdio" {
		t.Fatalf("默认模式应为 stdio（command=[serialhub --stdio]），得到 %v", entry)
	}
	if _, has := entry["url"]; has {
		t.Errorf("stdio 模式不应写入 url 字段: %v", entry)
	}
}
