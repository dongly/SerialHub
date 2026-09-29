package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dongly/serialhub/internal/instance"
	"github.com/dongly/serialhub/internal/testutil"
	"github.com/dongly/serialhub/pkg/config"
	"github.com/dongly/serialhub/pkg/mcp"
)

// syncBuffer 是并发安全的输出缓冲：exec 的拷贝 goroutine 写入的同时，
// 测试可能在子进程仍存活时读取（失败诊断），加锁避免 data race。
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestStdioTakeoverChild 是端到端接管测试的子进程入口：由父进程经
// exec(测试二进制, -test.run=^TestStdioTakeoverChild$) 拉起，env
// SERIALHUB_TAKEOVER_CHILD=1 时运行真实 runStdio——有主则透明代理
// （主死后原地升级为主），无主则自成主实例。正常 `go test` 直接
// 跑到本函数时 env 不符，立即返回（同 instance_test 的子进程先例）。
func TestStdioTakeoverChild(t *testing.T) {
	foreground := os.Getenv("SERIALHUB_FOREGROUND_CHILD") == "1"
	if !foreground && os.Getenv("SERIALHUB_TAKEOVER_CHILD") != "1" {
		return
	}
	port, err := strconv.Atoi(os.Getenv("SERIALHUB_TAKEOVER_PORT"))
	if err != nil || port <= 0 {
		fmt.Fprintln(os.Stderr, "[SerialHub] [takeover-child] 缺少有效 SERIALHUB_TAKEOVER_PORT")
		os.Exit(2)
	}
	host = "127.0.0.1"
	mcpPort = port
	configPath = "" // stdio 主模式不落盘（persistConfig 对空路径 no-op）
	if foreground {
		// 前台模式（runMaster 路径）：锁被占 → 转透明代理；
		// 主实例死亡后原地接管为主（父测试断言同 PID、同端口接管）。
		if err := runMaster(config.GetDefault()); err != nil {
			fmt.Fprintf(os.Stderr, "[SerialHub] [foreground-child] 退出: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if err := runStdio(config.GetDefault()); err != nil {
		fmt.Fprintf(os.Stderr, "[SerialHub] [takeover-child] 退出: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// TestProxyToMaster_RetriesTakeoverWhenMasterNotReady：接管重试期间发现的
// 新主若在 HTTP 就绪前再次死亡，必须按 masterLost 继续有界重试，而不是让
// 代理直接退出——此时 stdio 连接仍有效，必须原样保留给下一轮。
func TestProxyToMaster_RetriesTakeoverWhenMasterNotReady(t *testing.T) {
	orig := waitForMasterReady
	waitForMasterReady = func(instance.LockInfo, time.Duration) bool { return false }
	t.Cleanup(func() { waitForMasterReady = orig })

	info := instance.LockInfo{Host: "127.0.0.1", Port: 1}

	// 持有 stdio 连接的接管重试：按 masterLost 返回并保留 handoff
	handoff := &mcp.StdioHandoff{}
	reason, got, err := proxyToMaster(info, handoff)
	if reason != mcp.ProxyMasterLost {
		t.Fatalf("reason = %q, 期望 masterLost（应继续有界重试）", reason)
	}
	if err == nil {
		t.Fatal("期望返回主未就绪错误")
	}
	if got != handoff {
		t.Fatal("必须原样保留 handoff 供下一轮重试")
	}

	// 初始启动（无 stdio 连接）：同样按 masterLost 返回，进入有界重试；
	// 首次无连接时 handoff 仍为 nil，升级时才创建 stdin reader。
	reason, got, err = proxyToMaster(info, nil)
	if reason != mcp.ProxyMasterLost {
		t.Fatalf("reason = %q, 期望 masterLost（首次等待失败也应重试）", reason)
	}
	if err == nil {
		t.Fatal("期望返回主未就绪错误")
	}
	if got != nil {
		t.Fatal("无 handoff 时不得凭空产生连接")
	}
}

// TestStdioTakeover_ProxyPromotesAfterMasterExit：完整链路——主实例退出后，
// stdio 代理经 /health 探活判定失联，在同一进程内原地升级：竞得单
// 实例锁成为新主并复用原 stdio 连接，原端口恢复服务。
//
// 本测试用真实 go-sdk MCP 客户端经代理 stdio 通信：先 initialize 并
// tools/list 成功，再杀主等接管，然后在**同一客户端会话**上再次
// tools/list——客户端不会重新 initialize，因此这一调用同时验证了
// 「原地升级」与「握手状态恢复」两件事。
func TestStdioTakeover_ProxyPromotesAfterMasterExit(t *testing.T) {
	// 跨平台：进程终止统一用 Process.Kill（Windows=TerminateProcess、
	// POSIX=SIGKILL），端口与 OS 文件锁随进程消亡释放，无需信号编排。

	// 选两个空闲端口：主实例用 masterPort，stdio 代理用 proxyPort。
	// 二者必须不同——接管后新主应继承死主的 masterPort 恢复服务，
	// 而不是用代理自己启动时的 proxyPort（Close 后到子进程监听之间
	// 存在小竞态，测试环境可接受）。
	freePort := func() int {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	masterPort := freePort()
	proxyPort := freePort()

	// 隔离单实例锁与日志目录：POSIX 走 XDG_CONFIG_HOME，
	// Windows 锁在 os.UserCacheDir()=%LOCALAPPDATA%，两个都指到临时目录。
	tmp := t.TempDir()
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", masterPort)
	childEnv := func(port int) []string {
		return append(os.Environ(),
			"XDG_CONFIG_HOME="+tmp,
			"LOCALAPPDATA="+tmp,
			"SERIALHUB_TAKEOVER_CHILD=1",
			fmt.Sprintf("SERIALHUB_TAKEOVER_PORT=%d", port),
		)
	}
	masterEnv := childEnv(masterPort)
	proxyEnv := childEnv(proxyPort)

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

	// 1) 主实例起来：stdin 由父进程持有管道保持打开（模拟 MCP 客户端连接
	//    存续，防 stdio reader EOF 提前退出），stdout 无人消费。
	var stderrA, stderrB syncBuffer
	masterCmd := exec.Command(os.Args[0], "-test.run=^TestStdioTakeoverChild$")
	masterCmd.Env = masterEnv
	masterCmd.Stderr = &stderrA
	if _, err := masterCmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := masterCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer masterCmd.Process.Kill()
	if !waitHealth(10 * time.Second) {
		t.Fatalf("主实例未就绪:\n%s", stderrA.String())
	}

	// 2) stdio 代理（发现活主，进入透明转发）：stdin/stdout 均为管道，
	//    供真实 MCP 客户端经 stdio 与本进程通信。
	proxyCmd := exec.Command(os.Args[0], "-test.run=^TestStdioTakeoverChild$")
	proxyCmd.Env = proxyEnv
	proxyCmd.Stderr = &stderrB
	proxyIn, err := proxyCmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	proxyOut, err := proxyCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := proxyCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer proxyCmd.Process.Kill()
	time.Sleep(1500 * time.Millisecond) // 等代理稳定进入转发状态

	// 3) 真实 MCP 客户端经代理 stdio 建立会话（initialize 由 SDK 完成，
	//    代理把握手透明转发给主实例）
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "takeover-e2e", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.IOTransport{
		Reader: proxyOut,
		Writer: testutil.NopWriteCloser{Writer: proxyIn},
	}, nil)
	if err != nil {
		t.Fatalf("经代理初始化 MCP 会话失败: %v\n代理输出:\n%s", err, stderrB.String())
	}

	// 代理转发时 tools/list 必须成功（证明透明转发链路可用）
	before, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("接管前 tools/list 失败: %v\n代理输出:\n%s", err, stderrB.String())
	}
	if len(before.Tools) == 0 {
		t.Fatal("接管前 tools/list 应返回已注册工具")
	}

	// 4) 主实例退出（强杀：端口与 OS 文件锁随进程消亡释放，跨平台）
	if err := masterCmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = masterCmd.Wait()

	// 杀主后端口必须立即不可用，确保后面的「恢复」确实是代理接管的结果
	if waitHealth(300 * time.Millisecond) {
		t.Fatal("主实例被杀后端口仍在响应，接管验证不成立")
	}

	// 5) 代理探活失联 → 原地升级为主（同进程），原端口恢复服务
	//   （真实探活节奏 2s×2 + 退避 0.5s + 主服务启动，~10s 内应完成）
	if !waitHealth(25 * time.Second) {
		t.Fatalf("主实例退出后代理未接管, 原端口未恢复:\n代理输出:\n%s", stderrB.String())
	}

	// 6) 接管者必须是代理进程本身（原地升级：pid 不变），且端口一致
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
	// 端口必须等于死主的 masterPort：接管继承死主端口恢复服务，
	// 而不是用代理自己启动时的 proxyPort。
	if lock.Port != masterPort {
		t.Fatalf("接管端口 = %d, 期望继承死主端口 %d", lock.Port, masterPort)
	}

	// 7) 同一客户端会话再次 tools/list：客户端不会重新 initialize，
	//    因此这要求接管后的新主恢复了代理捕获的握手状态。
	var after *mcpsdk.ListToolsResult
	deadline := time.Now().Add(10 * time.Second)
	for {
		after, err = session.ListTools(ctx, nil)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("接管后 tools/list 失败（会话状态未恢复？）: %v\n代理输出:\n%s", err, stderrB.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
	if len(after.Tools) == 0 {
		t.Fatal("接管后 tools/list 应返回已注册工具")
	}

	// 8) 清理：接管进程即代理进程本身，终止并回收即可
	_ = proxyCmd.Process.Kill()
	_ = proxyCmd.Wait()
}

// TestRunStdio_WaitReadyHasBoundedRetries 主实例持锁但 HTTP 永久不就绪（挂死）时，
// 代理必须有限次重探后放弃：这类「慢启动」不消耗接管预算，若无独立上限，
// 代理会每轮 WaitReady+退避无限自旋，MCP 客户端永久挂起。
func TestRunStdio_WaitReadyHasBoundedRetries(t *testing.T) {
	// 隔离单实例锁目录：POSIX 走 XDG_CONFIG_HOME，Windows 走 %LOCALAPPDATA%
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	// 本进程自持锁并写入 Port>0 的元数据：模拟「有活主但 HTTP 起不来」
	if _, err := instance.Acquire("127.0.0.1", 5599); err != nil {
		t.Fatalf("自持实例锁失败: %v", err)
	}
	t.Cleanup(instance.Release)

	origWait, origBackoff := waitForMasterReady, takeoverBackoff
	waitForMasterReady = func(instance.LockInfo, time.Duration) bool { return false }
	takeoverBackoff = time.Millisecond
	t.Cleanup(func() { waitForMasterReady, takeoverBackoff = origWait, origBackoff })

	done := make(chan error, 1)
	go func() { done <- runStdio(config.GetDefault()) }()
	select {
	case err := <-done:
		if !errors.Is(err, errMasterMetadataUnavailable) {
			t.Fatalf("应返回「主实例元数据不可用」, 实际 %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("慢启动等待未设上限：代理在持锁不就绪的主实例前无界自旋")
	}
}

// TestForegroundProxy_PromotesAfterMasterExit：前台模式（runMaster）下第二
// 实例发现活主后转透明代理；主实例死亡时，前台代理经 /health 探活判定失联
// 后在**同一进程内原地升级为主实例**（继承死主端口），原端口恢复服务——
// 与 --stdio 模式的接管语义一致（区别：前台代理的 stdio 是终端，无交接物）。
func TestForegroundProxy_PromotesAfterMasterExit(t *testing.T) {
	tmp := t.TempDir()
	freePort := func() int {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	masterPort := freePort()
	proxyPort := freePort() // 前台子进程自己的 mcpPort（仅 Acquire 用，代理跟随锁元数据）
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", masterPort)

	// 子进程 env：XDG/LOCALAPPDATA 隔离锁与日志；端口经 SERIALHUB_TAKEOVER_PORT
	// 传入（master 子进程＝监听端口；foreground 子进程＝Acquire 用的 mcpPort）。
	baseEnv := func(port int) []string {
		return append(os.Environ(),
			"XDG_CONFIG_HOME="+tmp,
			"LOCALAPPDATA="+tmp,
			"SERIALHUB_TAKEOVER_PORT="+strconv.Itoa(port),
		)
	}
	masterEnv := append(baseEnv(masterPort), "SERIALHUB_TAKEOVER_CHILD=1")
	proxyEnv := append(baseEnv(proxyPort), "SERIALHUB_FOREGROUND_CHILD=1")

	var stderrMaster, stderrProxy syncBuffer
	newChild := func(env []string, stderr *syncBuffer) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestStdioTakeoverChild$")
		cmd.Env = env
		cmd.Stderr = stderr
		cmd.StdinPipe() // 持有写端：子进程 stdin 不 EOF（避免 StdioClosed 抢先）
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

	// 1) 主实例子进程（runStdio → 自成主，监听 masterPort）
	masterCmd := newChild(masterEnv, &stderrMaster)
	if err := masterCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer masterCmd.Process.Kill()
	if !waitHealth(10 * time.Second) {
		t.Fatalf("主实例未就绪:\n%s", stderrMaster.String())
	}

	// 2) 前台子进程（runMaster → 锁被占 → 透明代理）
	proxyCmd := newChild(proxyEnv, &stderrProxy)
	if err := proxyCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer proxyCmd.Process.Kill()
	time.Sleep(1500 * time.Millisecond) // 等其进入转发状态

	// 3) 杀主实例：前台代理应经探活 2×2s 判定失联并原地接管成主，
	//    原端口恢复服务（真实节奏 2s×2 + 退避 0.5s + 启动，~10s 内应完成）
	if err := masterCmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = masterCmd.Wait()
	if !waitHealth(20 * time.Second) {
		t.Fatalf("主实例死亡后前台代理未接管, 原端口未恢复:\n%s", stderrProxy.String())
	}

	// 4) 接管者必须是前台代理进程本身（同 pid 原地升级），且端口继承死主
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
		t.Fatalf("接管后主 pid = %d, 期望前台代理进程 %d（原地升级应同 pid）", lock.Pid, proxyCmd.Process.Pid)
	}
	if lock.Port != masterPort {
		t.Fatalf("接管端口 = %d, 期望继承死主端口 %d", lock.Port, masterPort)
	}

	// 5) 清理：接管进程即前台代理本身，终止并回收即可
	_ = proxyCmd.Process.Kill()
	_ = proxyCmd.Wait()
}
