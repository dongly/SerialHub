package serial

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	serial "go.bug.st/serial"

	"github.com/dongly/serialhub/internal/testutil"
)

// quietPort 模拟"连接正常但无数据"的串口：Read 阻塞直到 Close。
// 用于让 readLoop 稳定驻留，测试主动断开/意外断开/重连的精确路径。
type quietPort struct {
	closed chan struct{}
	once   sync.Once
}

func newQuietPort() *quietPort {
	return &quietPort{closed: make(chan struct{})}
}

func (q *quietPort) Read(p []byte) (int, error) {
	// 模拟真实串口的读超时：周期性空转返回 (0,nil)，让 readLoop
	// 有机会检查 ctx；Close 后立即返回 ErrClosedPipe
	select {
	case <-q.closed:
		return 0, io.ErrClosedPipe
	case <-time.After(20 * time.Millisecond):
		return 0, nil
	}
}

func (q *quietPort) Write(p []byte) (int, error) { return len(p), nil }

func (q *quietPort) Close() error {
	q.once.Do(func() { close(q.closed) })
	return nil
}

func (q *quietPort) SetReadTimeout(t time.Duration) error { return nil }

// fakePortDetails usbPortDetails 的测试假实现。
type fakePortDetails struct {
	name  string
	isUSB bool
	vid   string
	pid   string
}

func (f *fakePortDetails) Name() string { return f.name }
func (f *fakePortDetails) IsUSB() bool  { return f.isUSB }
func (f *fakePortDetails) VID() string  { return f.vid }
func (f *fakePortDetails) PID() string  { return f.pid }

// newReconnectTestManager 构造注入了假端口列表/USB 身份、快速退避的 manager。
func newReconnectTestManager(t *testing.T, port string, ports []string, details []usbPortDetails) *SerialManager {
	t.Helper()
	sm, err := NewSerialManager(&Config{Port: port, BaudRate: 115200, DataBits: 8, Parity: "none", StopBits: 1})
	if err != nil {
		t.Fatalf("NewSerialManager() failed: %v", err)
	}
	sm.reconnectBaseDelay = 5 * time.Millisecond
	sm.listPortsFn = func() ([]string, error) { return ports, nil }
	// 隔离真实 /dev/pts：宿主机的 pty 节点不应混入重连测试的列表断言
	sm.listPtsFn = func() []string { return nil }
	sm.listDetailedPorts = func() []usbPortDetails { return details }
	return sm
}

// waitConnected 轮询等待连接建立，超时失败。
func waitConnected(t *testing.T, sm *SerialManager, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sm.IsConnected() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("等待重连成功超时")
}

