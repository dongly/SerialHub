// Package tools provides MCP tools for serial port operations.
package tools

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/dongly/serialhub/internal/buffer"
	"github.com/dongly/serialhub/pkg/serial"
)

// scriptMu 保证全局同时只有一个脚本在运行：多个剧本对同一串口的
// 发送顺序会互相踩踏（剧本 A 的应答被剧本 B 的匹配抢先消费）。
var scriptMu sync.Mutex

// 定时/匹配扫描的内部上限与默认值
const (
	scriptWindowMax     = 8 * 1024                                       // 匹配滚动窗口上限（跨 chunk 拼接）
	scriptRecvMax       = 1024 * 1024                                    // 回放数据上限（保留最新；截断时返回 receivedTruncated=true）
	scriptScheduleMax   = 100000                                         // 展开后定时触发条目上限（防 count 膨胀 OOM）
	scriptMaxAtMsValue  = int64(math.MaxInt64 / int64(time.Millisecond)) // 触发时刻偏移上限（ms，防止 ×time.Millisecond 溢出成负时长）
	defaultMatchDelayMs = 5                                              // 匹配写未指定 delayMs 时的默认延时（ms）；显式 0=命中立即写
)

// TimedWrite 定时写条目。
// 统一模型：单发 = atMs 一次；周期 = atMs 起、每 intervalMs 一次、共 count 次
// （触发时刻 atMs + i*intervalMs，首拍在 atMs）。
type TimedWrite struct {
	AtMs       int64  `json:"atMs,omitempty"`       // 相对脚本启动的毫秒偏移，默认 0
	IntervalMs int64  `json:"intervalMs,omitempty"` // 0 = 单发
	Count      int    `json:"count,omitempty"`      // 周期次数；IntervalMs>0 且 <=0 时默认 1
	Data       string `json:"data"`
	AddNewline *bool  `json:"addNewline,omitempty"` // 默认 true（同 serial_write）
}

// MatchRule 匹配写规则：接收数据匹配 pattern（Go 正则）时自动发送。
// 默认命中一次即失效；Repeat 持续生效，MaxCount>0 时限次。
// DelayMs 未指定时默认 defaultMatchDelayMs（5ms）延时再写；显式 0=命中立即写；
// 指针区分"未填"与"填 0"（同 AddNewline）。每次命中独立计时。
type MatchRule struct {
	Pattern    string `json:"pattern"`
	Data       string `json:"data"`
	AddNewline *bool  `json:"addNewline,omitempty"` // 默认 true
	Repeat     bool   `json:"repeat,omitempty"`
	MaxCount   int    `json:"maxCount,omitempty"` // Repeat=true 且 >0 时的最大触发次数
	DelayMs    *int64 `json:"delayMs,omitempty"`  // 触发后延时再写（ms）；nil=默认 5ms，0=立即
}

// ScriptInput serial_script 工具输入。
type ScriptInput struct {
	TimeoutMs  int          `json:"timeoutMs"` // 必填，范围由 [script] 配置决定
	Writes     []TimedWrite `json:"writes,omitempty"`
	Matches    []MatchRule  `json:"matches,omitempty"`
	ReturnData *bool        `json:"returnData,omitempty"` // 默认 true
}

// matchState 单条匹配规则的执行状态。
type matchState struct {
	rule     MatchRule
	re       *regexp.Regexp
	delayMs  int64 // 规范化后的延时（构造时解析：nil=defaultMatchDelayMs，非 nil 取值）
	fired    int   // 已触发次数
	scanFrom int64 // 该规则已扫描到的绝对偏移（防重复触发）
	done     bool  // 触发次数耗尽（单发命中 / repeat 达 maxCount）
}

// scriptFireItem 展开后的定时触发项（按 offsetMs 升序）。
// 只存偏移不存绝对时刻：start 在展开完成后才取，展开耗时不吞定时预算。
type scriptFireItem struct {
	offsetMs   int64 // 相对 start 的触发偏移（ms），fireAt = start + offsetMs
	writeIdx   int   // input.Writes 下标
	occurrence int
	payload    []byte
}

// scriptInbox 观察者→脚本事件循环的无界队列：
// Append 回调不能阻塞（串口读取路径），也不能丢数据（破坏匹配语义），
// 故入队后以容量 1 的信号 channel 唤醒循环，由循环侧统一排水。
type scriptInbox struct {
	mu  sync.Mutex
	q   []inboxItem
	sig chan struct{}
}

