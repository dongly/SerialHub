package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSerialhubRunningAt(t *testing.T) {
	t.Run("SerialHub的health响应识别为true", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.Write([]byte(`{"status":"ok","service":"serialhub","role":"master"}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		if !serialhubRunningAt(server.URL) {
			t.Fatal("SerialHub /health（含 service+role 字段）应返回 true")
		}
	})

	t.Run("老版本实例无service字段回退role识别", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.Write([]byte(`{"status":"ok","role":"worker"}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		if !serialhubRunningAt(server.URL) {
			t.Fatal("老版本实例（仅 role=worker）应回退识别为 true")
		}
	})

	t.Run("其他服务的200不误判", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"status":"ok"}`)) // 无 role 字段：不是 SerialHub
		}))
		defer server.Close()
		if serialhubRunningAt(server.URL) {
			t.Fatal("无 role 字段的 /health 不应误判为 SerialHub")
		}
	})

	t.Run("其他服务冒用role不被service拒绝", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"status":"ok","service":"other-tool","role":"master"}`))
		}))
		defer server.Close()
		if serialhubRunningAt(server.URL) {
			t.Fatal("service 不是 serialhub 时不应识别为 SerialHub")
		}
	})

	t.Run("404视为无实例", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()
		if serialhubRunningAt(server.URL) {
			t.Fatal("404 应返回 false")
		}
	})

	t.Run("连接失败视为无实例", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		server.Close() // 立即关闭，端口不再服务
		if serialhubRunningAt(server.URL) {
			t.Fatal("连接失败应返回 false")
		}
	})
}

func TestDetectCandidateTargetsImpl(t *testing.T) {
	writeCfg := func(t *testing.T, path, content string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hasTarget := func(ts []candidateTarget, host string, port int) bool {
		for _, x := range ts {
			if x.host == host && x.port == port {
				return true
			}
		}
		return false
	}

	t.Run("无配置时默认端口的两个回环地址", func(t *testing.T) {
		targets := detectCandidateTargetsImpl(false, "", t.TempDir(), 5050)
		if len(targets) != 2 || !hasTarget(targets, "127.0.0.1", 5050) || !hasTarget(targets, "::1", 5050) {
			t.Fatalf("targets=%v", targets)
		}
	})

	t.Run("Unix用户配置端口加入检测", func(t *testing.T) {
		xdg := t.TempDir()
		writeCfg(t, filepath.Join(xdg, "serialhub", "config.toml"), "[MCP]\nHTTPPort = 6060")
		targets := detectCandidateTargetsImpl(false, "", xdg, 5050)
		// 未写 Host 时 config.Load 填默认 127.0.0.1（仅 IPv4 回环），单地址探测
		if len(targets) != 3 || !hasTarget(targets, "127.0.0.1", 6060) {
			t.Fatalf("targets=%v", targets)
		}
	})

	t.Run("配置host通配时以双回环探测", func(t *testing.T) {
		xdg := t.TempDir()
		writeCfg(t, filepath.Join(xdg, "serialhub", "config.toml"), "Host = \"0.0.0.0\"\n[MCP]\nHTTPPort = 6060")
		targets := detectCandidateTargetsImpl(false, "", xdg, 5050)
		if len(targets) != 4 || !hasTarget(targets, "127.0.0.1", 6060) || !hasTarget(targets, "::1", 6060) {
			t.Fatalf("targets=%v", targets)
		}
	})

	t.Run("配置host为具体地址时按该地址探测", func(t *testing.T) {
		xdg := t.TempDir()
		writeCfg(t, filepath.Join(xdg, "serialhub", "config.toml"), "Host = \"192.168.1.5\"\n[MCP]\nHTTPPort = 6060")
		targets := detectCandidateTargetsImpl(false, "", xdg, 5050)
		if len(targets) != 3 || !hasTarget(targets, "192.168.1.5", 6060) {
			t.Fatalf("targets=%v", targets)
		}
	})

	t.Run("具体host配默认端口时仍加入该地址", func(t *testing.T) {
		// 回归：实例绑定非回环地址但用默认端口时，不能因端口相同而漏检
		xdg := t.TempDir()
		writeCfg(t, filepath.Join(xdg, "serialhub", "config.toml"), "Host = \"192.168.1.5\"\n[MCP]\nHTTPPort = 5050")
		targets := detectCandidateTargetsImpl(false, "", xdg, 5050)
		if !hasTarget(targets, "192.168.1.5", 5050) {
			t.Fatalf("targets=%v", targets)
		}
	})

	t.Run("配置host为IPv6通配时含回环v6", func(t *testing.T) {
		xdg := t.TempDir()
		writeCfg(t, filepath.Join(xdg, "serialhub", "config.toml"), "Host = \"::\"\n[MCP]\nHTTPPort = 6060")
		targets := detectCandidateTargetsImpl(false, "", xdg, 5050)
		if len(targets) != 4 || !hasTarget(targets, "::1", 6060) {
			t.Fatalf("targets=%v", targets)
		}
	})

	t.Run("Windows读exe同目录配置", func(t *testing.T) {
		exeDir := t.TempDir()
		writeCfg(t, filepath.Join(exeDir, "config.toml"), "[MCP]\nHTTPPort = 7070")
		targets := detectCandidateTargetsImpl(true, exeDir, t.TempDir(), 5050)
		// Host 默认 127.0.0.1，7070 单地址加入
		if len(targets) != 3 || !hasTarget(targets, "127.0.0.1", 7070) {
			t.Fatalf("targets=%v", targets)
		}
	})

	t.Run("Windows无exe目录时仅默认端口", func(t *testing.T) {
		targets := detectCandidateTargetsImpl(true, "", t.TempDir(), 5050)
		if len(targets) != 2 {
			t.Fatalf("targets=%v", targets)
		}
	})
}

// TestExecuteUninstallActions 覆盖执行链路：前置硬错误必须阻止二进制自删。
func TestExecuteUninstallActions(t *testing.T) {
	selfCalled := false
	actions := []uninstallAction{
		{desc: "正常项", probe: func() (bool, string) { return true, "存在" },
			run: func() (string, bool, error) { return "已完成", false, nil }},
		{desc: "失败项", probe: func() (bool, string) { return true, "存在" },
			run: func() (string, bool, error) { return "", false, errors.New("boom") }},
		{desc: "二进制", probe: func() (bool, string) { return true, "存在" }, selfBinary: true,
			run: func() (string, bool, error) { selfCalled = true; return "已删除", false, nil }},
	}
	failures, manual := executeUninstallActions(actions)
	if len(failures) != 1 || !strings.Contains(failures[0], "失败项") {
		t.Fatalf("failures=%v", failures)
	}
	if selfCalled {
		t.Fatal("存在失败项时不应执行二进制自删")
	}
	if manual {
		t.Fatal("无 manual 项")
	}

	// 无失败时自删正常执行
	selfCalled = false
	noFail := []uninstallAction{actions[0], actions[2]}
	failures, _ = executeUninstallActions(noFail)
	if len(failures) != 0 || !selfCalled {
		t.Fatalf("failures=%v selfCalled=%v", failures, selfCalled)
	}
}

// TestPathExists 覆盖存在性检查的三态：存在 / 不存在 / 检查失败（不吞错）。
func TestPathExists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	os.WriteFile(path, []byte("x"), 0o644)

	if ok, err := pathExists(path); err != nil || !ok {
		t.Fatalf("存在: ok=%v err=%v", ok, err)
	}
	if ok, err := pathExists(filepath.Join(tmp, "gone.toml")); err != nil || ok {
		t.Fatalf("不存在: ok=%v err=%v", ok, err)
	}

	t.Run("检查失败如实报错", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("依赖 Unix 目录权限模型；Windows ACL 行为需平台专用验证")
		}
		if os.Geteuid() == 0 {
			t.Skip("root 无视目录权限，无法触发 Stat 权限错误")
		}
		blocked := filepath.Join(tmp, "blocked")
		if err := os.MkdirAll(filepath.Join(blocked, "sub"), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.Chmod(blocked, 0o000); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		defer os.Chmod(blocked, 0o755)
		if _, err := pathExists(filepath.Join(blocked, "sub", "f.toml")); err == nil {
			t.Fatal("权限拒绝的 Stat 应返回错误而非当作不存在")
		}
	})
}

