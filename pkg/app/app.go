package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/pkg/bridge"
	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/serial"
	"github.com/yourname/serialhub/pkg/telnet"
	"github.com/yourname/serialhub/pkg/tray"
)

type App struct {
	Version string
	Serial  *serial.SerialManager
	Telnet  *telnet.TelnetServer
	Bridge  *bridge.DataBridge
	Buffer  *buffer.DataBuffer
	Tray    *tray.TrayManager
	Config  *config.Config
	cancel  context.CancelFunc
}

func NewApp(version string, cfg *config.Config) (*App, error) {
	buf := buffer.NewDataBuffer()

	var sm *serial.SerialManager
	if cfg.Serial.Port != "" {
		serialCfg := configToSerialConfig(&cfg.Serial)
		var err error
		sm, err = serial.NewSerialManager(serialCfg)
		if err != nil {
			return nil, err
		}
	}

	return &App{
		Version: version,
		Serial:  sm,
		Buffer:  buf,
		Config:  cfg,
	}, nil
}

func configToSerialConfig(cfg *config.SerialConfig) *serial.Config {
	return &serial.Config{
		Port:     cfg.Port,
		BaudRate: cfg.BaudRate,
		DataBits: cfg.DataBits,
		Parity:   cfg.Parity,
		StopBits: float32(cfg.StopBits),
	}
}

func (a *App) RunServe(ctx context.Context, telnetPort, mcpPort int, noTray bool) error {
	logrus.Infof("[SerialHub] SerialHub v%s 启动中...", a.Version)
	logrus.Info("[SerialHub] 运行模式: serve")

	host := "127.0.0.1"

	var err error
	a.Telnet, err = telnet.NewTelnetServer(host, telnetPort)
	if err != nil {
		return fmt.Errorf("创建 Telnet 服务失败: %w", err)
	}

	if a.Serial != nil && a.Buffer != nil {
		a.Bridge, err = bridge.NewDataBridge(a.Serial, a.Telnet, a.Buffer)
		if err != nil {
			return fmt.Errorf("创建数据桥接失败: %w", err)
		}
	}

	if err := a.Telnet.Start(); err != nil {
		return fmt.Errorf("启动 Telnet 服务失败: %w", err)
	}
	logrus.Infof("[SerialHub] Telnet 服务已启动，端口: %d", telnetPort)

	if a.Bridge != nil {
		a.Bridge.Start()
		logrus.Info("[SerialHub] 数据桥接已启动")
	}

	if a.Serial != nil && !noTray {
		a.Tray = tray.NewTrayManager(a.Serial, a.Config, telnetPort, mcpPort, a.Version)
		go a.Tray.Run(ctx)
	}

	logrus.Infof("[SerialHub] MCP HTTP 服务: http://%s:%d/mcp", host, mcpPort)
	logrus.Infof("[SerialHub] 健康检查: http://%s:%d/health", host, mcpPort)
	logrus.Infof("[SerialHub] Telnet 端口: %d", telnetPort)
	logrus.Info("[SerialHub] 服务已启动，按 Ctrl+C 退出")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logrus.Info("[SerialHub] 正在关闭...")
	a.Close()
	return nil
}

func (a *App) Close() {
	if a.Bridge != nil {
		a.Bridge.Stop()
	}

	if a.Telnet != nil {
		a.Telnet.Stop()
	}

	if a.Serial != nil {
		a.Serial.Close()
	}

	if a.cancel != nil {
		a.cancel()
	}

	logrus.Info("[SerialHub] 已关闭")
}
