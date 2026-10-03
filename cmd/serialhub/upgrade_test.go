package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dongly/serialhub/internal/i18n"
	"github.com/dongly/serialhub/internal/testutil"
)

func TestTrimVPrefix(t *testing.T) {
	for in, want := range map[string]string{"v0.5.2": "0.5.2", "0.5.2": "0.5.2", " v1.0 ": "1.0"} {
		if got := trimVPrefix(in); got != want {
			t.Errorf("trimVPrefix(%q)=%q want %q", in, got, want)
		}
	}
}

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.5.1", "0.5.2", -1},
		{"0.5.2", "0.5.1", 1},
		{"0.5.1", "0.5.1", 0},
		{"v0.6.0", "0.5.9", 1},
		{"0.10.0", "0.9.9", 1},
		{"1.0", "1.0.0", 0},
		{"0.5", "0.5.1", -1},
		{"0.5.1-beta", "0.5.1", -1}, // 非数值段字符串比较
	}
	for _, tc := range cases {
		if got := compareVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersion(%q,%q)=%d want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestVerifySHA256(t *testing.T) {
	data := []byte("hello")
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:]) + "  serialhub.tar.gz\n"

	if err := verifySHA256(data, []byte(good)); err != nil {
		t.Fatalf("正确摘要不应报错: %v", err)
	}
	if err := verifySHA256([]byte("tampered"), []byte(good)); err == nil {
		t.Fatal("篡改数据应报错")
	}
	if err := verifySHA256(data, []byte("")); err == nil {
		t.Fatal("空校验文件应报错")
	}
	if err := verifySHA256(data, []byte(strings.ToUpper(good))); err != nil {
		t.Fatalf("大写摘要应接受: %v", err)
	}
}

