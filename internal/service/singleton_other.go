//go:build !windows

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type SingleInstanceLock struct {
	lockFile *os.File
	lockPath string
}

func NewSingleInstanceLock() *SingleInstanceLock {
	return &SingleInstanceLock{
		lockPath: filepath.Join(os.TempDir(), "serialhub.lock"),
	}
}

func (sil *SingleInstanceLock) TryLock() error {
	f, err := os.OpenFile(sil.lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("无法创建锁文件: %w", err)
	}
	sil.lockFile = f

	flock := syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Whence: 0,
		Start:  0,
		Len:    1,
		Pid:    int32(os.Getpid()),
	}

	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &flock); err != nil {
		f.Close()
		sil.lockFile = nil
		return fmt.Errorf("SerialHub 已在运行中")
	}

	f.Truncate(0)
	fmt.Fprintf(f, "%d", os.Getpid())
	f.Sync()

	return nil
}

func (sil *SingleInstanceLock) Unlock() error {
	if sil.lockFile == nil {
		return nil
	}

	flock := syscall.Flock_t{
		Type:   syscall.F_UNLCK,
		Whence: 0,
		Start:  0,
		Len:    1,
	}

	syscall.FcntlFlock(sil.lockFile.Fd(), syscall.F_SETLK, &flock)
	sil.lockFile.Close()
	sil.lockFile = nil
	return nil
}