// waitClosed 轮询等待端口被异步关闭完成（Close 在独立 goroutine 中执行，
// 不能在触发事件后立即断言）。
func waitClosed(t *testing.T, port *testutil.MockSerialPort, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !port.IsClosed() {
		if time.Now().After(deadline) {
			t.Fatal("等待端口被关闭超时")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// currentReconnectGen 读取当前重连代次（测试直接启动 reconnectLoop 用）。
func currentReconnectGen(sm *SerialManager) uint64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.reconnectGen
}

// TestReadLoop_ClearsConnectionOnError 核心回归：意外断开后立即清连接状态，
// IsConnected 不再恒真（修复"假连接"导致无法再连接）。
func TestReadLoop_ClearsConnectionOnError(t *testing.T) {
	sm := newReconnectTestManager(t, "ttyUSB9", []string{"ttyUSB9"}, nil)
	defer sm.Close()

	mockPort := testutil.NewMockSerialPort(nil)
	mockPort.ReadErr = errors.New("device detached")
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	events := make(chan Event, 4)
	sm.SetEventHandler(func(e Event) { events <- e })

	sm.wg.Add(1)
	go sm.readLoop(mockPort)

	select {
	case e := <-events:
		if e.Type != EventError {
			t.Errorf("意外读错误应触发 EventError，实际 %v", e.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("未收到断开事件")
	}

	if sm.IsConnected() {
		t.Error("意外断开后 IsConnected() 应立即为 false（假连接修复）")
	}
	waitClosed(t, mockPort, time.Second)
}

// TestReconnect_SucceedsAfterRetries 端口存在但前几次打开失败：退避后重连成功。
func TestReconnect_SucceedsAfterRetries(t *testing.T) {
	sm := newReconnectTestManager(t, "ttyUSB9", []string{"ttyUSB9"}, nil)
	defer sm.Close()

	var openCalls int32
	sm.openPort = func(name string, mode *serial.Mode) (Port, error) {
		if atomic.AddInt32(&openCalls, 1) <= 2 {
			return nil, fmt.Errorf("设备忙 (第 %d 次)", openCalls)
		}
		return newQuietPort(), nil
	}

	connected := make(chan Event, 4)
	sm.SetEventHandler(func(e Event) {
		if e.Type == EventConnected {
			connected <- e
		}
	})

	// 直接触发意外断开路径（绕过 readLoop，聚焦重连循环）
	sm.mu.Lock()
	sm.portVID, sm.portPID = "", ""
	sm.mu.Unlock()
	go sm.reconnectLoop("ttyUSB9", "", "", currentReconnectGen(sm))

	waitConnected(t, sm, 2*time.Second)
	if n := atomic.LoadInt32(&openCalls); n != 3 {
		t.Errorf("openPort 调用 %d 次, want 3", n)
	}
}

// TestReconnect_GivesUpAfterMaxAttempts 端口始终打不开：耗尽上限后放弃并广播错误。
func TestReconnect_GivesUpAfterMaxAttempts(t *testing.T) {
	sm := newReconnectTestManager(t, "ttyUSB9", []string{"ttyUSB9"}, nil)
	sm.reconnectMaxAttempts = 3

	var openCalls int32
	sm.openPort = func(name string, mode *serial.Mode) (Port, error) {
		atomic.AddInt32(&openCalls, 1)
		return nil, errors.New("设备不可用")
	}

	events := make(chan Event, 8)
	sm.SetEventHandler(func(e Event) { events <- e })

	done := make(chan struct{})
	gen := currentReconnectGen(sm) // 先同步取代次，避免与主流程 gen++ 竞态
	go func() {
		defer close(done)
		sm.reconnectLoop("ttyUSB9", "", "", gen)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("重连循环未在耗尽上限后退出")
	}

	if n := atomic.LoadInt32(&openCalls); n != 3 {
		t.Errorf("openPort 调用 %d 次, want 3", n)
	}
	// 最终应广播一条"自动重连失败"错误事件（emitEvent 异步，需等待到达）
	var gaveUp bool
	timeout := time.After(1 * time.Second)
	for !gaveUp {
		select {
		case e := <-events:
			if e.Type == EventError && strings.Contains(e.Message, "自动重连失败") {
				gaveUp = true
			}
		case <-timeout:
			t.Fatal("等待自动重连失败事件超时")
		}
	}
	sm.Close()
}

// TestReconnect_UserReconnectedFirst 重连等待期间用户手动恢复：循环应退出且不再尝试。
func TestReconnect_UserReconnectedFirst(t *testing.T) {
	sm := newReconnectTestManager(t, "ttyUSB9", []string{"ttyUSB9"}, nil)
	defer sm.Close()

	var openCalls int32
	sm.openPort = func(name string, mode *serial.Mode) (Port, error) {
		atomic.AddInt32(&openCalls, 1)
		return newQuietPort(), nil
	}
	sm.reconnectBaseDelay = 100 * time.Millisecond // 拉长首拍，给手动重连留窗口

	done := make(chan struct{})
	gen := currentReconnectGen(sm) // 先同步取代次，避免与主流程 gen++ 竞态
	go func() {
		defer close(done)
		sm.reconnectLoop("ttyUSB9", "", "", gen)
	}()

	// 用户在首拍等待期间手动连接成功
	sm.mu.Lock()
	sm.port = newQuietPort()
	sm.mu.Unlock()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("检测到已恢复后重连循环未退出")
	}
	if n := atomic.LoadInt32(&openCalls); n != 0 {
		t.Errorf("用户已重连时不应再调用 openPort，实际 %d 次", n)
	}
}

// TestReconnect_PortRenamedByVIDPID 端口改名：原端口消失，按 VID/PID 匹配新端口，
// 且配置同步更新为新端口名。
func TestReconnect_PortRenamedByVIDPID(t *testing.T) {
	details := []usbPortDetails{
		&fakePortDetails{name: "ttyUSB7", isUSB: true, vid: "10c4", pid: "ea60"},
		&fakePortDetails{name: "OTHER", isUSB: true, vid: "1a86", pid: "7523"},
	}
	// 可用列表中原名已消失，只剩改名后的新端口
	sm := newReconnectTestManager(t, "ttyUSB8", []string{"ttyUSB7", "OTHER"}, details)
	defer sm.Close()

	sm.openPort = func(name string, mode *serial.Mode) (Port, error) {
		if name != "ttyUSB7" {
			return nil, fmt.Errorf("端口不存在: %s", name)
		}
		return newQuietPort(), nil
	}

	go sm.reconnectLoop("ttyUSB8", "10c4", "ea60", currentReconnectGen(sm))

	waitConnected(t, sm, 2*time.Second)
	if got := sm.CurrentPort(); got != "ttyUSB7" {
		t.Errorf("重连后 CurrentPort() = %s, want ttyUSB7", got)
	}
}

// TestReconnect_CancelStopsLoop manager 关闭（ctx 取消）时重连循环应立即退出。
func TestReconnect_CancelStopsLoop(t *testing.T) {
	sm := newReconnectTestManager(t, "ttyUSB9", nil, nil) // 无可用端口，循环会一直等
	defer sm.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		sm.reconnectLoop("ttyUSB9", "", "", currentReconnectGen(sm))
	}()

	time.Sleep(20 * time.Millisecond)
	sm.cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后重连循环未退出")
	}
}

// TestMatchReconnectPort 重连目标解析纯函数：原名优先、VID/PID 匹配、边界情况。
func TestMatchReconnectPort(t *testing.T) {
	details := []usbPortDetails{
		&fakePortDetails{name: "ttyUSB1", isUSB: true, vid: "10c4", pid: "ea60"},
		&fakePortDetails{name: "ttyACM0", isUSB: false, vid: "10c4", pid: "ea60"}, // 非 USB 不匹配
	}
	tests := []struct {
		name     string
		ports    []string
		details  []usbPortDetails
		portName string
		vid, pid string
		want     string
	}{
		{"原名存在优先", []string{"ttyUSB0", "ttyUSB1"}, details, "ttyUSB0", "10c4", "ea60", "ttyUSB0"},
		{"原名消失按VID/PID", []string{"ttyUSB1"}, details, "ttyUSB0", "10c4", "ea60", "ttyUSB1"},
		{"无VID/PID降级为空", []string{"ttyUSB1"}, details, "ttyUSB0", "", "", ""},
		{"VID不匹配", []string{"ttyUSB1"}, details, "ttyUSB0", "1a86", "7523", ""},
		{"匹配端口不在可用列表", []string{"ttyACM0"}, details, "ttyUSB0", "10c4", "ea60", ""},
		{"无任何端口", nil, details, "ttyUSB0", "10c4", "ea60", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchReconnectPort(tt.ports, tt.details, tt.portName, tt.vid, tt.pid)
			if got != tt.want {
				t.Errorf("matchReconnectPort() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReconnect_ErrChanFullStillBroadcasts 错误通道满（无消费方）时，
// 意外断开的事件广播与自动重连不得被背压阻塞（诊断通道不承载控制流）。
func TestReconnect_ErrChanFullStillBroadcasts(t *testing.T) {
	sm := newReconnectTestManager(t, "ttyUSB9", []string{"ttyUSB9"}, nil)
	defer sm.Close()

	sm.openPort = func(name string, mode *serial.Mode) (Port, error) {
		return newQuietPort(), nil
	}

	// 预填满 errChan（容量 16，无消费方）
	for i := 0; i < cap(sm.errChan); i++ {
		sm.errChan <- fmt.Errorf("占位错误 %d", i)
	}

	events := make(chan Event, 4)
	sm.SetEventHandler(func(e Event) { events <- e })

	mockPort := testutil.NewMockSerialPort(nil)
	mockPort.ReadErr = errors.New("device detached")
	sm.mu.Lock()
	sm.port = mockPort
	sm.mu.Unlock()

	sm.wg.Add(1)
	go sm.readLoop(mockPort)

	// 事件必须及时到达（未被塞满的 errChan 阻塞）
	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("errChan 满时断开事件仍被阻塞")
	}
	// 自动重连必须照常启动并成功
	waitConnected(t, sm, 2*time.Second)
}

// TestReconnect_DisconnectCancelsPending 用户在掉线后主动点"断开"
// （幂等成功）：挂起的自动重连任务必须退出，设备回神后不得自动连上。
func TestReconnect_DisconnectCancelsPending(t *testing.T) {
	sm := newReconnectTestManager(t, "ttyUSB9", []string{"ttyUSB9"}, nil)
	defer sm.Close()

	var openCalls int32
	sm.openPort = func(name string, mode *serial.Mode) (Port, error) {
		atomic.AddInt32(&openCalls, 1)
		return newQuietPort(), nil
	}
	sm.reconnectBaseDelay = 100 * time.Millisecond // 拉长首拍，给断开操作留窗口

	// 未连接状态下（模拟已掉线）挂起重连任务
	done := make(chan struct{})
	gen := currentReconnectGen(sm) // 先同步取代次，再让用户操作改写代次
	go func() {
		defer close(done)
		sm.reconnectLoop("ttyUSB9", "", "", gen)
	}()

	// 用户点断开（此时 port 已 nil）：幂等成功且作废重连
	if err := sm.Disconnect(); err != nil {
		t.Fatalf("掉线后 Disconnect() 应幂等成功，实际: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("用户断开后重连任务未退出")
	}
	if n := atomic.LoadInt32(&openCalls); n != 0 {
		t.Errorf("用户断开后不应再尝试连接，openPort 调用 %d 次", n)
	}
}

// gatedPort 时序可控的假串口：Read 入口先发 entered 信号，随后阻塞直到
// 测试放行（close release），放行时返回预设数据。用于确定性地复现
// 「Read 在途期间端口被替换」的交错（等待 entered 而非 sleep）。
type gatedPort struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	data    []byte
}

func newGatedPort(data []byte) *gatedPort {
	return &gatedPort{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		data:    data,
	}
}

func (g *gatedPort) Read(p []byte) (int, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return copy(p, g.data), nil
}
func (g *gatedPort) Write(p []byte) (int, error)          { return len(p), nil }
func (g *gatedPort) Close() error                         { return nil }
func (g *gatedPort) SetReadTimeout(t time.Duration) error { return nil }

// waitPortEntered 有界等待 readLoop 已挂起在 gatedPort.Read 上，超时失败。
func waitPortEntered(t *testing.T, g *gatedPort) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("读取循环未进入 Read")
	}
}

// waitReadLoopExit 等待读取循环（wg）全部退出，超时失败。
func waitReadLoopExit(t *testing.T, sm *SerialManager) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("读取循环未能退出")
	}
}

// TestReadLoop_SticksToItsOwnPort 读取循环绑定启动时的端口实例：
// 旧端口的在途 Read 返回后，不得转而读取 sm.port 上的新连接，
// 其在途数据也不得在新连接建立后发布到 dataChan。
func TestReadLoop_SticksToItsOwnPort(t *testing.T) {
	t.Run("空转返回后旧循环退出", func(t *testing.T) {
		sm := newReconnectTestManager(t, "ttyUSB9", []string{"ttyUSB9"}, nil)
		defer sm.Close()

		oldPort := newGatedPort(nil) // 放行后返回 (0,nil)
		sm.mu.Lock()
		sm.port = oldPort
		sm.mu.Unlock()
		sm.wg.Add(1)
		go sm.readLoop(oldPort)

		waitPortEntered(t, oldPort) // 确定性地等到旧循环挂起在 Read 上

		// 旧 Read 在途期间：端口被替换为新连接（模拟用户断开后连接 B）
		sm.mu.Lock()
		sm.port = newQuietPort()
		sm.mu.Unlock()

		close(oldPort.release) // 放行旧 Read

		waitReadLoopExit(t, sm)
		if !sm.IsConnected() {
			t.Error("新端口连接状态不应被旧循环破坏")
		}
	})

	t.Run("在途数据不发布到新连接", func(t *testing.T) {
		sm := newReconnectTestManager(t, "ttyUSB9", []string{"ttyUSB9"}, nil)
		defer sm.Close()

		oldPort := newGatedPort([]byte("OLD-DATA"))
		sm.mu.Lock()
		sm.port = oldPort
		sm.mu.Unlock()
		sm.wg.Add(1)
		go sm.readLoop(oldPort)

		waitPortEntered(t, oldPort)

		// 旧 Read 在途期间切换到新端口
		sm.mu.Lock()
		sm.port = newQuietPort()
		sm.mu.Unlock()

		close(oldPort.release) // 旧 Read 携带数据返回

		waitReadLoopExit(t, sm)

		// 旧连接的在途数据不得混入新连接的数据流（发送方已退出，无需等待）
		select {
		case d := <-sm.dataChan:
			t.Errorf("旧连接在途数据不应发布，收到 %q", d)
		default:
		}
	})
}

// TestReconnect_RevertsToOriginalPort 改名目标打开失败后原名恢复：
// 下一拍按当前配置同步回原名连接，而不是沿用已失败的改名目标。
func TestReconnect_RevertsToOriginalPort(t *testing.T) {
	details := []usbPortDetails{
		&fakePortDetails{name: "ttyUSB7", isUSB: true, vid: "10c4", pid: "ea60"},
		&fakePortDetails{name: "ttyUSB9", isUSB: true, vid: "10c4", pid: "ea60"},
	}
	sm := newReconnectTestManager(t, "ttyUSB9", nil, details)
	defer sm.Close()
	sm.reconnectBaseDelay = 5 * time.Millisecond

	var mu sync.Mutex
	cur := []string{"ttyUSB7"} // 第一拍：仅改名后的 B 存在
	sm.listPortsFn = func() ([]string, error) {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), cur...), nil
	}

	var lastPort atomic.Value // string
	sm.openPort = func(name string, mode *serial.Mode) (Port, error) {
		lastPort.Store(name)
		if name == "ttyUSB7" {
			// B 打开失败后原名恢复
			mu.Lock()
			cur = []string{"ttyUSB9", "ttyUSB7"}
			mu.Unlock()
			return nil, errors.New("设备不可用")
		}
		return newQuietPort(), nil
	}

	gen := currentReconnectGen(sm)
	go sm.reconnectLoop("ttyUSB9", "10c4", "ea60", gen)

	waitConnected(t, sm, 2*time.Second)
	if got, _ := lastPort.Load().(string); got != "ttyUSB9" {
		t.Errorf("原名恢复后应连接 ttyUSB9，实际 openPort(%s)", got)
	}
	if got := sm.CurrentPort(); got != "ttyUSB9" {
		t.Errorf("重连成功后 CurrentPort() = %s, want ttyUSB9", got)
	}
}

// TestConnect_AfterCloseRejected 管理器关闭后连接必须被拒绝：
// 防止 Close 的 wg.Wait 之后仍有 Connect 发布 readLoop（WaitGroup 复用
// panic / 关闭后串口被重新占用）。
func TestConnect_AfterCloseRejected(t *testing.T) {
	sm := newReconnectTestManager(t, "ttyUSB9", []string{"ttyUSB9"}, nil)
	sm.openPort = func(name string, mode *serial.Mode) (Port, error) {
		return newQuietPort(), nil
	}

	if err := sm.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	if err := sm.Connect(); err == nil {
		t.Error("Close() 后 Connect() 应返回错误")
	}
	// 幂等：重复 Close 不应 panic
	if err := sm.Close(); err != nil {
		t.Fatalf("重复 Close() 应幂等成功，实际: %v", err)
	}
}
