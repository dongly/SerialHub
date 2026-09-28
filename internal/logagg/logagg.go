// Package logagg 聚合高频数据内容日志（串口/WebSocket 收发数据的 %q 内容）：
// 同一数据源在一个时间窗内（默认 500ms）的多次写入合并为一条 Trace 级日志，
// 避免逐字节刷屏。窗口只按时间关闭：展示内容最多保留 512 字节，超出部分
// 仅计入条数与总字节数并标注截断，不会提前结束窗口（高吞吐下仍每窗口一条）。
// 仅当日志级别开启 Trace（--log-data）时工作，否则 Add 直接短路返回。
package logagg

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// 数据内容日志的预定义来源标签。
const (
	TagSerialRead    = "串口读取"
	TagSerialWrite   = "串口写入"
	TagWebSocketRecv = "WebSocket 接收"
)

// 时间窗与展示上限（测试可注入修改，生产只读）。
var (
	window          = 500 * time.Millisecond
	maxContentBytes = 512 // 单条日志展示的内容上限（字节），超出仅计数并标注截断
)

type aggregator struct {
	tag string

	mu        sync.Mutex
	buf       []byte      // 窗口内展示内容（最多 maxContentBytes 字节）
	count     int         // 窗口内 Add 次数
	total     int         // 窗口内累计字节数（不受展示截断影响）
	truncated bool        // 展示内容是否因达到上限被截断
	timer     *time.Timer // 当前窗口定时器，nil 表示无活动窗口
	epoch     int         // 窗口代号：flush 时递增，作废在途 timer 回调
}

// Add 追加一段数据到指定来源的聚合窗口。
// 窗口自首条数据起算 window 时长，期间只累计不落盘；到时整窗输出一条。
// 并发安全；日志级别未开 Trace 时为零开销短路。
func Add(tag string, data []byte) {
	if len(data) == 0 || !logrus.IsLevelEnabled(logrus.TraceLevel) {
		return
	}
	get(tag).add(data)
}

// FlushAll 立即落盘所有未 flush 的窗口（进程退出前收尾，避免最后一窗丢失）。
func FlushAll() {
	globalMu.Lock()
	aggs := make([]*aggregator, 0, len(registry))
	for _, a := range registry {
		aggs = append(aggs, a)
	}
	globalMu.Unlock()
	for _, a := range aggs {
		a.flushNow()
	}
}

var (
	globalMu sync.Mutex
	registry = map[string]*aggregator{}
)

func get(tag string) *aggregator {
	globalMu.Lock()
	defer globalMu.Unlock()
	a, ok := registry[tag]
	if !ok {
		a = &aggregator{tag: tag}
		registry[tag] = a
	}
	return a
}

func (a *aggregator) add(data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.count++
	a.total += len(data)
	// 展示额度只追加未满部分：大包不再整段复制，超出部分仅计数并标记截断
	if room := maxContentBytes - len(a.buf); room > 0 {
		if len(data) <= room {
			a.buf = append(a.buf, data...)
		} else {
			a.buf = append(a.buf, data[:room]...)
			a.truncated = true
		}
	} else {
		a.truncated = true
	}
	if a.timer == nil {
		// 开新窗口：自首条数据起 window 后整窗 flush（epoch 由 flush 递增，
		// 此处捕获当前值，供到期回调校验窗口是否仍有效）
		epoch := a.epoch
		a.timer = time.AfterFunc(window, func() { a.flushAt(epoch) })
	}
}

// flushAt 校验窗口代号后 flush：窗口被 FlushAll 提前落盘后，其 timer
// 到期时代号已不匹配，直接作废，不影响随后开启的新窗口。
func (a *aggregator) flushAt(epoch int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.epoch != epoch {
		return
	}
	a.flushLocked()
}

func (a *aggregator) flushNow() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.timer != nil {
		// Stop 返回 false 表示回调已触发：其 flushAt 会因 epoch 不匹配被作废
		a.timer.Stop()
	}
	a.flushLocked()
}

func (a *aggregator) flushLocked() {
	a.timer = nil
	if a.count == 0 {
		return
	}

	if a.truncated {
		logrus.Tracef("[SerialHub] 数据日志[%s] %d 条 / %d 字节（截断）: %q",
			a.tag, a.count, a.total, string(a.buf))
	} else {
		logrus.Tracef("[SerialHub] 数据日志[%s] %d 条 / %d 字节: %q",
			a.tag, a.count, a.total, string(a.buf))
	}

	a.buf = a.buf[:0]
	a.count = 0
	a.total = 0
	a.truncated = false
	a.epoch++
}
