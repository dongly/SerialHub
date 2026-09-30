package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dongly/serialhub/internal/instance"
)

func TestSerialhubInstanceAt(t *testing.T) {
	health := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.Write([]byte(body))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
	}
	// want: local/remote/none
	check := func(t *testing.T, url, want string) {
		t.Helper()
		local, remote := serialhubInstanceAt(url)
		got := "none"
		if local {
			got = "local"
		} else if remote {
			got = "remote"
		}
		if got != want {
			t.Fatalf("探活分类不符：got=%s want=%s", got, want)
		}
	}

	t.Run("SerialHub的health响应识别为本机实例", func(t *testing.T) {
		srv := health(`{"status":"ok","service":"serialhub","role":"master"}`)
		defer srv.Close()
		check(t, srv.URL, "local")
	})

	t.Run("老版本实例无service字段回退role识别", func(t *testing.T) {
		srv := health(`{"status":"ok","role":"worker"}`)
		defer srv.Close()
		check(t, srv.URL, "local")
	})

	t.Run("同侧实例识别为本机", func(t *testing.T) {
		srv := health(fmt.Sprintf(`{"status":"ok","service":"serialhub","role":"master","side":%q}`, instance.LocalSide()))
		defer srv.Close()
		check(t, srv.URL, "local")
	})

	t.Run("对侧实例经端口转发不拦截本机卸载", func(t *testing.T) {
		// side 与本机必然不同（LocalSide 不会返回该值）
		srv := health(`{"status":"ok","service":"serialhub","role":"master","side":"opposite-side"}`)
		defer srv.Close()
		check(t, srv.URL, "remote")
	})

	t.Run("对侧worker角色同样不拦截", func(t *testing.T) {
		srv := health(`{"status":"ok","service":"serialhub","role":"worker","side":"opposite-side"}`)
		defer srv.Close()
		check(t, srv.URL, "remote")
	})

	t.Run("坏JSON响应视为无实例", func(t *testing.T) {
		srv := health(`{"status":"ok","service":"serialhub"`) // 截断的 JSON
		defer srv.Close()
		check(t, srv.URL, "none")
	})

	t.Run("探活超时视为无实例", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second) // 远超 serialhubInstanceAt 的 500ms 客户端超时
		}))
		defer srv.Close()
		check(t, srv.URL, "none")
	})

	t.Run("其他服务的200不误判", func(t *testing.T) {
		srv := health(`{"status":"ok"}`) // 无 role 字段：不是 SerialHub
		defer srv.Close()
		check(t, srv.URL, "none")
	})

	t.Run("其他服务冒用role不被service拒绝", func(t *testing.T) {
		srv := health(`{"status":"ok","service":"other-tool","role":"master"}`)
		defer srv.Close()
		check(t, srv.URL, "none")
	})

	t.Run("404视为无实例", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		check(t, srv.URL, "none")
	})

	t.Run("连接失败视为无实例", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close() // 立即关闭，端口不再服务
		check(t, srv.URL, "none")
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
	// 9 个 MCP 条目 + 配置目录 + 二进制（安装目录随包文件动作仅在
	// exe 同目录检出启动脚本时追加，测试目录无启动脚本故为 11）
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

func TestRemoveInstallDirExtrasIn(t *testing.T) {
	// 造一个发布包样式的安装目录：随包文件 + 升级残留 + 用户文件 + 假 exe
	dir := t.TempDir()
	mk := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"serialhub.ps1", "serialhub.bat", "VERSION",
		"README.md", "README.en.md", "QUICKSTART.md", "MCP.md", "LICENSE"} {
		mk(n, "x")
	}
	mk("serialhub.exe.old-1719000000000000000", "old-bin")
	mk("serialhub.exe.0.6.0-old", "legacy-old-bin")
	mk("serialhub.exe", "fake-exe")
	mk("mynote.txt", "用户自建文件")

	desc, _, err := removeInstallDirExtrasIn(dir)
	if err != nil {
		t.Fatalf("removeInstallDirExtrasIn: %v", err)
	}
	if !strings.Contains(desc, "10") {
		t.Errorf("应删除 10 个随包/残留文件，desc=%s", desc)
	}
	// 随包文件与升级残留全部删除
	for _, n := range []string{"serialhub.ps1", "serialhub.bat", "VERSION",
		"README.md", "README.en.md", "QUICKSTART.md", "MCP.md", "LICENSE",
		"serialhub.exe.old-1719000000000000000", "serialhub.exe.0.6.0-old"} {
		if ok, _ := pathExists(filepath.Join(dir, n)); ok {
			t.Errorf("%s 应被删除", n)
		}
	}
	// 用户文件与 exe 不受影响
	for _, n := range []string{"mynote.txt", "serialhub.exe"} {
		if ok, _ := pathExists(filepath.Join(dir, n)); !ok {
			t.Errorf("%s 不应被删除", n)
		}
	}
}

