package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/sirupsen/logrus"
	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/internal/service"
	"github.com/yourname/serialhub/pkg/bridge"
	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/mcp"
	"github.com/yourname/serialhub/pkg/serial"
	"github.com/yourname/serialhub/pkg/tray"
	"github.com/yourname/serialhub/pkg/web"
)

func runServe(cmd *cobra.Command, args []string) error {
	if err := service.EnsureSingleInstance(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	defer service.ReleaseSingleInstance()

	cfg := loadConfig()
	setupLogger(cfg)
	logrus.Infof("[SerialHub] SerialHub v%s 启动中...", appVersion)
	logrus.Info("[SerialHub] 运行模式: serve")

	buf := buffer.NewDataBuffer()
	serialCfg := configToSerialConfig(&cfg.Serial)
	sm, _ := serial.NewSerialManager(serialCfg)
	if sm == nil {
		serialCfg = &serial.Config{Port: "", BaudRate: 115200, DataBits: 8, Parity: "none", StopBits: 1}
		sm, _ = serial.NewSerialManager(serialCfg)
	}

	enableTray := runtime.GOOS == "windows"
	logrus.Debugf("[SerialHub] 托盘检查: GOOS=%s, enableTray=%v", runtime.GOOS, enableTray)

	if enableTray {
		return runWithTray(cfg, sm, buf)
	}
	return runWithoutTray(cfg, sm, buf)
}

func runWithTray(cfg *config.Config, sm *serial.SerialManager, buf *buffer.DataBuffer) error {
	logrus.Debug("[SerialHub] 系统托盘模式已启用")

	if minimized {
		tray.DisableCloseButton()
		tray.HideConsole()
	}

	trayMgr := tray.NewTrayManager(sm, cfg, host, mcpPort, appVersion, minimized)
	saveConfigFunc := createSaveConfigFunc(cfg)
	trayMgr.SetOnConfigChanged(func(port string, baudRate int, dataBits int, parity string, stopBits float64) {
		saveConfigFunc(&serial.Config{
			Port:     port,
			BaudRate: baudRate,
			DataBits: dataBits,
			Parity:   parity,
			StopBits: float32(stopBits),
		})
	})
	sm.SetConfigChangeHandler(saveConfigFunc)

	var wsSrv *web.WebSocketServer
	var cancelFunc context.CancelFunc

	sm.SetEventHandler(createEventHandler(sm, trayMgr, wsSrv))

	trayMgr.SetOnReady(func() {
		_, cancel := context.WithCancel(context.Background())
		cancelFunc = cancel

		if err := sm.Connect(); err != nil {
			logrus.Debugf("[SerialHub] 自动连接串口失败: %v", err)
		} else {
			logrus.Infof("[SerialHub] 已自动连接串口: %s", sm.GetConfig().String())
		}

		wsSrv, err := web.NewWebSocketServer(host, mcpPort, func() string {
			if sm.IsConnected() {
				return sm.GetConfig().String()
			}
			return ""
		})
		if err != nil {
			logrus.Errorf("[SerialHub] 创建 WebSocket 服务失败: %v", err)
			return
		}

		startServices(sm, wsSrv, buf)

		addr := fmt.Sprintf("%s:%d", host, mcpPort)
		logrus.Infof("[SerialHub] MCP HTTP 服务: http://%s/mcp", addr)
		logrus.Infof("[SerialHub] 健康检查: http://%s/health", addr)
		logrus.Infof("[SerialHub] WebSocket 端口: %d", mcpPort)
	})

	trayMgr.SetOnExit(func() {
		if cancelFunc != nil {
			cancelFunc()
		}
	})

	trayMgr.Run(context.Background())

	logrus.Info("[SerialHub] 正在关闭...")
	closeLogger()
	return nil
}

func runWithoutTray(cfg *config.Config, sm *serial.SerialManager, buf *buffer.DataBuffer) error {
	sm.SetConfigChangeHandler(createSaveConfigFunc(cfg))

	wsSrv, err := web.NewWebSocketServer(host, mcpPort, func() string {
		if sm.IsConnected() {
			return sm.GetConfig().String()
		}
		return ""
	})
	if err != nil {
		return fmt.Errorf("创建 WebSocket 服务失败: %w", err)
	}

	sm.SetEventHandler(createSerialEventHandler(wsSrv))

	startServices(sm, wsSrv, buf)

	addr := fmt.Sprintf("%s:%d", host, mcpPort)
	logrus.Infof("[SerialHub] MCP HTTP 服务: http://%s/mcp", addr)
	logrus.Infof("[SerialHub] 健康检查: http://%s/health", addr)
	logrus.Infof("[SerialHub] WebSocket 端口: %d", mcpPort)
	logrus.Info("[SerialHub] 服务已启动，按 Ctrl+C 退出")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logrus.Info("[SerialHub] 正在关闭...")
	closeLogger()
	return nil
}

func createSaveConfigFunc(cfg *config.Config) func(*serial.Config) {
	return func(serialCfg *serial.Config) {
		cfg.Serial.Port = serialCfg.Port
		cfg.Serial.BaudRate = serialCfg.BaudRate
		cfg.Serial.DataBits = serialCfg.DataBits
		cfg.Serial.Parity = serialCfg.Parity
		cfg.Serial.StopBits = float64(serialCfg.StopBits)
		if configPath != "" {
			if err := config.Save(configPath, cfg); err != nil {
				logrus.Warnf("[SerialHub] 保存配置失败: %v", err)
			}
		}
	}
}

func createEventHandler(sm *serial.SerialManager, trayMgr *tray.TrayManager, wsSrv *web.WebSocketServer) func(serial.Event) {
	return func(event serial.Event) {
		trayMgr.UpdateSerialStatus()

		if wsSrv != nil {
			var msg string
			switch event.Type {
			case serial.EventConnected:
				msg = fmt.Sprintf("\r\n[SerialHub] 串口已连接: %s\r\n", event.Port)
			case serial.EventDisconnected:
				msg = fmt.Sprintf("\r\n[SerialHub] 串口已断开\r\n")
			case serial.EventError:
				msg = fmt.Sprintf("\r\n[SerialHub] 串口错误: %s\r\n", event.Message)
			}
			if msg != "" {
				wsSrv.Broadcast([]byte(msg))
			}
		}
	}
}

func createSerialEventHandler(wsSrv *web.WebSocketServer) func(serial.Event) {
	return func(event serial.Event) {
		var msg string
		switch event.Type {
		case serial.EventConnected:
			msg = fmt.Sprintf("\r\n[SerialHub] 串口已连接: %s\r\n", event.Port)
		case serial.EventDisconnected:
			msg = fmt.Sprintf("\r\n[SerialHub] 串口已断开\r\n")
		case serial.EventError:
			msg = fmt.Sprintf("\r\n[SerialHub] 串口错误: %s\r\n", event.Message)
		}
		if msg != "" {
			wsSrv.Broadcast([]byte(msg))
		}
	}
}

func startServices(sm *serial.SerialManager, wsSrv *web.WebSocketServer, buf *buffer.DataBuffer) {
	bridgeSrv, err := bridge.NewDataBridge(sm, wsSrv, buf)
	if err != nil {
		logrus.Warnf("[SerialHub] 创建数据桥接失败: %v", err)
	} else {
		bridgeSrv.SetCommandHandler(createCommandHandler(sm))
		bridgeSrv.Start()
		logrus.Info("[SerialHub] 数据桥接已启动")
	}

	mcpSrv, err := mcp.NewMCPServer(sm, buf, wsSrv)
	if err != nil {
		logrus.Errorf("[SerialHub] 创建 MCP 服务失败: %v", err)
		return
	}
	if err := mcpSrv.RegisterTools(); err != nil {
		logrus.Errorf("[SerialHub] 注册 MCP 工具失败: %v", err)
		return
	}

	addr := fmt.Sprintf("%s:%d", host, mcpPort)
	if _, err := mcpSrv.StartHTTPServer(addr); err != nil {
		logrus.Errorf("[SerialHub] 启动 HTTP 服务失败: %v", err)
		return
	}
}