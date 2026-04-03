//go:build windows

package tray

import (
	"syscall"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")

	user32         = syscall.NewLazyDLL("user32.dll")
	procShowWindow = user32.NewProc("ShowWindow")
)

const (
	SW_HIDE = 0
	SW_SHOW = 5
)

func GetConsoleWindow() syscall.Handle {
	ret, _, _ := procGetConsoleWindow.Call()
	return syscall.Handle(ret)
}

func ShowWindow(hwnd syscall.Handle, cmdShow int) bool {
	ret, _, _ := procShowWindow.Call(
		uintptr(hwnd),
		uintptr(cmdShow),
	)
	return ret != 0
}

func HideConsole() {
	hwnd := GetConsoleWindow()
	if hwnd != 0 {
		ShowWindow(hwnd, SW_HIDE)
	}
}

func ShowConsole() {
	hwnd := GetConsoleWindow()
	if hwnd != 0 {
		ShowWindow(hwnd, SW_SHOW)
	}
}
