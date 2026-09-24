package mcpsetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// mergeJSON 应保留文件中的其他顶层键与其他服务器条目
func TestMergeJSON_保留既有条目(t *testing.T) {
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
		map[string]any{"url": "http://127.0.0.1:5000/mcp"},
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
	if servers["serialhub"].(map[string]any)["url"] != "http://127.0.0.1:5000/mcp" {
		t.Fatal("serialhub 条目未更新")
	}
}

// 拒绝覆盖时应返回 ErrEntryExists 且文件不变
func TestMergeJSON_拒绝覆盖(t *testing.T) {
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
func TestMergeJSON_新建文件(t *testing.T) {
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
		{"opencode项目HTTP", Options{Client: "opencode", Scope: ScopeProject, Mode: ModeHTTP, URL: "http://127.0.0.1:5000/mcp"},
			"opencode.json", "mcp", "type", "remote"},
		{"opencode用户stdio", Options{Client: "opencode", Scope: ScopeUser, Mode: ModeStdio},
			cfg(".config", "opencode", "opencode.json"), "mcp", "type", "local"},
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
func TestScope校验(t *testing.T) {
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
