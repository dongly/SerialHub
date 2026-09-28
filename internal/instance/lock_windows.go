//go:build windows

package instance

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lockBusy(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }

const (
	lockfileExclusiveLock   = 0x2 // LOCKFILE_EXCLUSIVE_LOCK
	lockfileFailImmediately = 0x1 // LOCKFILE_FAIL_IMMEDIATELY
)

// tryLock 尝试对文件起始 1 字节区域加排他锁（非阻塞）：成功返回 nil；
// 已被其他进程持有时返回错误。LockFileEx 区域锁由内核管理，进程退出
// （含崩溃/被杀）时自动释放。
func tryLock(f *os.File) error {
	var overlapped windows.Overlapped
	// 锁元数据区以外的字节：Windows 区域锁会阻止其他进程读取被锁区域。
	overlapped.Offset = 4096
	return windows.LockFileEx(windows.Handle(f.Fd()), lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0, &overlapped)
}