func TestRemoveInstallDirExtrasIn_NonInstallDir(t *testing.T) {
	// 无启动脚本 → 不是安装目录，什么都不删
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := removeInstallDirExtrasIn(dir)
	if err != nil {
		t.Fatalf("非安装目录应安全跳过: %v", err)
	}
	if ok, _ := pathExists(filepath.Join(dir, "README.md")); !ok {
		t.Error("非安装目录的文件不应被删除")
	}
}

// 部分删除失败必须中止：README.md 被占用（以非空目录模拟删除失败）时，
// 后续的升级残留与启动脚本（安装目录认定特征）不得被删，保证重跑可续清。
func TestRemoveInstallDirExtrasIn_AbortOnFailure(t *testing.T) {
	dir := t.TempDir()
	// README.md 造成不可删除：非空目录
	if err := os.MkdirAll(filepath.Join(dir, "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md", "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"serialhub.ps1", "serialhub.exe.old-0.6.0"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := removeInstallDirExtrasIn(dir)
	if err == nil {
		t.Fatal("README.md 删除失败时必须返回错误")
	}
	// 特征文件（启动脚本与升级残留）必须原样保留
	for _, name := range []string{"serialhub.ps1", "serialhub.exe.old-0.6.0"} {
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
			t.Errorf("%s 应保留在场供重跑续清: %v", name, statErr)
		}
	}
}

func TestWindowsDelayedRemoveScript(t *testing.T) {
	// 用例：中文、空格与弯引号（U+2019）混合路径——旧单引号字面量方案
	// 会被弯引号破坏，码元数组应对全部字符免疫
	exe := `D:\My Tools\don’t串口\serialhub.exe`
	dir := `D:\My Tools\don’t串口`
	got := windowsDelayedRemoveScript(exe, dir)

	// 路径以 UTF-16 码元列表嵌入，$e/$d 各一次，脚本内 [char[]] 重建
	if !strings.Contains(got, "$e = -join [char[]]("+psCharArray(exe)+"); ") {
		t.Errorf("exe 应以码元数组构造 $e: %q", got)
	}
	if !strings.Contains(got, "$d = -join [char[]]("+psCharArray(dir)+"); ") {
		t.Errorf("dir 应以码元数组构造 $d: %q", got)
	}
	// 前置等待后进入三轮循环；以文件是否在场判定（不依赖退出码）
	if !strings.Contains(got, "Start-Sleep -Seconds 2; foreach ($i in 1..3) {") {
		t.Errorf("缺少首轮等待+循环前缀: %q", got)
	}
	if !strings.Contains(got, "if (Test-Path -LiteralPath $e) { Remove-Item -Force -LiteralPath $e; ") {
		t.Errorf("循环体应为 -LiteralPath 变量检查+删除+间隔等待: %q", got)
	}
	// 尾部：目录存在且已空才删除
	if !strings.HasSuffix(got, "if ((Test-Path -LiteralPath $d) -and -not (Get-ChildItem -Force -LiteralPath $d)) { [void][System.IO.Directory]::Delete($d) }") {
		t.Errorf("应以目录空判定+删除结尾: %q", got)
	}
	// 注入面归零：路径原文（含弯引号/空格/中文）不得出现在脚本文本中
	for _, lit := range []string{exe, dir, `don’t`, "串口"} {
		if strings.Contains(got, lit) {
			t.Errorf("脚本文本不应包含路径原文 %q: %q", lit, got)
		}
	}
	// cmd 批处理残留回归检查：不得再出现 cmd 语法
	for _, frag := range []string{"cmd", "del /f", "if exist", "ping -n"} {
		if strings.Contains(got, frag) {
			t.Errorf("不应再包含 cmd 批处理语法 %q: %q", frag, got)
		}
	}
	// 单引号字面量残留回归检查：路径一律走变量，不再以引号字面量进入脚本
	if strings.Contains(got, "LiteralPath '") {
		t.Errorf("不应出现单引号路径字面量: %q", got)
	}
	// 括号配对
	open, close := strings.Count(got, "("), strings.Count(got, ")")
	if open != close {
		t.Errorf("括号不配对: open=%d close=%d", open, close)
	}
}

func TestWindowsDelayedRemoveScript_WildcardPath(t *testing.T) {
	// 方括号目录在 -Path 通配符语义下是字符类；码元数组以变量传递，
	// 配合 -LiteralPath 双保险，脚本文本不含路径原文
	exe := `D:\Tools[1]\serialhub\serialhub.exe`
	dir := `D:\Tools[1]\serialhub`
	got := windowsDelayedRemoveScript(exe, dir)
	if strings.Count(got, "LiteralPath $e") != 2 {
		t.Errorf("exe 应以 -LiteralPath 变量出现 2 次: %q", got)
	}
	if !strings.Contains(got, "Test-Path -LiteralPath $d") || !strings.Contains(got, "Get-ChildItem -Force -LiteralPath $d") {
		t.Errorf("目录判定应使用 -LiteralPath 变量: %q", got)
	}
	for _, lit := range []string{exe, dir, "Tools[1]"} {
		if strings.Contains(got, lit) {
			t.Errorf("脚本文本不应包含路径原文 %q: %q", lit, got)
		}
	}
	// 码元列表应完整覆盖方括号路径（代理对之外的常规编码由同源函数生成）
	if !strings.Contains(got, psCharArray(exe)) || !strings.Contains(got, psCharArray(dir)) {
		t.Errorf("应嵌入完整码元列表: %q", got)
	}
}

func TestPsCharArray(t *testing.T) {
	// 路径编码为逗号分隔 UTF-16 码元：空串、ASCII、中文（BMP 内单码元）、
	// 弯引号与增补平面字符（代理对拆两码元）四类代表
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"空串", "", ""},
		{"ASCII", "AB", "65,66"},
		{"中文BMP单码元", "串", "20018"},               // U+4E32
		{"弯引号", "’", "8217"},                     // U+2019
		{"代理对拆两码元", "\U00010437", "55297,56375"}, // U+10437 → D801 DC37
	}
	for _, c := range cases {
		if got := psCharArray(c.in); got != c.want {
			t.Errorf("%s: psCharArray(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestHasInstallLayout(t *testing.T) {
	// 认定安装目录：启动脚本或升级残留任一在场；仅文档不算（防误伤）
	cases := []struct {
		name  string
		files []string
		want  bool
	}{
		{"ps1 脚本", []string{"/x/serialhub.ps1", "/x/README.md"}, true},
		{"bat 脚本", []string{"/x/serialhub.bat"}, true},
		{"sh 脚本", []string{"/x/serialhub.sh"}, true},
		{"升级残留 exe.old", []string{"/x/serialhub.exe.old-0.6.0", "/x/serialhub.exe"}, true},
		{"升级残留历史名", []string{"/x/serialhub.exe.0.6.0-old"}, true},
		{"升级残留无扩展名", []string{"/x/serialhub.old-abc"}, true},
		{"仅文档不算", []string{"/x/README.md", "/x/LICENSE", "/x/VERSION"}, false},
		{"空列表", nil, false},
	}
	for _, c := range cases {
		if got := hasInstallLayout(c.files); got != c.want {
			t.Errorf("%s: hasInstallLayout=%v, 期望 %v", c.name, got, c.want)
		}
	}
}

func TestRemoveInstallDirExtrasIn_RetryAfterPartialFailure(t *testing.T) {
	// 续清场景：首跑删掉了启动脚本后部分失败，目录里剩升级残留+文档，
	// 重跑仍须认定为安装目录并清掉残留（升级残留参与认定）
	dir := t.TempDir()
	for _, f := range []string{"serialhub.exe.old-0.6.0", "README.md", "LICENSE"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	msg, _, err := removeInstallDirExtrasIn(dir)
	if err != nil {
		t.Fatalf("续清应成功: %v", err)
	}
	if !strings.Contains(msg, "3") {
		t.Errorf("应清理 3 个文件: %s", msg)
	}
	for _, f := range []string{"serialhub.exe.old-0.6.0", "README.md", "LICENSE"} {
		if ok, _ := pathExists(filepath.Join(dir, f)); ok {
			t.Errorf("%s 应被删除", f)
		}
	}
}
