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

// lockOffset 是加锁字节在文件中的偏移。选一个远超元数据长度的远端偏移：
// 读取方只会读取文件开头的一小段 JSON 元数据，不会触及该字节；否则
// 像 PowerShell Get-Content 这类按大缓冲区（数 KB）读取的工具会读到被锁
// 区域并报 ERROR_LOCK_VIOLATION（表现为“文件正被另一进程使用”）。
const lockOffset = 0x7FFFFFFF

// tryLock 尝试对远端 1 字节区域加排他锁（非阻塞）：成功返回 nil；
// 已被其他进程持有时返回错误。LockFileEx 区域锁由内核管理，进程退出
// （含崩溃/被杀）时自动释放。
func tryLock(f *os.File) error {
	var overlapped windows.Overlapped
	overlapped.Offset = lockOffset
	return windows.LockFileEx(windows.Handle(f.Fd()), lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0, &overlapped)
}
