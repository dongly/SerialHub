//go:build !windows

package tray

func HideConsole()           {}
func ShowConsole()           {}
func DisableCloseButton()    {}
func IsConsoleVisible() bool { return true }
