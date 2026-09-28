//go:build !windows

package instance

import (
	"errors"
	"os"
	"syscall"
)

func lockBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

// tryLock 尝试对整个文件加排他锁（非阻塞）：成功立即返回 nil；
// 已被其他进程（或其他 open file description）持有时返回错误。
// flock 锁由内核管理，进程退出（含崩溃/被杀）时自动释放。
func tryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
