//go:build windows

package service

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	procCreateMutex = modkernel32.NewProc("CreateMutexW")
	procCloseHandle = modkernel32.NewProc("CloseHandle")
)

const (
	ERROR_ALREADY_EXISTS = 183
)

type SingleInstanceLock struct {
	mutexName string
	handle    syscall.Handle
}

func NewSingleInstanceLock() *SingleInstanceLock {
	return &SingleInstanceLock{
		mutexName: `Global\SerialHub_Single_Instance`,
	}
}

func (sil *SingleInstanceLock) TryLock() error {
	namePtr, _ := syscall.UTF16PtrFromString(sil.mutexName)

	handle, _, err := procCreateMutex.Call(
		0,
		0,
		uintptr(unsafe.Pointer(namePtr)),
	)

	if errno, ok := err.(syscall.Errno); ok && errno != 0 && errno != ERROR_ALREADY_EXISTS {
		return fmt.Errorf("创建互斥量失败: %w", err)
	}

	if handle == 0 {
		return fmt.Errorf("创建互斥量失败: 返回空句柄")
	}

	if errno, ok := err.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		procCloseHandle.Call(handle)
		return fmt.Errorf("SerialHub 已在运行中")
	}

	sil.handle = syscall.Handle(handle)
	return nil
}

func (sil *SingleInstanceLock) Unlock() error {
	if sil.handle != 0 {
		procCloseHandle.Call(uintptr(sil.handle))
		sil.handle = 0
	}
	return nil
}
