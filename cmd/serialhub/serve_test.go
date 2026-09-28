package main

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// blockingWriter 模拟日志输出链卡死：Write 永远阻塞直到 release 关闭。
type blockingWriter struct {
	release chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.release
	return len(p), nil
}

// TestFlushBoundedLog_日志输出阻塞时按时返回 验证停机收尾的有界性：
// 日志 writer 卡死（stdout 背压/文件锁等价物）时，flushBoundedLog 不得
// 永久阻塞控制线程，应在 ~500ms 超时后返回（防止未来把同步日志移回
// 控制线程导致强退路径到不了 os.Exit）。
func TestFlushBoundedLog_日志输出阻塞时按时返回(t *testing.T) {
	release := make(chan struct{})
	blocked := &blockingWriter{release: release}

	std := logrus.StandardLogger()
	origOut := std.Out
	logrus.SetOutput(blocked)
	defer func() {
		// 先释放卡住的 Write（其持有 logger 锁），再恢复输出
		close(release)
		logrus.SetOutput(origOut)
	}()

	start := time.Now()
	flushBoundedLog(logrus.InfoLevel, "停机通知（应被阻塞但有界返回）")
	elapsed := time.Since(start)

	if elapsed >= time.Second {
		t.Errorf("日志输出阻塞时 flushBoundedLog 应在 ~500ms 返回，实际 %s", elapsed)
	}
	if elapsed < 450*time.Millisecond {
		t.Errorf("flushBoundedLog 过早返回（%s）：应等到 500ms 超时才放弃", elapsed)
	}
}
