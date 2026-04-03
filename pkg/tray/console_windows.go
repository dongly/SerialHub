//go:build windows

package tray

import (
	"syscall"

	"github.com/sirupsen/logrus"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")

	user32                  = syscall.NewLazyDLL("user32.dll")
	procShowWindow          = user32.NewProc("ShowWindow")
	procGetSystemMenu       = user32.NewProc("GetSystemMenu")
	procRemoveMenu          = user32.NewProc("RemoveMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
)

const (
	SW_HIDE     = 0
	SW_SHOW     = 5
	SW_MINIMIZE = 6
	SW_RESTORE  = 9

	SC_CLOSE     = 0xF060
	MF_BYCOMMAND = 0x00000000
)

func GetConsoleWindow() syscall.Handle {
	ret, _, _ := procGetConsoleWindow.Call()
	return syscall.Handle(ret)
}

func ShowWindow(hwnd syscall.Handle, cmdShow int) bool {
	ret, _, _ := procShowWindow.Call(uintptr(hwnd), uintptr(cmdShow))
	return ret != 0
}

func SetForegroundWindow(hwnd syscall.Handle) bool {
	ret, _, _ := procSetForegroundWindow.Call(uintptr(hwnd))
	return ret != 0
}

func IsWindowVisible(hwnd syscall.Handle) bool {
	ret, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	return ret != 0
}

func GetSystemMenu(hwnd syscall.Handle, bRevert bool) syscall.Handle {
	var revert uintptr
	if bRevert {
		revert = 1
	}
	ret, _, _ := procGetSystemMenu.Call(uintptr(hwnd), revert)
	return syscall.Handle(ret)
}

func RemoveMenu(hMenu syscall.Handle, nPosition uint, uFlags uint) bool {
	ret, _, _ := procRemoveMenu.Call(uintptr(hMenu), uintptr(nPosition), uintptr(uFlags))
	return ret != 0
}

func DisableCloseButton() {
	hwnd := GetConsoleWindow()
	if hwnd == 0 {
		return
	}
	hMenu := GetSystemMenu(hwnd, false)
	if hMenu == 0 {
		return
	}
	if RemoveMenu(hMenu, SC_CLOSE, MF_BYCOMMAND) {
		logrus.Debug("[SerialHub] 已禁用控制台关闭按钮")
	}
}

func IsConsoleVisible() bool {
	hwnd := GetConsoleWindow()
	if hwnd == 0 {
		return false
	}
	return IsWindowVisible(hwnd)
}

func HideConsole() {
	hwnd := GetConsoleWindow()
	if hwnd != 0 {
		ShowWindow(hwnd, SW_HIDE)
		logrus.Debug("[SerialHub] 控制台已隐藏")
	}
}

func ShowConsole() {
	hwnd := GetConsoleWindow()
	if hwnd != 0 {
		ShowWindow(hwnd, SW_RESTORE)
		ShowWindow(hwnd, SW_SHOW)
		SetForegroundWindow(hwnd)
		logrus.Debug("[SerialHub] 控制台已显示并激活")
	}
}
