package testutil

import "io"

// NopWriteCloser 包装 io.Writer 使其满足 io.WriteCloser：Close 为 no-op。
// 用于把不该被真正关闭的写端（如子进程管道、io.Discard）交给需要 Close 的接口。
type NopWriteCloser struct{ io.Writer }

func (NopWriteCloser) Close() error { return nil }
