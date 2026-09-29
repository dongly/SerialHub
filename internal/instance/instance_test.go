package instance

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testLockPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "instance.lock")
	lockPathOverride = path
	t.Cleanup(func() { Release(); lockPathOverride = "" })
	return path
}

func TestAcquireReadRelease(t *testing.T) {
	path := testLockPath(t)
	info, err := Acquire("127.0.0.1", 5050)
	if err != nil || info.PID != os.Getpid() {
		t.Fatalf("Acquire: %+v %v", info, err)
	}
	if got := Read(); got == nil || got.Port != 5050 {
		t.Fatalf("持锁时 Read: %+v", got)
	}
	if old, err := Acquire("127.0.0.1", 6060); !errors.Is(err, ErrActive) || old.Port != 5050 {
		t.Fatalf("重复获取不得覆盖元数据: %+v %v", old, err)
	}
	Release()
	Release()
	if got := Read(); got != nil {
		t.Fatalf("释放后仍有持有者: %+v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("锁文件应常驻以免 inode 竞态: %v", err)
	}
}

func TestStaleMetadata(t *testing.T) {
	path := testLockPath(t)
	old := LockInfo{PID: 999999, Port: 1234}
	b, _ := json.Marshal(old)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Read(); got != nil {
		t.Fatalf("无持有者的元数据不算活主: %+v", got)
	}
	info, err := Acquire("127.0.0.1", 6060)
	if err != nil || info.Port != 6060 {
		t.Fatalf("重新取得锁失败: %+v %v", info, err)
	}
	Release()
}

// 子进程持有锁但没有监听 HTTP，主进程必须仍能发现且不能夺锁。
func TestCrossProcessStartupAndRecovery(t *testing.T) {
	path := testLockPath(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHolderProcess$")
	cmd.Env = append(os.Environ(), "SERIALHUB_LOCK_CHILD="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready := make([]byte, 6)
	if _, err := io.ReadFull(stdout, ready); err != nil || !strings.Contains(string(ready), "READY") {
		t.Fatalf("子进程未持锁: %q %v", ready, err)
	}
	if got := Read(); got == nil || got.Port != 6060 {
		t.Fatalf("无 HTTP 服务也应看到持锁进程: %+v", got)
	}
	if old, err := Acquire("127.0.0.1", 7070); !errors.Is(err, ErrActive) || old.Port != 6060 {
		t.Fatalf("跨进程互斥失败: %+v %v", old, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	cmd.Process = nil
	if got := Read(); got != nil {
		t.Fatalf("进程强退后内核应释放锁: %+v", got)
	}
	if _, err := Acquire("127.0.0.1", 7070); err != nil {
		t.Fatalf("强退后接管失败: %v", err)
	}
}

func TestLockHolderProcess(t *testing.T) {
	path := os.Getenv("SERIALHUB_LOCK_CHILD")
	if path == "" {
		return
	}
	lockPathOverride = path
	if _, err := Acquire("127.0.0.1", 6060); err != nil {
		t.Fatal(err)
	}
	os.Stdout.WriteString("READY\n")
	// 等待父进程 Kill。不要用 select{}：无 case 的 select 会触发 Go 死锁
	// 检测器 fatal 退出（Windows 测试进程无常驻信号 goroutine 兜底），
	// 子进程立即释放锁，父进程就会误判"无活主"。挂一个定时器保持运行时
	// 活跃：5 分钟足够父进程完成检查，也避免父进程异常退出后子进程长期占锁。
	select {
	case <-time.After(5 * time.Minute):
	}
}

// TestRaceChildProcess 是两个子进程并发抢锁的参与者：成功者持锁 2s 后再退出，
// 失败者立即报告 REFUSED。
func TestRaceChildProcess(t *testing.T) {
	path := os.Getenv("SERIALHUB_RACE_CHILD")
	if path == "" {
		return
	}
	lockPathOverride = path
	if _, err := Acquire("127.0.0.1", 6060); err != nil {
		if errors.Is(err, ErrActive) {
			os.Stdout.WriteString("REFUSED\n")
			return
		}
		t.Fatal(err)
	}
	os.Stdout.WriteString("ACQUIRED\n")
	time.Sleep(2 * time.Second)
}

// TestConcurrentAcquireOneWinner 两个子进程同时抢锁：恰好一个成功。
func TestConcurrentAcquireOneWinner(t *testing.T) {
	path := testLockPath(t)
	run := func() (string, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRaceChildProcess$")
		cmd.Env = append(os.Environ(), "SERIALHUB_RACE_CHILD="+path)
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	type res struct {
		out string
		err error
	}
	ch := make(chan res, 2)
	for i := 0; i < 2; i++ {
		go func() { out, err := run(); ch <- res{out, err} }()
	}
	verdict := func(out string) string {
		switch {
		case strings.Contains(out, "ACQUIRED"):
			return "ACQUIRED"
		case strings.Contains(out, "REFUSED"):
			return "REFUSED"
		default:
			return ""
		}
	}
	a, b := verdict((<-ch).out), verdict((<-ch).out)
	if (a != "ACQUIRED" && a != "REFUSED") || (b != "ACQUIRED" && b != "REFUSED") {
		t.Fatalf("未知子进程输出: %q %q", a, b)
	}
	if (a == "ACQUIRED") == (b == "ACQUIRED") {
		t.Fatalf("应恰好一个成功一个被拒: %q %q", a, b)
	}
}

func TestURL(t *testing.T) {
	for _, tc := range []struct{ host, want string }{
		{"", "http://127.0.0.1:5050"},
		{"0.0.0.0", "http://127.0.0.1:5050"},
		{"::1", "http://[::1]:5050"},
	} {
		if got := (LockInfo{Host: tc.host, Port: 5050}).URL(); got != tc.want {
			t.Errorf("%s: %s != %s", tc.host, got, tc.want)
		}
	}
}

func TestWaitReady(t *testing.T) {
	old := healthProbeTimeout
	healthProbeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { healthProbeTimeout = old })

	if WaitReady(LockInfo{Host: "127.0.0.1", Port: 1}, 200*time.Millisecond) {
		t.Fatal("不可达实例不应就绪")
	}

	// 延迟就绪：前 300ms 返回 500，之后返回本产品身份，验证轮询等待
	var ready int32
	time.AfterFunc(300*time.Millisecond, func() { atomic.StoreInt32(&ready, 1) })
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&ready) == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"status":"ok","service":"serialhub","role":"master"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	if !WaitReady(LockInfo{Host: "127.0.0.1", Port: port}, 3*time.Second) {
		t.Fatal("延迟就绪的实例应被等到")
	}
}

func TestWaitForInfo(t *testing.T) {
	testLockPath(t)
	if got := WaitForInfo(300 * time.Millisecond); got != nil {
		t.Fatalf("无活主应返回 nil: %+v", got)
	}
	if _, err := Acquire("127.0.0.1", 5050); err != nil {
		t.Fatal(err)
	}
	if got := WaitForInfo(2 * time.Second); got == nil || got.Port != 5050 {
		t.Fatalf("持锁后应读到元数据: %+v", got)
	}
}

// TestUpdatePort 验证端口迁移后 lock 元数据同步（端口被占自动 +1 的场景）。
// TestSideDetail 验证细粒度来源标识：WSL 下返回发行版名，
// 非 WSL 平台返回主机名（不断言具体值，只验证行为一致且不 panic）。
func TestSideDetail(t *testing.T) {
	if IsWSL() {
		t.Setenv("WSL_DISTRO_NAME", "TestDistro")
		if got := SideDetail(); got != "TestDistro" {
			t.Errorf("WSL 下 SideDetail 预期 TestDistro，实际 %q", got)
		}
	} else {
		h, _ := os.Hostname()
		if got := SideDetail(); got != h {
			t.Errorf("非 WSL 下 SideDetail 预期 hostname %q，实际 %q", h, got)
		}
	}
}

func TestUpdatePort(t *testing.T) {
	_ = testLockPath(t)

	if _, err := Acquire("127.0.0.1", 5099); err != nil {
		t.Fatalf("Acquire 失败: %v", err)
	}
	defer Release()

	UpdatePort(5100)
	info := Read()
	if info == nil || info.Port != 5100 {
		t.Fatalf("UpdatePort 后 Read 预期 port=5100，实际 %+v", info)
	}
	if info.Host != "127.0.0.1" || info.PID != os.Getpid() {
		t.Errorf("UpdatePort 不应改动 host/pid: %+v", info)
	}

	// 越界端口忽略，元数据不变
	UpdatePort(70000)
	if info = Read(); info == nil || info.Port != 5100 {
		t.Fatalf("越界 UpdatePort 后 port 应保持 5100，实际 %+v", info)
	}
}

// TestUpdatePort_未持锁时幂等空操作。
func TestUpdatePort_未持锁时幂等空操作(t *testing.T) {
	path := testLockPath(t)
	Release() // 确保未持锁
	UpdatePort(5100)
	if _, err := os.Stat(path); err == nil {
		// 文件可能因 testLockPath 创建？未 Acquire 不写内容，文件不存在是正常路径
	}
	// 未持锁时不 panic、不影响后续 Acquire
	if _, err := Acquire("127.0.0.1", 5099); err != nil {
		t.Fatalf("UpdatePort 后 Acquire 失败: %v", err)
	}
}
