package mcpsetup

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestUninstallJSON(t *testing.T) {
	t.Run("删除条目并保留其他服务", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "opencode.json")
		writeTestFile(t, path, `{"mcp":{"servers":{"serialhub":{"type":"local"},"other":{"type":"local"}}}}`)
		removed, err := uninstallJSON(path, "mcp.servers", "serialhub")
		if err != nil || !removed {
			t.Fatalf("removed=%v err=%v", removed, err)
		}
		got := readTestFile(t, path)
		if strings.Contains(got, "serialhub") {
			t.Fatalf("serialhub 未删除: %s", got)
		}
		if !strings.Contains(got, "other") {
			t.Fatalf("其他服务被误删: %s", got)
		}
	})

	t.Run("容器清空后连容器一起删但root保留", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "opencode.json")
		writeTestFile(t, path, `{"theme":"dark","mcp":{"servers":{"serialhub":{"type":"local"}}}}`)
		removed, err := uninstallJSON(path, "mcp.servers", "serialhub")
		if err != nil || !removed {
			t.Fatalf("removed=%v err=%v", removed, err)
		}
		got := readTestFile(t, path)
		if strings.Contains(got, "mcp") {
			t.Fatalf("空容器 mcp 未清理: %s", got)
		}
		if !strings.Contains(got, "theme") {
			t.Fatalf("root 其他键被误删: %s", got)
		}
	})

	t.Run("文件不存在时幂等", func(t *testing.T) {
		removed, err := uninstallJSON(filepath.Join(t.TempDir(), "none.json"), "mcpServers", "serialhub")
		if err != nil || removed {
			t.Fatalf("removed=%v err=%v", removed, err)
		}
	})

	t.Run("条目不存在时幂等", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "mcp.json")
		writeTestFile(t, path, `{"mcpServers":{"other":{}}}`)
		removed, err := uninstallJSON(path, "mcpServers", "serialhub")
		if err != nil || removed {
			t.Fatalf("removed=%v err=%v", removed, err)
		}
		if !strings.Contains(readTestFile(t, path), "other") {
			t.Fatal("文件不应被修改")
		}
	})

	t.Run("中间路径不是对象时幂等", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		writeTestFile(t, path, `{"mcp":"不是对象"}`)
		removed, err := uninstallJSON(path, "mcp.servers", "serialhub")
		if err != nil || removed {
			t.Fatalf("removed=%v err=%v", removed, err)
		}
	})

	t.Run("解析失败报错", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		writeTestFile(t, path, "not-json")
		if _, err := uninstallJSON(path, "mcpServers", "serialhub"); err == nil {
			t.Fatal("期望解析错误")
		}
	})

	t.Run("读取失败如实报错", func(t *testing.T) {
		// path 是目录：ReadFile 返回 EISDIR，不应被当作「不存在」幂等跳过
		dir := t.TempDir()
		if _, err := uninstallJSON(dir, "mcpServers", "serialhub"); err == nil {
			t.Fatal("读取目录应报错而非幂等跳过")
		}
	})

	t.Run("写回保留原文件权限", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix 权限语义")
		}
		tmp := t.TempDir()
		path := filepath.Join(tmp, "opencode.json")
		os.WriteFile(path, []byte(`{"mcp":{"servers":{"serialhub":{},"keep":{}}}}`), 0o600)
		removed, err := uninstallJSON(path, "mcp.servers", "serialhub")
		if err != nil || !removed {
			t.Fatalf("removed=%v err=%v", removed, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("权限应保留 0600，实际 %v", info.Mode().Perm())
		}
	})
}

func TestUninstallFrom_项目级(t *testing.T) {
	t.Chdir(t.TempDir())

	t.Run("opencode", func(t *testing.T) {
		writeTestFile(t, "opencode.json", `{"mcp":{"servers":{"serialhub":{"type":"local"}},"keep":{}}}`)
		desc, removed, _, err := UninstallFrom(UninstallOptions{Client: "opencode", Scope: ScopeProject})
		if err != nil || !removed {
			t.Fatalf("removed=%v err=%v desc=%s", removed, err, desc)
		}
		got := readTestFile(t, "opencode.json")
		if strings.Contains(got, "serialhub") {
			t.Fatalf("未删除: %s", got)
		}
		if !strings.Contains(got, "keep") {
			t.Fatalf("mcp.keep 被误删: %s", got)
		}
	})

	t.Run("条目不存在时跳过", func(t *testing.T) {
		writeTestFile(t, "opencode.json", `{"mcp":{"servers":{}}}`)
		desc, removed, _, err := UninstallFrom(UninstallOptions{Client: "opencode", Scope: ScopeProject})
		if err != nil || removed {
			t.Fatalf("removed=%v err=%v", removed, err)
		}
		if !strings.Contains(desc, "跳过") {
			t.Fatalf("描述应含跳过: %s", desc)
		}
	})

	t.Run("层级不支持时报错", func(t *testing.T) {
		if _, _, _, err := UninstallFrom(UninstallOptions{Client: "windsurf", Scope: ScopeProject}); err == nil {
			t.Fatal("windsurf 不支持项目级，期望报错")
		}
	})
}