// makeTarGz 构造发布包形态的 tar.gz（顶层目录 + 二进制文件）。
func makeTarGz(t *testing.T, entries []struct{ name, content string }) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name: e.name, Mode: 0o755, Size: int64(len(e.content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

// makeZip 构造 Windows 发布包形态的 zip。
func makeZip(t *testing.T, entries []struct{ name, content string }) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()
	return buf.Bytes()
}

func TestExtractBinary(t *testing.T) {
	t.Run("tar.gz按文件名解出", func(t *testing.T) {
		data := makeTarGz(t, []struct{ name, content string }{
			{"serialhub-0.6.0-linux-amd64/README.md", "readme"},
			{"serialhub-0.6.0-linux-amd64/serialhub", "NEW-BIN"},
		})
		got, err := extractBinary(data, "linux")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "NEW-BIN" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("zip按文件名解出", func(t *testing.T) {
		data := makeZip(t, []struct{ name, content string }{
			{"serialhub-0.6.0-windows-amd64/serialhub.exe", "WIN-BIN"},
			{"serialhub-0.6.0-windows-amd64/config.toml", "cfg"},
		})
		got, err := extractBinary(data, "windows")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "WIN-BIN" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("zip目录与符号链接条目被跳过只取普通文件", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		// 目录条目：basename 恰为 serialhub.exe 但不是普通文件
		fh := &zip.FileHeader{Name: "serialhub-0.6.0-windows-amd64/serialhub.exe/", Method: zip.Store}
		fh.SetMode(os.ModeDir | 0o755)
		if _, err := zw.CreateHeader(fh); err != nil {
			t.Fatal(err)
		}
		// 符号链接条目
		fh = &zip.FileHeader{Name: "evil/serialhub.exe", Method: zip.Store}
		fh.SetMode(os.ModeSymlink | 0o777)
		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(`C:\Windows\system32\calc.exe`))
		// 真正的二进制
		w, err = zw.Create("serialhub-0.6.0-windows-amd64/serialhub.exe")
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("REAL-BIN"))
		zw.Close()

		got, err := extractBinary(buf.Bytes(), "windows")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "REAL-BIN" {
			t.Fatalf("应取普通文件，got %q", got)
		}
	})

	t.Run("zip多个同名候选报错", func(t *testing.T) {
		data := makeZip(t, []struct{ name, content string }{
			{"dirA/serialhub.exe", "A"},
			{"dirB/serialhub.exe", "B"},
		})
		if _, err := extractBinary(data, "windows"); err == nil || !strings.Contains(err.Error(), fmt.Sprintf(i18n.UpgradeErrors.MultipleCount, 2, "serialhub.exe")) {
			t.Fatalf("应拒绝自动选择，err=%v", err)
		}
	})

	t.Run("zip空内容报错", func(t *testing.T) {
		data := makeZip(t, []struct{ name, content string }{
			{"serialhub-0.6.0-windows-amd64/serialhub.exe", ""},
		})
		if _, err := extractBinary(data, "windows"); err == nil || !strings.Contains(err.Error(), fmt.Sprintf(i18n.UpgradeErrors.EmptyEntry, "serialhub.exe")) {
			t.Fatalf("空内容应报错，err=%v", err)
		}
	})

	t.Run("解压内容超过上限报错", func(t *testing.T) {
		old := maxExtractedBytes
		maxExtractedBytes = 8
		defer func() { maxExtractedBytes = old }()
		if int64(old) <= 8 {
			t.Fatal("前提不成立：默认上限应大于测试注入值")
		}
		data := makeTarGz(t, []struct{ name, content string }{
			{"serialhub-0.6.0-linux-amd64/serialhub", "0123456789ABCDEF"}, // 16B > 8B
		})
		if _, err := extractBinary(data, "linux"); err == nil || !strings.Contains(err.Error(), fmt.Sprintf(i18n.UpgradeErrors.ExtractLimit, 8)) {
			t.Fatalf("超限应报错，err=%v", err)
		}
	})

	t.Run("tar累计解压流量超限报错", func(t *testing.T) {
		// 顺序解压格式下，目标条目前的大体积非目标条目也要消耗解压流量。
		// 预算取 600：大于 filler 的 512B tar header（能顺利读完进入数据段），
		// 小于 header+数据块（512+512），必然在跳过非目标数据的中途超限，
		// 确保覆盖「非目标条目消耗预算」而非仅在 header 阶段失败。
		old := maxArchiveStreamBytes
		maxArchiveStreamBytes = 600
		defer func() { maxArchiveStreamBytes = old }()
		if old <= 600 {
			t.Fatal("前提不成立：默认累计上限应大于测试注入值")
		}
		data := makeTarGz(t, []struct{ name, content string }{
			{"serialhub-0.6.0-linux-amd64/filler.bin", strings.Repeat("F", 64)}, // 先耗尽预算
			{"serialhub-0.6.0-linux-amd64/serialhub", "REAL-BIN"},
		})
		if _, err := extractBinary(data, "linux"); err == nil {
			t.Fatal("累计解压流量超限应报错")
		}
	})

	t.Run("未找到时报错", func(t *testing.T) {
		data := makeTarGz(t, []struct{ name, content string }{
			{"serialhub-0.6.0-linux-amd64/README.md", "readme"},
		})
		if _, err := extractBinary(data, "linux"); err == nil {
			t.Fatal("期望报错")
		}
	})

	t.Run("符号链接条目被跳过只取普通文件", func(t *testing.T) {
		// 打包异常场景：先出现指向别处的 serialhub 符号链接，真二进制在后面。
		// 旧实现会解出链接条目的空内容；新实现只接受普通文件。
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		link := &tar.Header{Name: "evil/serialhub", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777}
		if err := tw.WriteHeader(link); err != nil {
			t.Fatal(err)
		}
		reg := &tar.Header{Name: "serialhub-0.6.0-linux-amd64/serialhub", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len("REAL-BIN"))}
		if err := tw.WriteHeader(reg); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte("REAL-BIN"))
		tw.Close()
		gw.Close()

		got, err := extractBinary(buf.Bytes(), "linux")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "REAL-BIN" {
			t.Fatalf("应解出普通文件内容，got %q", got)
		}
	})

	t.Run("多个同名候选报错", func(t *testing.T) {
		data := makeTarGz(t, []struct{ name, content string }{
			{"dirA/serialhub", "BIN-A"},
			{"dirB/serialhub", "BIN-B"},
		})
		if _, err := extractBinary(data, "linux"); err == nil {
			t.Fatal("多个候选应拒绝自动选择")
		}
	})

	t.Run("空内容报错", func(t *testing.T) {
		data := makeTarGz(t, []struct{ name, content string }{
			{"serialhub-0.6.0-linux-amd64/serialhub", ""},
		})
		if _, err := extractBinary(data, "linux"); err == nil {
			t.Fatal("空内容应拒绝")
		}
	})
}

