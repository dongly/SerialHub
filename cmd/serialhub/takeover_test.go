package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dongly/serialhub/pkg/config"
)

// TestStdioTakeoverChild 是端到端接管测试的子进程入口：由父进程经
// exec(测试二进制, -test.run=^TestStdioTakeoverChild$) 拉起，env
// SERIALHUB_TAKEOVER_CHILD=1 时运行真实 runStdio——有主则透明代理
// （主死后原地升级为主），无主则自成主实例。正常 `go test` 直接
// 跑到本函数时 env 不符，立即返回（同 instance_test 的子进程先例）。
func TestStdioTakeoverChild(t *testing.T) {
	if os.Getenv("SERIALHUB_TAKEOVER_CHILD") != "1" {
		return
	}
	port, err := strconv.Atoi(os.Getenv("SERIALHUB_TAKEOVER_PORT"))
	if err != nil || port <= 0 {
		fmt.Fprintln(os.Stderr, "[takeover-child] 缺少有效 SERIALHUB_TAKEOVER_PORT")
		os.Exit(2)
	}
	host = "127.0.0.1"
	mcpPort = port
	configPath = "" // stdio 主模式不落盘（persistConfig 对空路径 no-op）
	if err := runStdio(config.GetDefault()); err != nil {
		fmt.Fprintf(os.Stderr, "[takeover-child] 退出: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// TestStdioTakeover_ProxyPromotesAfterMasterExit：完整链路——主实例退出后，
// stdio 代理经 /health 探活判定失联，在同一进程内原地升级：竞得单
// 实例锁成为新主并复用原 stdio 连接，原端口恢复服务。
func TestStdioTakeover_ProxyPromotesAfterMasterExit(t *testing.T) {
	// 跨平台：进程终止统一用 Process.Kill（Windows=TerminateProcess、
	// POSIX=SIGKILL），端口与 OS 文件锁随进程消亡释放，无需信号编排。

	// 选一个空闲端口（Close 后到子进程监听之间存在小竞态，测试环境可接受）
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	// 隔离单实例锁与日志目录：POSIX 走 XDG_CONFIG_HOME，
	// Windows 锁在 os.UserCacheDir()=%LOCALAPPDATA%，两个都指到临时目录。
	tmp := t.TempDir()
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	env := append(os.Environ(),
		"XDG_CONFIG_HOME="+tmp,
		"LOCALAPPDATA="+tmp,
		"SERIALHUB_TAKEOVER_CHILD=1",
		fmt.Sprintf("SERIALHUB_TAKEOVER_PORT=%d", port),
	)

	// 子进程的 stdin 由父进程持有管道保持打开（模拟 MCP 客户端连接存续，
	// 防 stdio reader EOF 提前退出）。写端由 exec.Cmd 内部持有，Wait 前不关闭；
	// stdout 无人消费，导向 io.Discard。
	newChild := func(stderr *bytes.Buffer) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestStdioTakeoverChild$")
		cmd.Env = env
		cmd.Stdout = nil
		cmd.Stderr = stderr
		if _, err := cmd.StdinPipe(); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	waitHealth := func(timeout time.Duration) bool {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			resp, err := http.Get(healthURL)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return true
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		return false
	}

	// 1) 主实例起来
	var stderrA, stderrB bytes.Buffer
	masterCmd := newChild(&stderrA)
	if err := masterCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer masterCmd.Process.Kill()
	if !waitHealth(10 * time.Second) {
		t.Fatalf("主实例未就绪:\n%s", stderrA.String())
	}

	// 2) stdio 代理（发现活主，进入透明转发）
	proxyCmd := newChild(&stderrB)
	if err := proxyCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer proxyCmd.Process.Kill()
	time.Sleep(1500 * time.Millisecond) // 等代理稳定进入转发状态

	// 3) 主实例退出（强杀：端口与 OS 文件锁随进程消亡释放，跨平台）
	if err := masterCmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = masterCmd.Wait()

	// 4) 代理探活失联 → 原地升级为主（同进程），原端口恢复服务
	//   （真实探活节奏 2s×2 + 退避 0.5s + 主服务启动，~10s 内应完成）
	if !waitHealth(20 * time.Second) {
		t.Fatalf("主实例退出后代理未接管, 原端口未恢复:\n代理输出:\n%s", stderrB.String())
	}

	// 5) 接管者必须是代理进程本身（原地升级：pid 不变），且端口一致
	lockData, err := os.ReadFile(filepath.Join(tmp, "serialhub", "instance.lock"))
	if err != nil {
		t.Fatalf("读取接管后的实例锁失败: %v", err)
	}
	var lock struct {
		Pid  int `json:"pid"`
		Port int `json:"port"`
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatalf("解析实例锁失败: %v\n锁内容: %s", err, lockData)
	}
	if lock.Pid != proxyCmd.Process.Pid {
		t.Fatalf("接管后主 pid = %d, 期望代理进程 %d（原地升级应同 pid）", lock.Pid, proxyCmd.Process.Pid)
	}
	if lock.Port != port {
		t.Fatalf("接管端口 = %d, 期望 %d", lock.Port, port)
	}

	// 6) 清理：接管进程即代理进程本身，终止并回收即可
	_ = proxyCmd.Process.Kill()
	_ = proxyCmd.Wait()
}
