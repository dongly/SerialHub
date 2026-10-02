package mcpsetup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tailscale/hujson"
)

// jsonc 含注释与尾逗号：写回保留注释与尾逗号，条目写入且输出仍是合法 JSONC
func TestMergeJSON_PreservesCommentsInJsonc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.jsonc")
	orig := `{
  // 顶层注释
  "mcp": {
    "servers": {
      // 已有服务
      "other": {"type": "local", "command": ["x"]}
    }
  },
  "theme": "dark",
}
`
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := mergeJSON(path, "mcp.servers", "serialhub",
		map[string]any{"type": "local", "command": []string{"serialhub", "--stdio"}}, nil)
	if err != nil {
		t.Fatalf("mergeJSON: %v", err)
	}
	if written != path {
		t.Fatalf("应写入 %s，得到 %s", path, written)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "// 顶层注释") || !strings.Contains(string(raw), "// 已有服务") {
		t.Fatalf("注释应保留:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"dark",`) {
		t.Fatalf("尾逗号应保留:\n%s", raw)
	}
	if _, perr := hujson.Parse(raw); perr != nil {
		t.Fatalf("输出应是合法 JSONC: %v\n%s", perr, raw)
	}
	if !strings.Contains(string(raw), `"serialhub"`) {
		t.Fatalf("条目未写入:\n%s", raw)
	}
}

