package tools

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dongly/serialhub/internal/buffer"
	"github.com/dongly/serialhub/pkg/serial"
)

// startPtyProcess 启动一个由 python3 持有的 pty：执行 pyScript（须向 stdout 输出
// 从设备路径一行），附加 args 透传给脚本，返回从设备路径（等价于一个真实串口）。
// 无 python3 或启动失败时 Skip，测试结束自动回收进程。
func startPtyProcess(t *testing.T, pyScript string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 不可用，跳过 pty 测试: %v", err)
	}
	cmd := exec.Command("python3", append([]string{"-c", pyScript}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Skipf("创建 pty 输出管道失败: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("启动 pty 设备失败: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Skipf("读取 pty 设备路径失败: %v（stderr: %s）", err, stderr.String())
	}
	port := strings.TrimSpace(line)
	if port == "" {
		t.Skip("pty 设备路径为空")
	}
	return port
}

// holdPtyPy：仅持有 pty 不读写（数据留在内核缓冲，不回流）。
const holdPtyPy = `import os, pty, sys, time
m, s = pty.openpty()
sys.stdout.write(os.ttyname(s) + "\n")
sys.stdout.flush()
time.sleep(600)`

// echoPtyPy：回显 responder——读 master 落盘到 argv[1] 并写回（脚本侧收到回显）。
const echoPtyPy = `import os, pty, sys, select, tty
log = open(sys.argv[1], "ab", buffering=0)
m, s = pty.openpty()
tty.setraw(s)
sys.stdout.write(os.ttyname(s) + "\n")
sys.stdout.flush()
while True:
    r, _, _ = select.select([m], [], [], 60)
    if not r:
        continue
    try:
        data = os.read(m, 4096)
    except OSError:
        break
    if not data:
        break
    log.write(data)
    os.write(m, data)`

// startPtyDevice 启动仅持有的 pty，返回从设备路径。
func startPtyDevice(t *testing.T) string {
	t.Helper()
	return startPtyProcess(t, holdPtyPy)
}

// newPtyEnv 创建连接到 pty 的 SerialManager 与独立 DataBuffer，测试结束自动清理。
func newPtyEnv(t *testing.T) (*serial.SerialManager, *buffer.DataBuffer, string) {
	t.Helper()
	port := startPtyDevice(t)
	cfg := serial.DefaultConfig()
	cfg.Port = port
	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("创建 SerialManager 失败: %v", err)
	}
	t.Cleanup(func() {
		if sm.IsConnected() {
			sm.Disconnect()
		}
		sm.Close()
	})
	res := ExecuteSerialConnect(sm, ConnectInput{Port: port})
	if !res.Success {
		t.Fatalf("连接 pty %s 失败: %s", port, res.Message)
	}
	return sm, buffer.NewDataBuffer(), port
}

// runScript 异步执行脚本，便于测试在执行期间向缓冲注入数据或断开连接。
func runScript(ctx context.Context, sm *serial.SerialManager, buf *buffer.DataBuffer, input ScriptInput) <-chan ToolResult {
	ch := make(chan ToolResult, 1)
	go func() {
		ch <- ExecuteSerialScript(ctx, sm, buf, input, testScriptLimits)
	}()
	return ch
}

// testScriptLimits 与生产默认一致的超时范围。
var testScriptLimits = ScriptLimits{MinMs: 100, MaxMs: 1800000}

// waitResult 等待脚本返回，10 秒未返回视为测试失败。
func waitResult(t *testing.T, ch <-chan ToolResult) ToolResult {
	t.Helper()
	select {
	case res := <-ch:
		return res
	case <-time.After(10 * time.Second):
		t.Fatal("脚本 10s 内未返回")
		return ToolResult{}
	}
}

// scriptData 断言并取出 Data 字典，缺失即测试失败。
func scriptData(t *testing.T, res ToolResult) map[string]any {
	t.Helper()
	data, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data 应为 map[string]any，实际: %#v", res.Data)
	}
	return data
}

// ==================== 参数校验（无需真实串口） ====================

