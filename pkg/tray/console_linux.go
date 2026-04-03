//go:build linux

package tray

// HideConsole 在 Linux 上隐藏控制台（当前为 no-op，Linux 通常通过终端启动）
func HideConsole() {
	// Linux 通常通过终端启动，隐藏控制台不是标准做法
	// 如需实现，可考虑使用 xdotool 或 wmctrl 等工具
}

// ShowConsole 在 Linux 上显示控制台（当前为 no-op）
func ShowConsole() {
	// Linux 通常通过终端启动，显示控制台不是标准做法
}
