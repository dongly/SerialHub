//go:build !windows

package mcp

import (
	"errors"
	"syscall"
)

// isBrokenPipe 识别类 Unix 系统上的管道断开：对端读端已关闭后写回会得到
// EPIPE（通常包在 *os.PathError 里）。
func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
