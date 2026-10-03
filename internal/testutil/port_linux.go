//go:build linux

package testutil

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Linux pty ioctl（asm-generic，各架构取值一致）：
// TIOCSPTLCK 解锁从端命名（值 0），TIOCGPTN 取从端编号。
const (
	ioctlTIOCSPTLCK = 0x40045431
	ioctlTIOCGPTN   = 0x80045430
)

// ptyMaster 持有 pty 主端：从端 /dev/pts/N 只有在主端打开期间才可持续
// 打开与读写；由测试进程退出时 OS 自动回收，不产生孤儿进程。
var ptyMaster *os.File

// probeTestPort 在 /dev/ptmx 上创建一对 pty 并返回从端路径 /dev/pts/N。
func probeTestPort() (string, bool, string) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return "", false, fmt.Sprintf("打开 /dev/ptmx 失败: %v", err)
	}
	unlock := int32(0)
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), ioctlTIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		m.Close()
		return "", false, fmt.Sprintf("解锁 pty 失败: %v", errno)
	}
	var n uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), ioctlTIOCGPTN, uintptr(unsafe.Pointer(&n))); errno != 0 {
		m.Close()
		return "", false, fmt.Sprintf("获取 pty 编号失败: %v", errno)
	}
	ptyMaster = m
	return fmt.Sprintf("/dev/pts/%d", n), true, ""
}
