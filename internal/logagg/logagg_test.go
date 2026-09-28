package logagg

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// captureHook 捕获全局 logrus 的日志条目，供断言。
type captureHook struct {
	mu      sync.Mutex
	entries []*logrus.Entry
}

func (h *captureHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *captureHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, entry)
	return nil
}

func (h *captureHook) all() []*logrus.Entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*logrus.Entry(nil), h.entries...)
}

// waitEntries 轮询等待 hook 收到至少 n 条日志（聚合 flush 由 timer 异步触发）。
func (h *captureHook) waitEntries(t *testing.T, n int, timeout time.Duration) []*logrus.Entry {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if es := h.all(); len(es) >= n {
			return es
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待 %d 条日志超时（%s），实际 %d 条", n, timeout, len(h.all()))
	return nil
}

// setup 注册捕获 hook 并启用 Trace 级别，返回清理函数。
func setup(t *testing.T) *captureHook {
	t.Helper()
	h := &captureHook{}
	std := logrus.StandardLogger()
	// 保存 hook 快照（AddHook 会就地修改 Hooks map，须深拷贝）
	origHooks := make(logrus.LevelHooks, len(std.Hooks))
	for lvl, hs := range std.Hooks {
		origHooks[lvl] = append([]logrus.Hook(nil), hs...)
	}
	logrus.AddHook(h)
	origLevel := logrus.GetLevel()
	logrus.SetLevel(logrus.TraceLevel)

	// 隔离全局 registry 与时间窗，避免用例间串扰
	origRegistry := registry
	origWindow := window
	registry = map[string]*aggregator{}
	window = 30 * time.Millisecond

	t.Cleanup(func() {
		std.ReplaceHooks(origHooks)
		logrus.SetLevel(origLevel)
		registry = origRegistry
		window = origWindow
	})
	return h
}

func TestAdd_AggregatesWithinWindow(t *testing.T) {
	h := setup(t)

	Add("t1", []byte("ab"))
	Add("t1", []byte("cd"))
	Add("t1", []byte("ef"))

	entries := h.waitEntries(t, 1, 500*time.Millisecond)
	if len(entries) != 1 {
		t.Fatalf("期望窗口内 3 条数据聚合为 1 条日志，实际 %d 条", len(entries))
	}
	msg := entries[0].Message
	if !strings.Contains(msg, "3 条 / 6 字节") {
		t.Errorf("日志应含计数与字节数，实际: %s", msg)
	}
	if !strings.Contains(msg, `"abcdef"`) {
		t.Errorf("日志应含聚合内容 abcdef，实际: %s", msg)
	}
	if entries[0].Level != logrus.TraceLevel {
		t.Errorf("聚合日志应为 Trace 级，实际 %v", entries[0].Level)
	}
}

func TestAdd_TruncatesWhenFull(t *testing.T) {
	h := setup(t)

	big := strings.Repeat("x", 600)
	Add("t2", []byte(big)) // 超过展示上限：到窗口结束才落盘，总字节如实计数

	entries := h.waitEntries(t, 1, 500*time.Millisecond)
	if len(entries) != 1 {
		t.Fatalf("单次超限数据应聚合为 1 条日志，实际 %d 条", len(entries))
	}
	msg := entries[0].Message
	if !strings.Contains(msg, "1 条 / 600 字节（截断）") {
		t.Errorf("截断窗口应标注总字节数与截断标记，实际: %s", msg)
	}
	// 展示内容不超过 512 字节（%q 转义后 512 个字符）
	if !strings.Contains(msg, strings.Repeat("x", 512)) {
		t.Errorf("展示内容应恰好 512 字节，实际: %s", msg)
	}
	if strings.Contains(msg, strings.Repeat("x", 513)) {
		t.Errorf("展示内容超过 512 字节未截断，实际: %s", msg)
	}
}

// TestAdd_HighThroughputSingleWindow 高吞吐不得退化为逐条输出：
// 窗口内多次大包提交，只输出一条聚合日志，条数与总字节数如实统计。
func TestAdd_HighThroughputSingleWindow(t *testing.T) {
	h := setup(t)

	pkt := []byte(strings.Repeat("y", 1024))
	for i := 0; i < 20; i++ {
		Add("thru", pkt) // 20 × 1KB 在同一窗口内
	}

	entries := h.waitEntries(t, 1, 500*time.Millisecond)
	if len(entries) != 1 {
		t.Fatalf("窗口内 20 次大包应聚合为 1 条日志，实际 %d 条", len(entries))
	}
	msg := entries[0].Message
	if !strings.Contains(msg, "20 条 / 20480 字节（截断）") {
		t.Errorf("应统计全部条数与字节并标注截断，实际: %s", msg)
	}
	if !strings.Contains(msg, strings.Repeat("y", 512)) {
		t.Errorf("展示内容应恰好 512 字节，实际: %s", msg)
	}
	if strings.Contains(msg, strings.Repeat("y", 513)) {
		t.Errorf("展示内容超过 512 字节未截断，实际: %s", msg)
	}
}

func TestAdd_DisabledWithoutTraceLevel(t *testing.T) {
	h := setup(t)
	logrus.SetLevel(logrus.InfoLevel) // 覆盖 setup 的 Trace：模拟未开启 --log-data

	Add("t3", []byte("data"))
	time.Sleep(2 * window) // 超过窗口时长，确认无任何输出

	if es := h.all(); len(es) != 0 {
		t.Errorf("Trace 未开启时 Add 不应产生日志，实际 %d 条", len(es))
	}
}

func TestFlushAll(t *testing.T) {
	h := setup(t)

	Add("t4", []byte("partial")) // 未攒满，等 timer 才会 flush
	FlushAll()                   // 立即收尾

	entries := h.waitEntries(t, 1, 100*time.Millisecond)
	if !strings.Contains(entries[0].Message, `"partial"`) {
		t.Errorf("FlushAll 应立即落盘未满窗口，实际: %s", entries[0].Message)
	}
}

func TestFlushAll_EmptyNoop(t *testing.T) {
	h := setup(t)

	FlushAll()

	if es := h.all(); len(es) != 0 {
		t.Errorf("无数据时 FlushAll 不应产生日志，实际 %d 条", len(es))
	}
}

// TestStaleTimerDoesNotFlushNewWindow 窗口被 FlushAll 提前落盘后，
// 其 timer 到期（旧代号）不得把随后的新窗口误落盘。
func TestStaleTimerDoesNotFlushNewWindow(t *testing.T) {
	h := setup(t)
	const tag = "t5"
	a := get(tag)

	// 窗口 1：小数据开启窗口（timer T1 挂起），读取其窗口代号
	Add(tag, []byte("old"))
	a.mu.Lock()
	oldEpoch := a.epoch
	a.mu.Unlock()

	// 窗口 1 被 FlushAll 提前落盘（epoch 递增作废 T1）
	FlushAll()
	if es := h.all(); len(es) != 1 || !strings.Contains(es[0].Message, `"old"`) {
		t.Fatalf("FlushAll 应落盘窗口 1，实际: %v", es)
	}

	// 窗口 2：新数据开启新窗口（timer T2，新代号）
	Add(tag, []byte("new"))

	// 确定性模拟旧 timer T1 此刻到期：旧代号作废，不得产生新日志
	a.flushAt(oldEpoch)
	if es := h.all(); len(es) != 1 {
		t.Fatalf("旧 timer 到期不应把窗口 2 落盘，实际 %d 条", len(es))
	}

	// 新窗口由 FlushAll（或 T2 自身到期）正常落盘
	FlushAll()
	es := h.all()
	if len(es) != 2 || !strings.Contains(es[1].Message, `"new"`) {
		t.Fatalf("窗口 2 应正常落盘为第 2 条，实际: %v", es)
	}
}

func TestDifferentTagsIndependent(t *testing.T) {
	h := setup(t)

	Add("t6a", []byte("AAA"))
	Add("t6b", []byte("BBB"))

	entries := h.waitEntries(t, 2, 500*time.Millisecond)
	if len(entries) != 2 {
		t.Fatalf("不同来源应各自独立落盘，实际 %d 条", len(entries))
	}
	msgs := entries[0].Message + "|" + entries[1].Message
	if !strings.Contains(msgs, "AAA") || !strings.Contains(msgs, "BBB") {
		t.Errorf("两条日志应分别包含各自来源的数据，实际: %s", msgs)
	}
}