func TestUninstallFrom_用户级(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("cursor", func(t *testing.T) {
		writeTestFile(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers":{"serialhub":{"command":"serialhub"},"other":{}}}`)
		desc, removed, _, err := UninstallFrom(UninstallOptions{Client: "cursor", Scope: ScopeUser})
		if err != nil || !removed {
			t.Fatalf("removed=%v err=%v desc=%s", removed, err, desc)
		}
		got := readTestFile(t, filepath.Join(home, ".cursor", "mcp.json"))
		if strings.Contains(got, "serialhub") || !strings.Contains(got, "other") {
			t.Fatalf("删除不正确: %s", got)
		}
	})

	t.Run("windsurf", func(t *testing.T) {
		writeTestFile(t, filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"), `{"mcpServers":{"serialhub":{}}}`)
		_, removed, _, err := UninstallFrom(UninstallOptions{Client: "windsurf", Scope: ScopeUser})
		if err != nil || !removed {
			t.Fatalf("removed=%v err=%v", removed, err)
		}
	})
}

func TestUninstallFrom_官方CLI不存在时给出提示(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // 里面没有任何可执行文件
	for _, tc := range []struct {
		client string
		scope  Scope
		want   string
	}{
		{"codex", ScopeUser, "~/.codex/config.toml"},
		{"claude", ScopeUser, "claude mcp remove"},
	} {
		desc, removed, manual, err := UninstallFrom(UninstallOptions{Client: tc.client, Scope: tc.scope})
		if err != nil || removed {
			t.Fatalf("%s: removed=%v err=%v", tc.client, removed, err)
		}
		if !manual {
			t.Fatalf("%s: CLI 缺失的手动指引应置 manual=true", tc.client)
		}
		if !strings.Contains(desc, tc.want) {
			t.Fatalf("%s: 描述应含 %q: %s", tc.client, tc.want, desc)
		}
	}
}

// TestUninstallFrom_符号链接配置在真实目标上操作：dotfiles 管理的配置是符号链接时，
// 卸载应更新链接指向的真实文件，而不是把链接替换成普通文件（拆链接）。
func TestUninstallFrom_符号链接配置在真实目标上操作(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix 符号链接")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	dotfiles := t.TempDir()
	real := filepath.Join(dotfiles, "opencode.json")
	if err := os.WriteFile(real, []byte(`{"mcp":{"servers":{"serialhub":{"type":"local"},"keep":{}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cfgDir, "opencode.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	desc, removed, _, err := UninstallFrom(UninstallOptions{Client: "opencode", Scope: ScopeUser})
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v desc=%s", removed, err, desc)
	}

	// 链接本身不被替换
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("配置路径应保持符号链接，实际变成了 %v", info.Mode())
	}
	// 真实目标已更新：serialhub 删除、keep 保留
	got, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "serialhub") {
		t.Fatalf("真实目标未删除 serialhub: %s", got)
	}
	if !strings.Contains(string(got), "keep") {
		t.Fatalf("真实目标 keep 被误删: %s", got)
	}
}

// TestWriteFileAtomic_冲突检测 覆盖乐观冲突检测的完整链路：临时文件就绪后、
// rename 前重读比较，外部已修改则中止、原文件保留、无临时残留。
func TestWriteFileAtomic_冲突检测(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.json")
	orig := []byte(`{"mcp":{"servers":{"serialhub":{},"other":{}}}}`)
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}

	// 模拟卸载读走快照后、提交前，客户端写入新配置（新增了其他服务）
	external := []byte(`{"mcp":{"servers":{"serialhub":{},"other":{},"new":{}}}}`)
	if err := os.WriteFile(path, external, 0o644); err != nil {
		t.Fatal(err)
	}

	err := writeFileAtomic(path, orig, []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "被其他程序修改") {
		t.Fatalf("应检测到外部修改并中止: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, external) {
		t.Fatalf("原文件应保留外部新内容: %s", got)
	}
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 1 {
		t.Fatalf("不应残留临时文件: %v", entries)
	}
}

// TestWriteFileAtomic_正常替换 覆盖正常路径：内容替换、权限保留、无临时残留。
func TestWriteFileAtomic_正常替换(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.json")
	orig := []byte(`{"a":1}`)
	os.WriteFile(path, orig, 0o600)

	next := []byte(`{"a":2}`)
	if err := writeFileAtomic(path, orig, next); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, next) {
		t.Fatalf("内容应替换为 %s: %s", next, got)
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("权限应保留 0600: %v", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 1 {
		t.Fatalf("不应残留临时文件: %v", entries)
	}
}

// TestUninstallJSON_悬空符号链接幂等跳过：链接指向的文件不存在时，
// 按条目不存在处理，不创建文件、不拆链接。
func TestUninstallJSON_悬空符号链接幂等跳过(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.json")
	if err := os.Symlink(filepath.Join(tmp, "gone-target.json"), path); err != nil {
		t.Skipf("当前环境不支持符号链接: %v", err)
	}
	removed, err := uninstallJSON(path, "mcp.servers", "serialhub")
	if err != nil || removed {
		t.Fatalf("悬空链接应幂等跳过: removed=%v err=%v", removed, err)
	}
	info, lerr := os.Lstat(path)
	if lerr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("符号链接本身应原样保留: %v", lerr)
	}
}
