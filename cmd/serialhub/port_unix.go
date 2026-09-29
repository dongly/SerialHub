//go:build !windows

package main

import (
	"errors"
	"syscall"
)

// isAddrInUse 判断 listen 错误是否为端口占用（Unix: EADDRINUSE）。
func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