func TestRemoveUserState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix 分支；Windows 删 exe 同目录，人工验证")
	}

	t.Run("删除整个用户配置目录", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmp)
		dir := filepath.Join(tmp, "serialhub")
		os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
		os.WriteFile(filepath.Join(dir, "config.toml"), []byte("httpPort = 5050"), 0o644)

		desc, err := removeUserState()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(desc, "已删除") {
			t.Fatalf("desc=%s", desc)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("目录应被删除")
		}
	})

	t.Run("不存在时幂等跳过", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmp)
		desc, err := removeUserState()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(desc, "跳过") {
			t.Fatalf("desc=%s", desc)
		}
	})

	t.Run("XDG与HOME均不可用时跳过", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		desc, err := removeUserState()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(desc, "跳过") && !strings.Contains(desc, "无法定位") {
			t.Fatalf("desc=%s", desc)
		}
	})
}

func TestUserStateExists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix 分支")
	}
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	if userStateExists() {
		t.Fatal("空目录应返回 false")
	}
	os.MkdirAll(filepath.Join(tmp, "serialhub"), 0o755)
	if !userStateExists() {
		t.Fatal("存在 serialhub/ 应返回 true")
	}
}

func TestBuildUninstallActions(t *testing.T) {
	actions := buildUninstallActions()
	// 9 个 MCP 条目 + 配置目录 + 二进制
	if len(actions) != 11 {
		t.Fatalf("动作数量=%d want 11", len(actions))
	}
	var descs []string
	selfCount := 0
	for _, a := range actions {
		descs = append(descs, a.desc)
		if a.run == nil {
			t.Fatal("每个动作必须有 run")
		}
		if a.selfBinary {
			selfCount++
		}
	}
	if selfCount != 1 {
		t.Fatalf("应恰有 1 个二进制自删项，实际 %d", selfCount)
	}
	joined := strings.Join(descs, "\n")
	for _, want := range []string{"OpenCode", "Claude", "Cursor", "Windsurf", "VS Code", "Codex", "配置与日志目录", "二进制"} {
		if !strings.Contains(joined, want) {
			t.Errorf("清单缺少 %q: %s", want, joined)
		}
	}
}
