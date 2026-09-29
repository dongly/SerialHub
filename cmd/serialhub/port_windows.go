//go:build windows

package main

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// isAddrInUse 判断 listen 错误是否为端口占用。
// Windows 的 socket 绑定冲突返回 WSAEADDRINUSE(10048)，
// 与 syscall.EADDRINUSE 不是同一错误值，必须分别判断。
func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, windows.WSAEADDRINUSE)
}
