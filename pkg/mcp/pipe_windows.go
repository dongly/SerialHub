//go:build windows

package mcp

import (
	"errors"
	"syscall"
)

// errPipeNoData 对应 Win32 ERROR_NO_DATA(232)，错误文案为
// "The pipe is being closed."：管道读端句柄已关闭后再写回即得此错。
// Go 的 syscall 包未导出该常量，故在此显式声明。
const errPipeNoData = syscall.Errno(232)

// isBrokenPipe 识别 Windows 上的管道断开。MCP 客户端关闭 stdio 读端后，
// 代理写回得到的是 232，Go 不会把它映射成 io.ErrClosedPipe，必须显式识别，
// 否则客户端的正常关闭会被当成异常退出（违反「stdio 关闭属正常退出」）。
func isBrokenPipe(err error) bool {
	return errors.Is(err, errPipeNoData)
}