func TestSerialScript_Validation(t *testing.T) {
	sm := newTestManager(t)
	buf := buffer.NewDataBuffer()
	ctx := context.Background()

	t.Run("NilManager", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, nil, buf, ScriptInput{TimeoutMs: 1000}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "串口管理器未初始化") {
			t.Errorf("预期管理器未初始化失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("NilBuffer", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, sm, nil, ScriptInput{TimeoutMs: 1000}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "数据缓冲未初始化") {
			t.Errorf("预期缓冲未初始化失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("MissingTimeout", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "timeoutMs 必填") {
			t.Errorf("预期缺 timeoutMs 失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("TimeoutBelowMin", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{TimeoutMs: 99}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "超出允许范围") {
			t.Errorf("预期低于下限失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("TimeoutAboveMax", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{TimeoutMs: 1800001}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "超出允许范围") {
			t.Errorf("预期高于上限失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("EmptyScript", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{TimeoutMs: 1000}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "剧本为空") {
			t.Errorf("预期空剧本失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("NotConnected", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{
			TimeoutMs: 1000,
			Writes:    []TimedWrite{{Data: "x"}},
		}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "串口未连接") {
			t.Errorf("预期未连接失败，实际: %t %s", res.Success, res.Message)
		}
	})
}

// ==================== 已连接后的参数校验 ====================

func TestSerialScript_ValidationConnected(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	ctx := context.Background()

	t.Run("BadPattern", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{
			TimeoutMs: 1000,
			Matches:   []MatchRule{{Pattern: "[", Data: "x"}},
		}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "正则无效") {
			t.Errorf("预期正则无效失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("NegativeAtMs", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{
			TimeoutMs: 1000,
			Writes:    []TimedWrite{{AtMs: -1, Data: "x"}},
		}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "atMs 不能为负") {
			t.Errorf("预期负 atMs 失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("AtMsOverflow", func(t *testing.T) {
		// 巨大 atMs × time.Millisecond 溢出成负时长会让远期写提前发送，必须拒绝
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{
			TimeoutMs: 1000,
			Writes:    []TimedWrite{{AtMs: math.MaxInt64, Data: "x"}},
		}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "超出可表示范围") {
			t.Errorf("预期 atMs 溢出失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("IntervalOverflow", func(t *testing.T) {
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{
			TimeoutMs: 1000,
			Writes:    []TimedWrite{{IntervalMs: math.MaxInt64, Count: 2, Data: "x"}},
		}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "超出可表示范围") {
			t.Errorf("预期 intervalMs 溢出失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("CountOverflow", func(t *testing.T) {
		// count 膨胀会 OOM，展开条目设上限
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{
			TimeoutMs: 1000,
			Writes:    []TimedWrite{{IntervalMs: 1, Count: 200000, Data: "x"}},
		}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "超出上限") {
			t.Errorf("预期 count 超限失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("CumulativeOverflow", func(t *testing.T) {
		// 多条周期写累计也不得越过上限（60000+60000 > 100000，第二条应拒绝）
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{
			TimeoutMs: 1000,
			Writes: []TimedWrite{
				{IntervalMs: 1, Count: 60000, Data: "x"},
				{IntervalMs: 1, Count: 60000, Data: "y"},
			},
		}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "超出上限") {
			t.Errorf("预期累计展开超限失败，实际: %t %s", res.Success, res.Message)
		}
	})
	t.Run("MaxIntCountNoOverflow", func(t *testing.T) {
		// len+want 求和若用 int 加法会溢出成负绕过检查：count=MaxInt 必须被直接拒绝
		res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{
			TimeoutMs: 1000,
			Writes: []TimedWrite{
				{AtMs: 0, Data: "x"},
				{IntervalMs: 1, Count: math.MaxInt, Data: "y"},
			},
		}, testScriptLimits)
		if res.Success || !strings.Contains(res.Message, "超出上限") {
			t.Errorf("预期 MaxInt count 拒绝失败，实际: %t %s", res.Success, res.Message)
		}
	})
}

// ==================== 定时写 ====================

func TestSerialScript_TimedWrites(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	// 无匹配规则：全部定时写发出即完成，无需等待超时
	res := ExecuteSerialScript(context.Background(), sm, buf, ScriptInput{
		TimeoutMs: 5000,
		Writes: []TimedWrite{
			{AtMs: 10, Data: "A", AddNewline: boolPtr(false)},
			{AtMs: 30, Data: "B", AddNewline: boolPtr(false)},
			{AtMs: 60, Data: "C", AddNewline: boolPtr(false)},
		},
	}, testScriptLimits)

	if !res.Success {
		t.Fatalf("预期成功，实际失败: %s", res.Message)
	}
	data := scriptData(t, res)
	if timedOut, _ := data["timedOut"].(bool); timedOut {
		t.Errorf("预期未超时，实际 timedOut=true")
	}
	if got, _ := data["writesFired"].(int); got != 3 {
		t.Errorf("预期 writesFired=3，实际 %d", got)
	}
	if got, _ := data["writesTotal"].(int); got != 3 {
		t.Errorf("预期 writesTotal=3，实际 %d", got)
	}
	triggers, _ := data["triggers"].([]map[string]any)
	if len(triggers) != 3 {
		t.Fatalf("预期 3 条触发记录，实际 %d", len(triggers))
	}
	for i, tr := range triggers {
		if tr["type"] != "timed" {
			t.Errorf("触发记录 #%d 类型应为 timed，实际 %v", i, tr["type"])
		}
		// 单发条目 occurrence 恒为 0
		if tr["occurrence"] != 0 {
			t.Errorf("触发记录 #%d occurrence 应为 0，实际 %v", i, tr["occurrence"])
		}
	}
	if !strings.Contains(res.Message, "脚本执行完成") {
		t.Errorf("消息应含'脚本执行完成'，实际: %s", res.Message)
	}
}

// ==================== 匹配写：单发 ====================

func TestSerialScript_MatchSingle(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	ctx := context.Background()
	ch := runScript(ctx, sm, buf, ScriptInput{
		TimeoutMs: 5000,
		Writes:    []TimedWrite{{AtMs: 0, Data: "hello"}},
		Matches:   []MatchRule{{Pattern: "READY", Data: "ACK"}},
	})
	// 等脚本完成启动（订阅 + 首拍定时写）
	time.Sleep(200 * time.Millisecond)
	buf.Append([]byte("device says READY now"))

	res := waitResult(t, ch)
	if !res.Success {
		t.Fatalf("预期成功，实际失败: %s", res.Message)
	}
	data := scriptData(t, res)
	counts, _ := data["ruleFireCounts"].([]int)
	if len(counts) != 1 || counts[0] != 1 {
		t.Errorf("预期单发规则触发 1 次，实际 %v", counts)
	}
	triggers, _ := data["triggers"].([]map[string]any)
	var matchFired, timedFired bool
	for _, tr := range triggers {
		switch tr["type"] {
		case "match":
			matchFired = true
			if tr["matched"] != "READY" {
				t.Errorf("matched 应为正则命中片段 READY，实际: %v", tr["matched"])
			}
		case "timed":
			timedFired = true
		}
	}
	if !timedFired {
		t.Errorf("应包含定时写触发记录: %v", triggers)
	}
	if !matchFired {
		t.Errorf("应包含匹配触发记录: %v", triggers)
	}
}

// ==================== 匹配写：大 chunk 先扫后裁 ====================

func TestSerialScript_LargeChunkHeadMatched(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	ctx := context.Background()
	ch := runScript(ctx, sm, buf, ScriptInput{
		TimeoutMs: 5000,
		Matches:   []MatchRule{{Pattern: "READY", Data: "ACK"}},
	})
	// 等脚本启动（订阅生效）
	time.Sleep(200 * time.Millisecond)
	// 单次注入 9KB（超过 8KB 匹配窗口）：命中串在最开头，不得因先裁剪而丢失
	big := append([]byte("READY"), make([]byte, 9*1024)...)
	buf.Append(big)

	res := waitResult(t, ch)
	if !res.Success {
		t.Fatalf("预期成功，实际失败: %s", res.Message)
	}
	data := scriptData(t, res)
	counts, _ := data["ruleFireCounts"].([]int)
	if len(counts) != 1 || counts[0] != 1 {
		t.Errorf("预期大 chunk 开头的 READY 命中 1 次，实际 %v", counts)
	}
}

// ==================== 匹配写：repeat + maxCount ====================

func TestSerialScript_RepeatMaxCount(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	ctx := context.Background()
	ch := runScript(ctx, sm, buf, ScriptInput{
		TimeoutMs: 5000,
		Writes:    []TimedWrite{{AtMs: 0, Data: "init"}},
		Matches: []MatchRule{
			{Pattern: "PING", Data: "PONG", Repeat: true, MaxCount: 2},
		},
	})
	time.Sleep(200 * time.Millisecond)
	// 单次注入 3 个匹配串，maxCount=2 应恰好触发 2 次（第 3 个被限次忽略）
	buf.Append([]byte("PING PING PING"))

	res := waitResult(t, ch)
	if !res.Success {
		t.Fatalf("预期成功，实际失败: %s", res.Message)
	}
	data := scriptData(t, res)
	counts, _ := data["ruleFireCounts"].([]int)
	if len(counts) != 1 || counts[0] != 2 {
		t.Errorf("预期 repeat+maxCount=2 触发恰好 2 次，实际 %v", counts)
	}
	triggers, _ := data["triggers"].([]map[string]any)
	matchFired := 0
	for _, tr := range triggers {
		if tr["type"] == "match" {
			// repeat 第 n 次命中的 occurrence 应为 n（从 0 起）
			if tr["occurrence"] != matchFired {
				t.Errorf("match 触发 occurrence 应为 %d，实际 %v", matchFired, tr["occurrence"])
			}
			matchFired++
		}
	}
	if matchFired != 2 {
		t.Errorf("预期 2 条 match 触发记录，实际 %d", matchFired)
	}
}

// ==================== 超时：成功 + 未触发清单 ====================

func TestSerialScript_TimeoutPending(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	res := ExecuteSerialScript(context.Background(), sm, buf, ScriptInput{
		TimeoutMs: 300,
		Matches:   []MatchRule{{Pattern: "NEVER_SEEN", Data: "x"}},
	}, testScriptLimits)

	if !res.Success {
		t.Fatalf("超时应为成功，实际失败: %s", res.Message)
	}
	data := scriptData(t, res)
	if timedOut, _ := data["timedOut"].(bool); !timedOut {
		t.Errorf("预期 timedOut=true")
	}
	pending, _ := data["pendingRules"].([]int)
	if len(pending) != 1 || pending[0] != 0 {
		t.Errorf("预期未触发规则 [0]，实际 %v", pending)
	}
	counts, _ := data["ruleFireCounts"].([]int)
	if len(counts) != 1 || counts[0] != 0 {
		t.Errorf("预期规则触发 0 次，实际 %v", counts)
	}
	if !strings.Contains(res.Message, "脚本超时") || !strings.Contains(res.Message, "未触发规则") {
		t.Errorf("消息应含超时与未触发规则信息，实际: %s", res.Message)
	}
}

// ==================== 断连中止（失败） ====================

func TestSerialScript_DisconnectAbort(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	ctx := context.Background()
	ch := runScript(ctx, sm, buf, ScriptInput{
		TimeoutMs: 10000,
		Matches:   []MatchRule{{Pattern: "NEVER_SEEN", Data: "x"}},
	})
	time.Sleep(250 * time.Millisecond)
	sm.Disconnect()

	res := waitResult(t, ch)
	if res.Success {
		t.Errorf("断连中止应为失败，实际成功: %s", res.Message)
	}
	if !strings.Contains(res.Message, "串口断连") {
		t.Errorf("消息应含'串口断连'，实际: %s", res.Message)
	}
}

// ==================== 全局单实例：第二个脚本被拒绝 + ctx 取消 ====================

func TestSerialScript_SecondRejected(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := runScript(ctx, sm, buf, ScriptInput{
		TimeoutMs: 5000,
		Matches:   []MatchRule{{Pattern: "NEVER_SEEN", Data: "x"}},
	})
	time.Sleep(200 * time.Millisecond)

	// 第二个脚本应立即被拒绝
	res2 := ExecuteSerialScript(context.Background(), sm, buffer.NewDataBuffer(), ScriptInput{
		TimeoutMs: 1000,
		Writes:    []TimedWrite{{Data: "y"}},
	}, testScriptLimits)
	if res2.Success || !strings.Contains(res2.Message, "已有脚本运行中") {
		t.Errorf("预期第二脚本被拒绝，实际: %t %s", res2.Success, res2.Message)
	}

	// 取消第一个脚本：失败返回
	cancel()
	res1 := waitResult(t, ch)
	if res1.Success {
		t.Errorf("取消应为失败，实际成功: %s", res1.Message)
	}
	if !strings.Contains(res1.Message, "脚本已取消") {
		t.Errorf("消息应含'脚本已取消'，实际: %s", res1.Message)
	}
}

// ==================== 旧缓冲数据不参与匹配 ====================

func TestSerialScript_OldDataIgnored(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	// 脚本启动前已在缓冲中的数据不应触发匹配
	buf.Append([]byte("READY\n"))

	res := ExecuteSerialScript(context.Background(), sm, buf, ScriptInput{
		TimeoutMs: 300,
		Matches:   []MatchRule{{Pattern: "READY", Data: "x"}},
	}, testScriptLimits)

	if !res.Success {
		t.Fatalf("预期超时成功，实际失败: %s", res.Message)
	}
	data := scriptData(t, res)
	if timedOut, _ := data["timedOut"].(bool); !timedOut {
		t.Errorf("旧数据不应触发匹配，预期 timedOut=true")
	}
	pending, _ := data["pendingRules"].([]int)
	if len(pending) != 1 || pending[0] != 0 {
		t.Errorf("预期规则未触发 [0]，实际 %v", pending)
	}
	if got := fmt.Sprintf("%v", data["receivedBytes"]); got != "0" {
		t.Errorf("预期 receivedBytes=0（仅统计启动后新数据），实际 %s", got)
	}
}

// ==================== 零宽正则：不触发、不卡死 ====================

func TestSerialScript_ZeroWidthNoLoop(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	// "^" 零宽 + repeat：合法正则但命中零宽，不得无限触发，也不得卡死事件循环
	ch := runScript(context.Background(), sm, buf, ScriptInput{
		TimeoutMs: 300,
		Writes:    []TimedWrite{{AtMs: 0, Data: "x", AddNewline: boolPtr(false)}},
		Matches:   []MatchRule{{Pattern: "^", Data: "boom", Repeat: true}},
	})
	time.Sleep(150 * time.Millisecond)
	buf.Append([]byte("data"))

	res := waitResult(t, ch) // 若零宽导致死循环，此处 10s 兜底失败
	if !res.Success {
		t.Fatalf("预期超时成功，实际失败: %s", res.Message)
	}
	data := scriptData(t, res)
	if timedOut, _ := data["timedOut"].(bool); !timedOut {
		t.Errorf("零宽命中不应触发规则，预期 timedOut=true")
	}
	counts, _ := data["ruleFireCounts"].([]int)
	if len(counts) != 1 || counts[0] != 0 {
		t.Errorf("零宽命中不应计入触发，预期 [0]，实际 %v", counts)
	}
	if got, _ := data["writesFired"].(int); got != 1 {
		t.Errorf("预期定时写正常发出 1 次，实际 %d", got)
	}
}

// ==================== 预取消 ctx：不发送任何写 ====================

func TestSerialScript_PreCanceledCtx(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := ExecuteSerialScript(ctx, sm, buf, ScriptInput{
		TimeoutMs: 5000,
		Writes:    []TimedWrite{{AtMs: 0, Data: "SHOULD_NOT_SEND"}},
	}, testScriptLimits)

	if res.Success || !strings.Contains(res.Message, "脚本已取消") {
		t.Errorf("预取消 ctx 应直接失败返回'脚本已取消'，实际: %t %s", res.Success, res.Message)
	}
	data := scriptData(t, res)
	if got, _ := data["writesFired"].(int); got != 0 {
		t.Errorf("预取消不得发送任何写，实际 writesFired=%d", got)
	}
}

// ==================== 锚点语义：^ 相对窗口起点，repeat 不重锚 ====================

func TestSerialScript_AnchorNotReanchored(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	// "^PING" + repeat：单次注入 "PINGPING"，第二个 PING 不是窗口起点，不得再次触发
	ch := runScript(context.Background(), sm, buf, ScriptInput{
		TimeoutMs: 5000,
		Matches:   []MatchRule{{Pattern: "^PING", Data: "PONG", Repeat: true}},
	})
	time.Sleep(200 * time.Millisecond)
	buf.Append([]byte("PINGPING"))

	res := waitResult(t, ch)
	if !res.Success {
		t.Fatalf("预期成功（命中 1 次即满足触发条件），实际失败: %s", res.Message)
	}
	data := scriptData(t, res)
	counts, _ := data["ruleFireCounts"].([]int)
	if len(counts) != 1 || counts[0] != 1 {
		t.Errorf("预期 ^ 锚定窗口起点仅触发 1 次，实际 %v", counts)
	}
}

// ==================== 断连后快速重连：按连接代次中止 ====================

func TestSerialScript_ReconnectAbort(t *testing.T) {
	sm, buf, port := newPtyEnv(t)
	ch := runScript(context.Background(), sm, buf, ScriptInput{
		TimeoutMs: 10000,
		Writes:    []TimedWrite{{AtMs: 500, Data: "late", AddNewline: boolPtr(false)}},
		Matches:   []MatchRule{{Pattern: "NEVER_SEEN", Data: "x"}},
	})
	time.Sleep(200 * time.Millisecond)

	// 断连后立即重连同一端口：连接状态恢复为已连接，但代次已变，旧脚本必须中止
	sm.Disconnect()
	// 端口释放是异步的：轮询重连直至成功（无论脚本经"未连接"还是"代次变化"路径判定，都应中止）
	var res2 ToolResult
	for i := 0; i < 40; i++ {
		res2 = ExecuteSerialConnect(sm, ConnectInput{Port: port})
		if res2.Success {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !res2.Success {
		// 先等脚本返回，避免持锁 goroutine 泄漏影响后续测试
		waitResult(t, ch)
		t.Fatalf("重连 %s 失败: %s", port, res2.Message)
	}

	res := waitResult(t, ch)
	if res.Success {
		t.Errorf("断连后重连，旧脚本应中止失败，实际成功: %s", res.Message)
	}
	if !strings.Contains(res.Message, "串口断连") {
		t.Errorf("消息应含'串口断连'，实际: %s", res.Message)
	}
	// 待发的 500ms 定时写不得写进重连后的新连接
	if got, _ := scriptData(t, res)["writesFired"].(int); got != 0 {
		t.Errorf("中止前不得发出任何定时写，实际 writesFired=%d", got)
	}
}

// ==================== 截止后到达的数据：不匹配、不计入回放 ====================

func TestSerialScript_PostDeadlineDataSkipped(t *testing.T) {
	sm, buf, _ := newPtyEnv(t)
	// deadline=330ms；370ms（晚于 deadline）注入的匹配数据
	// 不得触发匹配写、不得计入回放、不得让剧本以"提前完成"成功返回
	ch := runScript(context.Background(), sm, buf, ScriptInput{
		TimeoutMs: 330,
		Matches:   []MatchRule{{Pattern: "PING", Data: "PONG"}},
	})
	time.Sleep(370 * time.Millisecond)
	buf.Append([]byte("PING"))

	res := waitResult(t, ch)
	if !res.Success {
		t.Fatalf("预期超时成功，实际失败: %s", res.Message)
	}
	data := scriptData(t, res)
	if timedOut, _ := data["timedOut"].(bool); !timedOut {
		t.Errorf("截止后数据不得让剧本'提前完成'，预期 timedOut=true")
	}
	counts, _ := data["ruleFireCounts"].([]int)
	if len(counts) != 1 || counts[0] != 0 {
		t.Errorf("截止后到达的数据不应触发匹配，预期 [0]，实际 %v", counts)
	}
	if got := fmt.Sprintf("%v", data["receivedBytes"]); got != "0" {
		t.Errorf("截止后到达的数据不应计入回放，预期 receivedBytes=0，实际 %s", got)
	}
}

// ==================== 真实链路：pty 回显设备 ====================

// startEchoResponder 启动 python3 回显 responder：持有 pty（从设备侧 raw，
// 无 ECHO 回环），把串口侧收到的数据落盘到日志文件并原样写回，返回从设备路径。
func startEchoResponder(t *testing.T, logPath string) string {
	t.Helper()
	return startPtyProcess(t, echoPtyPy, logPath)
}

// waitForLogContent 轮询日志文件直至包含全部期望子串（设备实收断言）。
func waitForLogContent(t *testing.T, path string, wants ...string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			ok := true
			for _, w := range wants {
				if !strings.Contains(string(raw), w) {
					ok = false
					break
				}
			}
			if ok {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	raw, _ := os.ReadFile(path)
	t.Fatalf("日志文件 3s 内未包含 %v，实际内容: %q", wants, string(raw))
}

func TestSerialScript_RealLinkEcho(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "echo.log")
	port := startEchoResponder(t, logPath)

	cfg := serial.DefaultConfig()
	cfg.Port = port
	sm, err := serial.NewSerialManager(cfg)
	if err != nil {
		t.Fatalf("创建 SerialManager 失败: %v", err)
	}
	t.Cleanup(func() {
		if sm.IsConnected() {
			sm.Disconnect()
		}
		sm.Close()
	})
	if res := ExecuteSerialConnect(sm, ConnectInput{Port: port}); !res.Success {
		t.Fatalf("连接 responder %s 失败: %s", port, res.Message)
	}

	buf := buffer.NewDataBuffer()
	// 复现 DataBridge 接线：串口接收 → DataBuffer（脚本经订阅观察）
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			select {
			case d, ok := <-sm.DataChan():
				if !ok {
					return
				}
				buf.Append(d)
			case <-done:
				return
			}
		}
	}()

	// 全链路：定时写 PING → 设备回显 PING → 匹配触发写 PONG → 设备回显 PONG
	res := ExecuteSerialScript(context.Background(), sm, buf, ScriptInput{
		TimeoutMs: 5000,
		Writes:    []TimedWrite{{AtMs: 0, Data: "PING", AddNewline: boolPtr(false)}},
		Matches:   []MatchRule{{Pattern: "PING", Data: "PONG", AddNewline: boolPtr(false)}},
	}, testScriptLimits)

	if !res.Success {
		t.Fatalf("预期真实链路成功，实际失败: %s", res.Message)
	}
	if timedOut, _ := scriptData(t, res)["timedOut"].(bool); timedOut {
		t.Errorf("回显设备应让脚本提前完成，不应超时")
	}
	data := scriptData(t, res)
	counts, _ := data["ruleFireCounts"].([]int)
	if len(counts) != 1 || counts[0] != 1 {
		t.Errorf("预期匹配规则触发 1 次，实际 %v", counts)
	}
	recv, _ := data["received"].(string)
	if !strings.Contains(recv, "PING") {
		t.Errorf("回放数据应含设备回显 PING，实际: %q", recv)
	}
	// 设备侧实际收到：PING 与 PONG 都写进了 responder 日志
	waitForLogContent(t, logPath, "PING", "PONG")
}
