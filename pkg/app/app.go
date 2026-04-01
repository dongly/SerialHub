// Package app 提供统一的 App 生命周期管理
package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/internal/service"
	"github.com/yourname/serialhub/pkg/bridge"
	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/serial"
	"github.com/yourname/serialhub/pkg/telnet"
	"github.com/yourname/serialhub/pkg/tray"
)

// App 统一管理所有组件生命周期
type App struct {
	Version string
	Serial  *serial.SerialManager
	Telnet  *telnet.TelnetServer
	Bridge  *bridge.DataBridge
	Buffer  *buffer.DataBuffer
	Tray    *tray.TrayManager
	Config  *config.Config
	Service *service.ServiceManager
	cancel  context.CancelFunc
}

// NewApp 创建 App 实例
func NewApp(version string, cfg *config.Config) *App {
	buf := buffer.NewDataBuffer()
	serialMgr := serial.NewSerialManager(cfg.Serial)
	telnetSrv := telnet.NewTelnetServer()

	return &App{
		Version: version,
		Serial:  serialMgr,
		Telnet:  telnetSrv,
		Buffer:  buf,
		Config:  cfg,
		Service: service.NewServiceManager(),
	}
}

// RunMCP 运行 MCP stdio 模式
func (a *App) RunMCP(ctx context.Context) error {
	logrus.Infof("[SerialHub] SerialHub v%s 启动中...", a.Version)
	logrus.Info("[SerialHub] 运行模式: mcp stdio")

	// TODO: 实现 MCP stdio 模式
	return nil
}

// RunServe 运行 serve 模式（HTTP+SSE + Telnet + Tray）
func (a *App) RunServe(ctx context.Context, telnetPort, mcpPort int) error {
	logrus.Infof("[SerialHub] SerialHub v%s 启动中...", a.Version)
	logrus.Info("[SerialHub] 运行模式: serve")

	// 创建 DataBridge
	a.Bridge = bridge.NewDataBridge(a.Serial, a.Telnet, a.Buffer, bridge.BridgeOptions{
		EnableTelnet: true,
		EnableMCP:    true,
		DebugLog:     a.Config.Debug,
	})

	// 启动 Telnet
	if err := a.Telnet.Start(ctx, telnetPort); err != nil {
		return err
	}
	logrus.Infof("[SerialHub] Telnet 服务已启动，端口: %d", telnetPort)

	// 启动 DataBridge
	a.Bridge.Start(ctx)
	logrus.Info("[SerialHub] 数据桥接已启动")

	// 写入服务状态
	if err := a.Service.WriteStatus(mcpPort); err != nil {
		logrus.Warnf("[SerialHub] 写入服务状态失败: %v", err)
	}

	// 创建系统托盘
	a.Tray = tray.NewTrayManager(a.Serial, a.Config, telnetPort, mcpPort, a.Version)

	// 启动系统托盘（在主 goroutine）
	go a.Tray.Run(ctx)

	logrus.Infof("[SerialHub] MCP HTTP 服务已启动: http://localhost:%d", mcpPort)

	return nil
}

// Close 按序关闭所有组件
func (a *App) Close() {
	logrus.Info("[SerialHub] 正在关闭...")

	// 按序关闭：Bridge → Telnet → Serial → Tray → Service
	if a.Bridge != nil {
		a.Bridge.Stop()
	}

	if a.Telnet != nil {
		a.Telnet.Stop()
	}

	a.Serial.Close()

	if a.Tray != nil {
		// Tray 会在主 goroutine 退出时自动关闭
	}

	if a.Service != nil {
		a.Service.ClearStatus()
	}

	if a.cancel != nil {
		a.cancel()
	}

	logrus.Info("[SerialHub] 已关闭")
}

// SetupSignalHandler 设置信号处理
func (a *App) SetupSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logrus.Infof("[SerialHub] 收到 %s 信号，正在关闭...", sig)
		a.Close()
		os.Exit(0)
	}()
}
