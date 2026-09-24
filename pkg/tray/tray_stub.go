//go:build !windows

// Package tray 非 Windows 平台（WSL/Linux/macOS）的空实现。
// 这些平台没有系统托盘，serve 模式走命令行路径（runWithoutTray），
// 本文件仅保证包可编译，且对外 API 与 Windows 版保持一致。
package tray

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/serial"
)

type TrayState string

const (
	TrayIdle      TrayState = "idle"
	TrayConnected TrayState = "connected"
	TrayError     TrayState = "error"
)

type OnReadyFunc func()
type OnConfigChangedFunc func(port string, baudRate int, dataBits int, parity string, stopBits float64)

type TrayManager struct {
	serial          *serial.SerialManager
	config          *config.Config
	host            string
	mcpPort         int
	version         string
	state           TrayState
	quitChan        chan struct{}
	readyCallback   OnReadyFunc
	exitCallback    func()
	onConfigChanged OnConfigChangedFunc
}

func NewTrayManager(serialMgr *serial.SerialManager, cfg *config.Config, host string, mcpPort int, version string, _ bool) *TrayManager {
	return &TrayManager{
		serial:   serialMgr,
		config:   cfg,
		host:     host,
		mcpPort:  mcpPort,
		version:  version,
		state:    TrayIdle,
		quitChan: make(chan struct{}),
	}
}

// SetOnReady 设置就绪回调（非 Windows 平台不会触发）
func (t *TrayManager) SetOnReady(fn OnReadyFunc) {
	t.readyCallback = fn
}

// SetOnExit 设置退出回调
func (t *TrayManager) SetOnExit(fn func()) {
	t.exitCallback = fn
}

// SetOnConfigChanged 设置配置变更回调（非 Windows 平台不会触发）
func (t *TrayManager) SetOnConfigChanged(fn OnConfigChangedFunc) {
	t.onConfigChanged = fn
}

// Run 阻塞直到收到 ctx 取消或 SIGINT/SIGTERM 信号，语义上对齐
// Windows 版的 systray.Run（阻塞直到退出）。正常情况下非 Windows
// 平台不会走到这里（serve.go 已按 GOOS 分流），仅为兜底。
func (t *TrayManager) Run(ctx context.Context) {
	logrus.Debug("[SerialHub] 非 Windows 平台无系统托盘，等待退出信号")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-ctx.Done():
	case <-sigChan:
	}
	t.onExit()
}

func (t *TrayManager) onExit() {
	logrus.Info("[SerialHub] 已退出")
	if t.exitCallback != nil {
		t.exitCallback()
	}
	close(t.quitChan)
}

func (t *TrayManager) QuitChan() <-chan struct{} {
	return t.quitChan
}

// UpdateState 更新状态（非 Windows 平台仅记录日志）
func (t *TrayManager) UpdateState(state TrayState) {
	t.state = state
	logrus.Debugf("[SerialHub] 状态更新: %s", state)
}

// UpdateSerialStatus 根据串口连接状态更新（非 Windows 平台仅记录日志）
func (t *TrayManager) UpdateSerialStatus() {
	connected := t.serial != nil && t.serial.IsConnected()
	newState := TrayIdle
	if connected {
		newState = TrayConnected
	}
	if t.state != newState {
		t.UpdateState(newState)
	}
}