// inboxItem 记录 chunk 及其到达时刻（由 Append 观察点捕获）：
// 启动前（展开期间）或超时后才到达的数据不参与匹配也不计入回放。
type inboxItem struct {
	data []byte
	at   time.Time
}

// delayedWrite 已命中但延时未到的写条目（小顶堆按 fireAt 升序）。
// 命中即计 ruleFireCounts 与 trigger（含序号）；实际写出由事件循环到点执行。
type delayedWrite struct {
	fireAt  time.Time // 命中时刻 + delayMs
	rule    int       // input.Matches 下标
	payload []byte
}

// delayedWriteHeap container/heap 小顶堆（按 fireAt）。
type delayedWriteHeap []delayedWrite

func (h delayedWriteHeap) Len() int            { return len(h) }
func (h delayedWriteHeap) Less(i, j int) bool  { return h[i].fireAt.Before(h[j].fireAt) }
func (h delayedWriteHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *delayedWriteHeap) Push(x interface{}) { *h = append(*h, x.(delayedWrite)) }
func (h *delayedWriteHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func (in *scriptInbox) push(chunk []byte, at time.Time) {
	in.mu.Lock()
	in.q = append(in.q, inboxItem{data: chunk, at: at})
	in.mu.Unlock()
	select {
	case in.sig <- struct{}{}:
	default:
	}
}

func (in *scriptInbox) drain() []inboxItem {
	in.mu.Lock()
	q := in.q
	in.q = nil
	in.mu.Unlock()
	return q
}

// ExecuteSerialScript 执行一次串口交互剧本（阻塞至完成/超时/取消/断连）。
// ScriptLimits serial_script 的 timeoutMs 允许范围（来自 [script] 配置）。
type ScriptLimits struct {
	MinMs int
	MaxMs int
}

// ExecuteSerialScript 执行一次串口交互剧本（定时写 + 匹配写），阻塞至完成/超时。
// ctx 取消即中止（失败）；断连（含断连后重连，按连接代次判定）中止失败；
// 全触发提前成功；超时成功并附 timedOut 与未触发规则清单。
func ExecuteSerialScript(ctx context.Context, sm *serial.SerialManager, buf *buffer.DataBuffer, input ScriptInput, limits ScriptLimits) ToolResult {
	if sm == nil {
		return ToolResult{Success: false, Message: "串口管理器未初始化"}
	}
	if buf == nil {
		return ToolResult{Success: false, Message: "数据缓冲未初始化"}
	}
	// 全局单实例：第二个脚本直接拒绝
	if !scriptMu.TryLock() {
		return ToolResult{Success: false, Message: "已有脚本运行中"}
	}
	defer scriptMu.Unlock()

	// ---- 校验 ----
	if input.TimeoutMs <= 0 {
		return ToolResult{Success: false, Message: "timeoutMs 必填"}
	}
	if limits.MinMs <= 0 || limits.MaxMs < limits.MinMs {
		return ToolResult{Success: false, Message: "脚本超时范围配置无效"}
	}
	if input.TimeoutMs < limits.MinMs || input.TimeoutMs > limits.MaxMs {
		return ToolResult{
			Success: false,
			Message: fmt.Sprintf("timeoutMs 超出允许范围 [%d, %d]: %d", limits.MinMs, limits.MaxMs, input.TimeoutMs),
		}
	}
	if len(input.Writes) == 0 && len(input.Matches) == 0 {
		return ToolResult{Success: false, Message: "剧本为空：writes 与 matches 至少配置一项"}
	}
	if !sm.IsConnected() {
		return ToolResult{Success: false, Message: "串口未连接"}
	}

	returnData := input.ReturnData == nil || *input.ReturnData

	// 编译匹配规则
	states := make([]*matchState, 0, len(input.Matches))
	for i, m := range input.Matches {
		if m.Pattern == "" {
			return ToolResult{Success: false, Message: fmt.Sprintf("匹配规则 #%d: pattern 不能为空", i)}
		}
		re, err := regexp.Compile(m.Pattern)
		if err != nil {
			return ToolResult{Success: false, Message: fmt.Sprintf("匹配规则 #%d: 正则无效: %v", i, err)}
		}
		// 延时到点时刻 = 命中时刻+delayMs 必须可表示（同定时写溢出防护）
		var delayMs int64 = defaultMatchDelayMs
		if m.DelayMs != nil {
			if *m.DelayMs < 0 {
				return ToolResult{Success: false, Message: fmt.Sprintf("匹配规则 #%d: delayMs 不能为负", i)}
			}
			if *m.DelayMs > scriptMaxAtMsValue {
				return ToolResult{Success: false, Message: fmt.Sprintf("匹配规则 #%d: delayMs 超出可表示范围", i)}
			}
			delayMs = *m.DelayMs
		}
		states = append(states, &matchState{rule: m, re: re, delayMs: delayMs})
	}

	// 先订阅再展开：展开大剧本（最多 10 万条）耗时期间到达的数据
	// 必须进入 inbox，之后按到达时刻与 start 比较决定是否参与匹配
	inbox := &scriptInbox{sig: make(chan struct{}, 1)}
	unsub := buf.Subscribe(inbox.push)
	defer unsub()

	connGen := sm.ConnectionGen() // 连接代次：断连/重连/改配置都会递增
	var schedule []scriptFireItem
	newPayload := func(data string, nl *bool) []byte {
		if nl == nil || *nl {
			return []byte(data + "\n")
		}
		return []byte(data)
	}
	for wi, w := range input.Writes {
		if w.AtMs < 0 {
			return ToolResult{Success: false, Message: fmt.Sprintf("定时写 #%d: atMs 不能为负", wi)}
		}
		if w.IntervalMs < 0 {
			return ToolResult{Success: false, Message: fmt.Sprintf("定时写 #%d: intervalMs 不能为负", wi)}
		}
		if w.Count < 0 {
			return ToolResult{Success: false, Message: fmt.Sprintf("定时写 #%d: count 不能为负", wi)}
		}
		// 触发时刻 start+at*Millisecond(ns) 必须可表示：大值溢出成负时长会让远期写提前发送
		if w.AtMs > scriptMaxAtMsValue {
			return ToolResult{Success: false, Message: fmt.Sprintf("定时写 #%d: atMs 超出可表示范围", wi)}
		}
		if w.IntervalMs > scriptMaxAtMsValue {
			return ToolResult{Success: false, Message: fmt.Sprintf("定时写 #%d: intervalMs 超出可表示范围", wi)}
		}
		// 展开条目累计上限：以"本条将展开 > 剩余额度"预检（不可写成
		// len+want 求和：want 接近 MaxInt 时求和溢出为负会绕过检查）
		want := 1
		if w.IntervalMs > 0 && w.Count > 0 {
			want = w.Count
		}
		if want > scriptScheduleMax-len(schedule) {
			return ToolResult{Success: false, Message: fmt.Sprintf("定时写 #%d: 展开条目超出上限 %d", wi, scriptScheduleMax)}
		}
		payload := newPayload(w.Data, w.AddNewline)
		if w.IntervalMs > 0 {
			count := w.Count
			if count == 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				if int64(i) > (scriptMaxAtMsValue-w.AtMs)/w.IntervalMs {
					return ToolResult{Success: false, Message: fmt.Sprintf("定时写 #%d: 触发时刻超出可表示范围", wi)}
				}
				at := w.AtMs + int64(i)*w.IntervalMs
				schedule = append(schedule, scriptFireItem{
					offsetMs:   at,
					writeIdx:   wi,
					occurrence: i,
					payload:    payload,
				})
			}
		} else {
			schedule = append(schedule, scriptFireItem{
				offsetMs: w.AtMs,
				writeIdx: wi,
				payload:  payload,
			})
		}
	}
	sort.Slice(schedule, func(i, j int) bool { return schedule[i].offsetMs < schedule[j].offsetMs })

	// 展开完成才算脚本启动：定时偏移、超时截止、数据参与范围都以 start 为基准
	start := time.Now()
	// fireAt(s)：第 s 条定时项的绝对触发时刻
	fireAt := func(s int) time.Time {
		return start.Add(time.Duration(schedule[s].offsetMs) * time.Millisecond)
	}
	deadline := start.Add(time.Duration(input.TimeoutMs) * time.Millisecond)
	connTick := time.NewTicker(100 * time.Millisecond)
	defer connTick.Stop()

	var (
		window   []byte // 匹配滚动窗口
		absStart int64  // window[0] 的绝对偏移
		total    int64  // 启动后累计接收字节
		received []byte // 回放数据（保留最新 scriptRecvMax）
		triggers []map[string]any
		schIdx   int // 已触发的定时项数

		// 延时写队列（delayMs>0 的命中）：命中即入堆，到点写出
		delayed       delayedWriteHeap
		delayedQueued int // 累计入队数
		delayedFired  int // 累计写出数（超时/取消时 queued-fired = 丢弃数）
	)

	elapsedMs := func() int64 { return time.Since(start).Milliseconds() }

	buildData := func(timedOut bool) map[string]any {
		pendingRules := make([]int, 0)
		for i, st := range states {
			if st.fired == 0 {
				pendingRules = append(pendingRules, i)
			}
		}
		ruleCounts := make([]int, len(states))
		for i, st := range states {
			ruleCounts[i] = st.fired
		}
		if triggers == nil {
			triggers = []map[string]any{}
		}
		d := map[string]any{
			"timedOut":             timedOut,
			"triggers":             triggers,
			"ruleFireCounts":       ruleCounts,
			"pendingRules":         pendingRules,
			"writesFired":          schIdx,
			"writesTotal":          len(schedule),
			"delayedWritesQueued":  delayedQueued,
			"delayedWritesFired":   delayedFired,
			"delayedWritesPending": delayedQueued - delayedFired,
			"receivedBytes":        total,
		}
		if returnData {
			d["received"] = string(received)
		}
		// 回放仅保留最新 scriptRecvMax 字节；总接收量见 receivedBytes
		d["receivedTruncated"] = total > int64(len(received))
		return d
	}

	// process 处理新到达数据：滚动窗口 + 按声明顺序扫描全部规则。
	// 超时（deadline）之后才到达的数据直接跳过：不匹配、不触发写、不计入回放。
	// 返回非 nil 表示写入失败（剧本中止）。
	process := func(items []inboxItem) error {
		for _, it := range items {
			// 启动前（展开期间已订阅但 start 未取）或超时后才到达的数据：
			// 不匹配、不触发写、不计入回放
			if it.at.Before(start) || it.at.After(deadline) {
				continue
			}
			c := it.data
			total += int64(len(c))

			received = append(received, c...)
			if len(received) > scriptRecvMax {
				received = append(received[:0], received[len(received)-scriptRecvMax:]...)
			}

			window = append(window, c...)
			// 先扫描后裁剪：单次到达的数据全文必被规则扫过，
			// 裁剪只限制"跨 chunk 回看历史"不超过 8KB
			for si, st := range states {
				if st.done {
					continue
				}
				// 全窗口扫描 + 绝对偏移过滤：锚点（如 ^）始终相对窗口起点，
				// 已触发过的命中（起点 < scanFrom）被过滤，不会重复触发
				for _, loc := range st.re.FindAllIndex(window, -1) {
					if loc[1] == loc[0] {
						continue // 零宽命中（如 "^"）不触发，避免停滞/无限循环
					}
					absS := absStart + int64(loc[0])
					absE := absStart + int64(loc[1])
					if absS < st.scanFrom {
						continue
					}
					if ctx.Err() != nil {
						return ctx.Err() // 批量命中期间被取消：不再继续发送
					}
					payload := newPayload(st.rule.Data, st.rule.AddNewline)
					occ := st.fired
					// 触发记录 = 命中事实（延时写可能因超时/取消未发出，此处如实记录）
					triggers = append(triggers, map[string]any{
						"type":       "match",
						"rule":       si,
						"occurrence": occ,
						"atMs":       elapsedMs(),
						"data":       string(payload),
						"matched":    string(window[loc[0]:loc[1]]),
						"delayMs":    st.delayMs,
					})
					st.fired++
					if !st.rule.Repeat || (st.rule.MaxCount > 0 && st.fired >= st.rule.MaxCount) {
						st.done = true
					}
					st.scanFrom = absE
					if st.delayMs > 0 {
						// 延时写：入堆由事件循环到点写出（与定时写同校验路径）
						if delayed.Len() >= scriptScheduleMax {
							return fmt.Errorf("延时写队列超出上限 %d", scriptScheduleMax)
						}
						heap.Push(&delayed, delayedWrite{
							fireAt:  time.Now().Add(time.Duration(st.delayMs) * time.Millisecond),
							rule:    si,
							payload: payload,
						})
						delayedQueued++
						if st.done {
							break
						}
						continue
					}
					// 立即写
					if _, err := sm.WriteIfSameGen(connGen, payload); err != nil {
						if errors.Is(err, serial.ErrStaleConnection) {
							return err // 代次已变：交由事件循环判定断连中止
						}
						return fmt.Errorf("匹配规则 #%d 写入失败: %w", si, err)
					}
					if st.done {
						break
					}
				}
			}

			// 扫描完毕再裁剪窗口到 8KB 上限（滚动淘汰最旧数据）
			if len(window) > scriptWindowMax {
				drop := len(window) - scriptWindowMax
				copy(window, window[drop:])
				window = window[:scriptWindowMax]
				absStart += int64(drop)
			}
		}
		return nil
	}

	complete := func() bool {
		if schIdx < len(schedule) {
			return false
		}
		if delayed.Len() > 0 {
			return false // 延时写未全部发出
		}
		for _, st := range states {
			if st.fired == 0 {
				return false
			}
		}
		return true
	}

	// cancelResult 统一构造"脚本已取消"的失败结果
	cancelResult := func() ToolResult {
		return ToolResult{Success: false, Message: "脚本已取消", Data: buildData(false)}
	}
	// staleAbort 统一构造"断连（含断连后快速重连）中止"的失败结果
	staleAbort := func() ToolResult {
		return ToolResult{Success: false, Message: "串口断连，脚本中止", Data: buildData(false)}
	}
	// completeResult 统一构造"全触发完成"的成功结果（含超时排水后补完的路径）
	// 返回前再查一次取消与连接：末次写入后的取消/断连（含快速重连）不得被成功掩盖
	completeResult := func() ToolResult {
		if ctx.Err() != nil {
			return cancelResult()
		}
		if !sm.IsConnected() || sm.ConnectionGen() != connGen {
			return staleAbort()
		}
		msg := fmt.Sprintf("脚本执行完成: 定时写 %d/%d, 匹配触发 %d/%d", schIdx, len(schedule), len(states), len(states))
		if delayedQueued > 0 {
			msg += fmt.Sprintf(", 延时写 %d/%d", delayedFired, delayedQueued)
		}
		return ToolResult{
			Success: true,
			Message: msg,
			Data:    buildData(false),
		}
	}
	// abortResult 统一构造"脚本中止"的失败结果
	abortResult := func(timedOut bool, cause string) ToolResult {
		return ToolResult{
			Success: false,
			Message: fmt.Sprintf("脚本中止: %v", cause),
			Data:    buildData(timedOut),
		}
	}
	// onProcessErr 归类 process 返回的错误：取消 > 代次失效 > 其他写入失败
	onProcessErr := func(err error, timedOut bool) ToolResult {
		if ctx.Err() != nil {
			return cancelResult()
		}
		if errors.Is(err, serial.ErrStaleConnection) {
			return staleAbort()
		}
		return abortResult(timedOut, err.Error())
	}

	// ---- 事件循环 ----
	for {
		// ctx 取消优先于一切（含预取消：不得先发写再成功返回）
		if ctx.Err() != nil {
			return cancelResult()
		}
		// 每轮开始都核对连接有效性（覆盖 100ms tick 以外的所有唤醒路径：
		// 定时器到点、新数据、取消），断连或代次变化（断连后快速重连）立即中止
		if !sm.IsConnected() || sm.ConnectionGen() != connGen {
			return staleAbort()
		}

		// 非阻塞排水：不遗漏 select 唤醒间隙已入队的数据
		if chunks := inbox.drain(); len(chunks) > 0 {
			if err := process(chunks); err != nil {
				return onProcessErr(err, false)
			}
		}

		if complete() {
			return completeResult()
		}

		if time.Now().After(deadline) {
			// b.mu 屏障：截止前已进入 Append 临界区的数据，其到达时刻捕获与
			// 观察者入队同临界区原子完成；此处取一次锁确保它们全部入队后才排水，
			// 不给"数据到达在截止前、入队在排水后"留窗口
			_ = buf.Length()
			// 超时前最后消化一次已到达数据（可能恰好补完最后一环）
			if chunks := inbox.drain(); len(chunks) > 0 {
				if err := process(chunks); err != nil {
					return onProcessErr(err, true)
				}
			}
			if complete() {
				return completeResult()
			}
			// 超时成功返回前最后核对：取消/断连不得被超时成功掩盖
			if ctx.Err() != nil {
				return cancelResult()
			}
			if !sm.IsConnected() || sm.ConnectionGen() != connGen {
				return staleAbort()
			}
			pending := make([]string, 0)
			for i, st := range states {
				if st.fired == 0 {
					pending = append(pending, fmt.Sprintf("#%d", i))
				}
			}
			msg := fmt.Sprintf("脚本超时: 定时写 %d/%d, 匹配触发 %d/%d", schIdx, len(schedule), firedMatches(states), len(states))
			if len(pending) > 0 {
				msg += ", 未触发规则: " + joinPendings(pending)
			}
			return ToolResult{Success: true, Message: msg, Data: buildData(true)}
		}

		// 到点的定时写（每轮复核截止时间：批次跨截止时剩余条目留待超时路径丢弃）
		for schIdx < len(schedule) && !fireAt(schIdx).After(time.Now()) && !time.Now().After(deadline) {
			if ctx.Err() != nil {
				return cancelResult() // 批量到期期间被取消：不再继续发送
			}
			item := schedule[schIdx]
			// 校验+写入原子完成：写入瞬间连接被替换（断连/快速重连）时拒绝写向新连接
			if _, err := sm.WriteIfSameGen(connGen, item.payload); err != nil {
				if errors.Is(err, serial.ErrStaleConnection) {
					return staleAbort()
				}
				return ToolResult{
					Success: false,
					Message: fmt.Sprintf("定时写 #%d 写入失败: %v", item.writeIdx, err),
					Data:    buildData(false),
				}
			}
			triggers = append(triggers, map[string]any{
				"type":       "timed",
				"rule":       item.writeIdx,
				"occurrence": item.occurrence,
				"atMs":       elapsedMs(),
				"data":       string(item.payload),
			})
			schIdx++
		}
		// 到点的延时写（命中已记录 trigger，此处只负责实际发出）。
		// 每轮复核截止时间：微决策"到点若已超时→丢弃不写"——批次跨截止时
		// 剩余条目不再发出，留在堆中由超时路径以 delayedWritesPending 上报
		for delayed.Len() > 0 && !delayed[0].fireAt.After(time.Now()) && !time.Now().After(deadline) {
			if ctx.Err() != nil {
				return cancelResult() // 批量到期期间被取消：不再继续发送
			}
			item := heap.Pop(&delayed).(delayedWrite)
			if _, err := sm.WriteIfSameGen(connGen, item.payload); err != nil {
				if errors.Is(err, serial.ErrStaleConnection) {
					return staleAbort()
				}
				return ToolResult{
					Success: false,
					Message: fmt.Sprintf("延时写 (匹配规则 #%d) 写入失败: %v", item.rule, err),
					Data:    buildData(false),
				}
			}
			delayedFired++
		}
		if complete() {
			return completeResult()
		}

		// 等待：ctx 取消 / 新数据 / 下一定时点（定时写与延时写取最早） / 断连检查
		var timerC <-chan time.Time
		var timer *time.Timer
		var nextWake time.Time
		hasWake := false
		if schIdx < len(schedule) {
			nextWake = fireAt(schIdx)
			hasWake = true
		}
		if delayed.Len() > 0 && (!hasWake || delayed[0].fireAt.Before(nextWake)) {
			nextWake = delayed[0].fireAt
			hasWake = true
		}
		if hasWake {
			d := time.Until(nextWake)
			if d < 0 {
				d = 0
			}
			timer = time.NewTimer(d)
			timerC = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return cancelResult()
		case <-inbox.sig:
		case <-timerC:
		case <-connTick.C:
			// 纯唤醒：连接有效性核对在下一轮循环顶部统一执行
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func firedMatches(states []*matchState) int {
	n := 0
	for _, st := range states {
		if st.fired > 0 {
			n++
		}
	}
	return n
}

func joinPendings(items []string) string {
	s := ""
	for i, it := range items {
		if i > 0 {
			s += " "
		}
		s += it
	}
	return s
}