func TestPickAssets(t *testing.T) {
	mk := func(name string) ghAsset { return ghAsset{Name: name, BrowserDownloadURL: "http://x/" + name} }
	rel := &ghRelease{TagName: "v0.6.0", Assets: []ghAsset{
		mk("serialhub-0.6.0-linux-amd64.tar.gz"),
		mk("serialhub-0.6.0-linux-amd64.tar.gz.sha256"),
		mk("serialhub-0.6.0-windows-amd64.zip"),
	}}

	t.Run("匹配平台并带校验文件", func(t *testing.T) {
		u := &upgrader{goos: "linux", goarch: "amd64"}
		asset, sumAsset, err := u.pickAssets(rel, "0.6.0")
		if err != nil {
			t.Fatal(err)
		}
		if asset == nil || asset.Name != "serialhub-0.6.0-linux-amd64.tar.gz" {
			t.Fatalf("asset=%+v", asset)
		}
		if sumAsset == nil {
			t.Fatal("应找到 sha256 资产")
		}
	})

	t.Run("平台缺失时报错", func(t *testing.T) {
		u := &upgrader{goos: "linux", goarch: "arm64"}
		if _, _, err := u.pickAssets(rel, "0.6.0"); err == nil {
			t.Fatal("期望报错")
		}
	})

	t.Run("缺失sha256校验文件时报错", func(t *testing.T) {
		// 安全基线：release 未提供 .sha256 时拒绝升级，而不是跳过校验
		noSha := &ghRelease{TagName: "v0.6.0", Assets: []ghAsset{
			mk("serialhub-0.6.0-linux-amd64.tar.gz"),
			mk("serialhub-0.6.0-windows-amd64.zip"),
		}}
		u := &upgrader{goos: "linux", goarch: "amd64"}
		if _, _, err := u.pickAssets(noSha, "0.6.0"); err == nil {
			t.Fatal("缺 sha256 应拒绝升级")
		}
	})
}

// newUpgradeTestServer 模拟 GitHub API 与资产下载。
func newUpgradeTestServer(t *testing.T, tagName string, tarGz []byte) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(tarGz)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/x/y/releases/latest":
			name := fmt.Sprintf("serialhub-%s-linux-amd64.tar.gz", strings.TrimPrefix(tagName, "v"))
			json.NewEncoder(w).Encode(ghRelease{TagName: tagName, Assets: []ghAsset{
				{Name: name, BrowserDownloadURL: server.URL + "/dl/" + name},
				{Name: name + ".sha256", BrowserDownloadURL: server.URL + "/dl/" + name + ".sha256"},
			}})
		case strings.HasPrefix(r.URL.Path, "/dl/") && strings.HasSuffix(r.URL.Path, ".sha256"):
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  x\n"))
		case strings.HasPrefix(r.URL.Path, "/dl/"):
			w.Write(tarGz)
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}

func TestUpgraderRun_FullUpgrade(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("该用例走 linux tar.gz 分支")
	}
	tarGz := makeTarGz(t, []struct{ name, content string }{
		{"serialhub-9.9.9-linux-amd64/serialhub", "NEW-BIN-9.9.9"},
	})
	server := newUpgradeTestServer(t, "v9.9.9", tarGz)
	defer server.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "serialhub")
	if err := os.WriteFile(exe, []byte("OLD-BIN"), 0o755); err != nil {
		t.Fatal(err)
	}

	u := &upgrader{
		apiBase: server.URL, repo: "x/y",
		goos: "linux", goarch: "amd64",
		exePath: exe, nowVer: "0.5.1",
		client: server.Client(),
	}
	var out strings.Builder
	if err := u.run(&out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NEW-BIN-9.9.9" {
		t.Fatalf("二进制未替换: %q", got)
	}
	if !strings.Contains(out.String(), i18n.CLI.ChecksumOK) {
		t.Fatalf("输出应含校验通过: %s", out.String())
	}
	// 临时文件不残留
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".serialhub-new-") {
			t.Fatalf("临时文件残留: %s", e.Name())
		}
	}
}

