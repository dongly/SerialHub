// Package tray 提供系统托盘功能
package tray

import (
	"context"
	"embed"
	"fmt"

	"github.com/getlantern/systray"
	"github.com/sirupsen/logrus"

	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/serial"
)

// TrayState 托盘状态类型
type TrayState string

const (
	TrayIdle      TrayState = "idle"
	TrayConnected TrayState = "connected"
	TrayError     TrayState = "error"
)

// TrayManager 系统托盘管理器
type TrayManager struct {
	serial     *serial.SerialManager
	config     *config.Config
	telnetPort int
	mcpPort    int
	version    string
	state      TrayState
}

//go:embed assets/*.png
var iconFS embed.FS

// NewTrayManager 创建托盘管理器
func NewTrayManager(serialMgr *serial.SerialManager, cfg *config.Config, telnetPort, mcpPort int, version string) *TrayManager {
	return &TrayManager{
		serial:     serialMgr,
		config:     cfg,
		telnetPort: telnetPort,
		mcpPort:    mcpPort,
		version:    version,
		state:      TrayIdle,
	}
}

// Run 启动系统托盘（必须在主 goroutine 调用）
func (t *TrayManager) Run(ctx context.Context) {
	systray.Run(func() {
		t.onReady()
	}, func() {
		t.onExit()
	})
}

// onReady 托盘就绪回调
func (t *TrayManager) onReady() {
	logrus.Info("[SerialHub] 系统托盘已启动")

	// 设置图标
	systray.SetIcon(t.getIcon())
	systray.SetTitle("SerialHub")
	systray.SetTooltip(fmt.Sprintf("SerialHub v%s", t.version))

	// 创建菜单
	mSerial := systray.AddMenuItem("📡 串口连接", "串口连接")
	mConfig := systray.AddMenuItem("⚙️ 串口参数", "串口参数")
	mPorts := systray.AddMenuItem("🌐 服务端口", "服务端口")
	systray.AddSeparator()
	mVersion := systray.AddMenuItem(fmt.Sprintf("📋 版本 %s", t.version), "版本")
	mVersion.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("❌ 退出", "退出 SerialHub")

	// 处理菜单点击
	go func() {
		for {
			select {
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			case <-mSerial.ClickedCh:
				t.toggleSerial()
			case <-mConfig.ClickedCh:
				t.showConfig()
			case <-mPorts.ClickedCh:
				t.showPorts()
			}
		}
	}()
}

// onExit 托盘退出回调
func (t *TrayManager) onExit() {
	logrus.Info("[SerialHub] 系统托盘已退出")
}

// UpdateState 更新托盘状态
func (t *TrayManager) UpdateState(state TrayState) {
	t.state = state
	systray.SetIcon(t.getIcon())
	logrus.Infof("[SerialHub] 托盘状态更新: %s", state)
}

// UpdateSerialStatus 根据串口状态更新托盘
func (t *TrayManager) UpdateSerialStatus() {
	if t.serial.IsConnected() {
		t.UpdateState(TrayConnected)
		systray.SetTooltip(fmt.Sprintf("SerialHub - 已连接 %s", t.serial.CurrentPort()))
	} else {
		t.UpdateState(TrayIdle)
		systray.SetTooltip("SerialHub - 未连接")
	}
}

// getIcon 根据状态返回图标
func (t *TrayManager) getIcon() []byte {
	// 简化实现，返回空图标
	// 实际应该从 embed.FS 加载
	return []byte{}
}

// toggleSerial 切换串口连接
func (t *TrayManager) toggleSerial() {
	if t.serial.IsConnected() {
		if err := t.serial.Disconnect(); err != nil {
			logrus.Errorf("[SerialHub] 断开串口失败: %v", err)
		} else {
			logrus.Infof("[SerialHub] 从托盘断开串口")
		}
	} else {
		if err := t.serial.Connect(); err != nil {
			logrus.Errorf("[SerialHub] 连接串口失败: %v", err)
		} else {
			logrus.Infof("[SerialHub] 从托盘连接串口 %s", t.config.Serial.Port)
		}
	}
	t.UpdateSerialStatus()
}

// showConfig 显示配置菜单
func (t *TrayManager) showConfig() {
	logrus.Debug("[SerialHub] 显示串口参数菜单")
}

// showPorts 显示端口菜单
func (t *TrayManager) showPorts() {
	logrus.Debugf("[SerialHub] Telnet端口: %d, MCP端口: %d", t.telnetPort, t.mcpPort)
}