// opencode.json 与 opencode.jsonc 并存：写入 jsonc，json 原样不动
func TestMergeJSON_PrefersJsoncWhenBothExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	jsonc := filepath.Join(dir, "opencode.jsonc")
	jsonOrig := `{"mcp":{"servers":{"legacy":{"type":"local","command":["a"]}}}}` + "\n"
	if err := os.WriteFile(path, []byte(jsonOrig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonc, []byte("{\n  \"theme\": \"dark\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := mergeJSON(path, "mcp.servers", "serialhub",
		map[string]any{"type": "local", "command": []string{"serialhub", "--stdio"}}, nil)
	if err != nil {
		t.Fatalf("mergeJSON: %v", err)
	}
	if written != jsonc {
		t.Fatalf("并存时应写入 jsonc，得到 %s", written)
	}
	rawC, _ := os.ReadFile(jsonc)
	if !strings.Contains(string(rawC), `"serialhub"`) || !strings.Contains(string(rawC), `"theme"`) {
		t.Fatalf("jsonc 应含新条目且保留原有键:\n%s", rawC)
	}
	rawJ, _ := os.ReadFile(path)
	if string(rawJ) != jsonOrig {
		t.Fatalf("opencode.json 应原样不动:\n%s", rawJ)
	}
}

// 内容为空白的配置视为「无配置」：跳过写入且不覆盖空白文件
func TestMergeJSON_BlankFileIsNoConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte("  \n\t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := mergeJSON(path, "mcp.servers", "serialhub", map[string]any{"type": "local"}, nil)
	if !errors.Is(err, ErrNoConfig) {
		t.Fatalf("空白文件应视为无配置，得到 %v", err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "  \n\t\n" {
		t.Fatalf("空白文件不应被覆盖: %q", raw)
	}
}

// Install 层「存在才写」：目标配置不存在时报 ErrNoConfig 且不创建文件
func TestInstall_NoConfigDoesNotCreate(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, err := Install(Options{Client: "claude", Scope: ScopeProject, Mode: ModeStdio})
	if !errors.Is(err, ErrNoConfig) {
		t.Fatalf("期望 ErrNoConfig，得到 %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("配置不存在时不应创建 .mcp.json")
	}
}

// 卸载：json/jsonc 并存时两个文件都探测，条目在哪个文件就从哪个移除，注释保留
func TestUninstallJSON_ProbesBothAndKeepsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	jsonc := filepath.Join(dir, "opencode.jsonc")
	jsonOrig := `{"mcp":{"servers":{"serialhub":{"type":"local"}}}}` + "\n"
	jsoncOrig := `{
  // 服务列表
  "mcp": {
    "servers": {
      // 串口桥
      "serialhub": {"type": "local"},
      "other": {"type": "local"}
    }
  }
}
`
	if err := os.WriteFile(path, []byte(jsonOrig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonc, []byte(jsoncOrig), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := uninstallJSON(path, "mcp.servers", "serialhub")
	if err != nil {
		t.Fatalf("uninstallJSON: %v", err)
	}
	if !removed {
		t.Fatal("应报告已移除")
	}
	rawC, _ := os.ReadFile(jsonc)
	if strings.Contains(string(rawC), `"serialhub"`) {
		t.Fatalf("jsonc 中条目未移除:\n%s", rawC)
	}
	if !strings.Contains(string(rawC), "// 服务列表") {
		t.Fatalf("保留容器上的注释应留下（被删条目自身的注释随条目移除，属 AST trivia 语义）:\n%s", rawC)
	}
	if !strings.Contains(string(rawC), `"other"`) {
		t.Fatalf("邻居条目应保留:\n%s", rawC)
	}
	if _, perr := hujson.Parse(rawC); perr != nil {
		t.Fatalf("输出应是合法 JSONC: %v", perr)
	}
	rawJ, _ := os.ReadFile(path)
	if string(rawJ) == jsonOrig {
		t.Fatalf("json 中的条目也应被移除:\n%s", rawJ)
	}
	// 二次执行幂等
	removed, err = uninstallJSON(path, "mcp.servers", "serialhub")
	if err != nil || removed {
		t.Fatalf("二次移除应幂等: removed=%v err=%v", removed, err)
	}
}

// EntryExists：json/jsonc 任一存在即视为配置面存在，空白视为不存在
func TestEntryExists_ProbesBothAndBlank(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // 项目级路径是 cwd 相对的 opencode.json，需隔离
	path := filepath.Join(dir, "opencode.json")
	jsonc := filepath.Join(dir, "opencode.jsonc")
	opts := UninstallOptions{Client: "opencode", Scope: ScopeProject}

	if EntryExists(opts) {
		t.Fatal("两个文件都不存在时应为 false")
	}
	if err := os.WriteFile(path, []byte("  "), 0o644); err != nil {
		t.Fatal(err)
	}
	if EntryExists(opts) {
		t.Fatal("空白文件应视为无配置")
	}
	if err := os.WriteFile(jsonc, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !EntryExists(opts) {
		t.Fatal("jsonc 存在时应为 true")
	}
}

// CLI 型用户级（claude / codex）同样「存在才写」：配置缺失时报 ErrNoConfig，
// 且在探测阶段即返回，绝不触达官方 CLI
func TestInstall_CLIUserScopeNoConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	_, err := Install(Options{Client: "claude", Scope: ScopeUser, Mode: ModeStdio})
	if !errors.Is(err, ErrNoConfig) {
		t.Fatalf("claude 用户级无配置应跳过，得到 %v", err)
	}
	_, err = Install(Options{Client: "codex", Mode: ModeStdio})
	if !errors.Is(err, ErrNoConfig) {
		t.Fatalf("codex 无配置应跳过，得到 %v", err)
	}
}

// json 与 jsonc 两个候选经符号链接指向同一文件：按真实路径去重，
// 卸载只写回一次，不触发乐观冲突检测
func TestUninstallJSON_DedupSymlinkedCandidates(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "shared.json")
	if err := os.WriteFile(real, []byte(`{"mcp":{"servers":{"serialhub":{"type":"local"}}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "opencode.json")); err != nil {
		t.Skipf("当前环境不支持符号链接: %v", err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "opencode.jsonc")); err != nil {
		t.Skipf("当前环境不支持符号链接: %v", err)
	}
	removed, err := uninstallJSON(filepath.Join(dir, "opencode.json"), "mcp.servers", "serialhub")
	if err != nil {
		t.Fatalf("同一真实文件被双候选命中时不应报冲突: %v", err)
	}
	if !removed {
		t.Fatal("应报告已移除")
	}
	raw, _ := os.ReadFile(real)
	if strings.Contains(string(raw), "serialhub") {
		t.Fatalf("条目未移除:\n%s", raw)
	}
}

// 覆盖写回时保留被替换条目位置附近的注释
func TestMergeJSON_ReplaceKeepsMemberComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	orig := `{
  "mcpServers": {
    // serialhub 服务
    "serialhub": {"url": "http://old/mcp"}
  }
}
`
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := mergeJSON(path, "mcpServers", "serialhub",
		map[string]any{"url": "http://127.0.0.1:5050/mcp"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("mergeJSON: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "// serialhub 服务") {
		t.Fatalf("被替换条目旁的注释应保留:\n%s", raw)
	}
	if _, perr := hujson.Parse(raw); perr != nil {
		t.Fatalf("输出应是合法 JSONC: %v\n%s", perr, raw)
	}
	if !strings.Contains(string(raw), "http://127.0.0.1:5050/mcp") {
		t.Fatalf("条目未更新:\n%s", raw)
	}
}