func TestUpgraderRun_AlreadyLatest(t *testing.T) {
	tarGz := makeTarGz(t, []struct{ name, content string }{
		{"serialhub-0.5.1-linux-amd64/serialhub", "SAME"},
	})
	server := newUpgradeTestServer(t, "v0.5.1", tarGz)
	defer server.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "serialhub")
	os.WriteFile(exe, []byte("OLD-BIN"), 0o755)

	u := &upgrader{
		apiBase: server.URL, repo: "x/y",
		goos: "linux", goarch: "amd64",
		exePath: exe, nowVer: "0.5.1",
		client: server.Client(),
	}
	var out strings.Builder
	if err := u.run(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), testutil.I18nPrefix(i18n.CLI.AlreadyLatest)) {
		t.Fatalf("输出: %s", out.String())
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "OLD-BIN" {
		t.Fatalf("最新版不应替换: %q", got)
	}
}

func TestUpgraderRun_VerifyFailureKeepsBinary(t *testing.T) {
	tarGz := makeTarGz(t, []struct{ name, content string }{
		{"serialhub-9.9.9-linux-amd64/serialhub", "EVIL"},
	})
	// 服务端返回错误的 sha256（资产 URL 用请求 Host 构造绝对地址）
	sum := sha256.Sum256([]byte("not-the-asset"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		switch {
		case r.URL.Path == "/repos/x/y/releases/latest":
			json.NewEncoder(w).Encode(ghRelease{TagName: "v9.9.9", Assets: []ghAsset{
				{Name: "serialhub-9.9.9-linux-amd64.tar.gz", BrowserDownloadURL: base + "/dl/a.tar.gz"},
				{Name: "serialhub-9.9.9-linux-amd64.tar.gz.sha256", BrowserDownloadURL: base + "/dl/a.tar.gz.sha256"},
			}})
		case r.URL.Path == "/dl/a.tar.gz":
			w.Write(tarGz)
		case r.URL.Path == "/dl/a.tar.gz.sha256":
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  a.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "serialhub")
	os.WriteFile(exe, []byte("OLD-BIN"), 0o755)

	u := &upgrader{
		apiBase: server.URL, repo: "x/y",
		goos: "linux", goarch: "amd64",
		exePath: exe, nowVer: "0.5.1",
		client: server.Client(),
	}
	err := u.run(io.Discard)
	if err == nil || !strings.Contains(err.Error(), testutil.I18nPrefix(i18n.CLI.VerifyFailed)) {
		t.Fatalf("期望校验失败错误, got %v", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "OLD-BIN" {
		t.Fatalf("校验失败不应替换: %q", got)
	}
}

func TestUpgraderRun_APIError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	u := &upgrader{
		apiBase: server.URL, repo: "x/y",
		goos: "linux", goarch: "amd64",
		exePath: "/nonexistent/serialhub", nowVer: "0.5.1",
		client: server.Client(),
	}
	if err := u.run(io.Discard); err == nil {
		t.Fatal("期望 API 错误")
	}
}

// TestCleanupLegacyLaunchers 验证升级成功后清理 exe 同目录的废弃旧启动
// 脚本：两个旧名被删且输出提示，无关文件保留，目录内无旧名残留。
func TestCleanupLegacyLaunchers(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"serialhub.ps1", "serialhub.bat", "sr.ps1", "sr.bat", "serialhub.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	u := &upgrader{exePath: filepath.Join(dir, "serialhub.exe")}
	u.cleanupLegacyLaunchers(&buf)

	for _, name := range []string{"serialhub.ps1", "serialhub.bat"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s 应被清理", name)
		}
	}
	for _, name := range []string{"sr.ps1", "sr.bat", "serialhub.exe"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s 不应被误删: %v", name, err)
		}
	}
	out := buf.String()
	for _, want := range []string{"serialhub.ps1", "serialhub.bat"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出应包含清理提示 %s: %q", want, out)
		}
	}
}
