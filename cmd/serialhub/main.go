package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/internal/service"
	"github.com/yourname/serialhub/pkg/bridge"
	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/mcp"
	"github.com/yourname/serialhub/pkg/serial"
	"github.com/yourname/serialhub/pkg/tray"
	"github.com/yourname/serialhub/pkg/web"
)

var (
	version    = "0.1.0"
	serialPort string
	baudRate   int
	configPath string
	debugMode  bool
	wsPort     int
	mcpPort    int
	host       string
	noTray     bool
	minimized  bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "serialhub",
		Short: "SerialHub - 串口与网络连接的双向桥接器",
		Long:  "SerialHub 将 MCU 串口数据同时转发到 WebSocket（人工监视）和 MCP（AI 工具程序化访问）。",
		RunE:  runServe,
		Args:  cobra.NoArgs,
	}

	rootCmd.PersistentFlags().StringVarP(&serialPort, "serial-port", "p", "", "串口名（如 COM9 或 /dev/ttyUSB0）")
	rootCmd.PersistentFlags().IntVarP(&baudRate, "baud-rate", "b", 115200, "波特率")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "配置文件路径")
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "D", false, "启用调试模式")
	rootCmd.Flags().IntVarP(&wsPort, "ws-port", "t", 2323, "WebSocket 服务端口")
	rootCmd.Flags().IntVarP(&mcpPort, "mcp-port", "m", 5000, "MCP HTTP 服务端口")
	rootCmd.Flags().StringVar(&host, "host", "127.0.0.1", "监听地址")
	rootCmd.Flags().BoolVar(&noTray, "no-tray", false, "禁用系统托盘")
	rootCmd.Flags().BoolVar(&minimized, "minimized", false, "由脚本启动，窗口最小化")

	rootCmd.Version = version
	rootCmd.SetVersionTemplate(fmt.Sprintf("SerialHub v%s\n", version))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	if err := service.EnsureSingleInstance(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	defer service.ReleaseSingleInstance()

	cfg := loadConfig()
	setupLogger(cfg)
	logrus.Infof("[SerialHub] SerialHub v%s 启动中...", version)
	logrus.Info("[SerialHub] 运行模式: serve")

	buf := buffer.NewDataBuffer()
	serialCfg := configToSerialConfig(&cfg.Serial)
	// 始终创建 SerialManager，即使端口为空，以便 DataBridge 可以正常工作
	sm, _ := serial.NewSerialManager(serialCfg)
	if sm == nil {
		// 如果配置转换失败，使用默认配置创建
		serialCfg = &serial.Config{Port: "", BaudRate: 115200, DataBits: 8, Parity: "none", StopBits: 1}
		sm, _ = serial.NewSerialManager(serialCfg)
	}

	enableTray := !noTray && runtime.GOOS == "windows"
	logrus.Debugf("[SerialHub] 托盘检查: noTray=%v, GOOS=%s, enableTray=%v", noTray, runtime.GOOS, enableTray)

	if enableTray {
		return runWithTray(cfg, sm, buf, enableTray)
	}
	return runWithoutTray(cfg, sm, buf)
}

func runWithTray(cfg *config.Config, sm *serial.SerialManager, buf *buffer.DataBuffer, _ bool) error {
	logrus.Debug("[SerialHub] 系统托盘模式已启用")

	// 由 start.ps1 启动时（minimized=true）：禁用关闭键，隐藏窗口，显示"显示/隐藏窗口"菜单
	// 直接双击启动时（minimized=false）：不禁用关闭键，窗口正常显示，不显示"显示/隐藏窗口"菜单
	if minimized {
		tray.DisableCloseButton()
		tray.HideConsole()
	}

	trayMgr := tray.NewTrayManager(sm, cfg, host, wsPort, mcpPort, version, minimized)

	// 统一配置保存逻辑
	saveConfigFunc := func(serialCfg *serial.Config) {
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

	trayMgr.SetOnConfigChanged(func(port string, baudRate int, dataBits int, parity string, stopBits float64) {
		saveConfigFunc(&serial.Config{
			Port:     port,
			BaudRate: baudRate,
			DataBits: dataBits,
			Parity:   parity,
			StopBits: float32(stopBits),
		})
	})

	// 设置 SerialManager 配置变更回调
	sm.SetConfigChangeHandler(saveConfigFunc)

	sm.SetEventHandler(func(event serial.Event) {
		trayMgr.UpdateSerialStatus()
	})

	var wsSrv *web.WebSocketServer
	var cancelFunc context.CancelFunc

	trayMgr.SetOnReady(func() {
		_, cancel := context.WithCancel(context.Background())
		cancelFunc = cancel

		// 尝试自动连接串口（如果配置了端口）
		if err := sm.Connect(); err != nil {
			logrus.Debugf("[SerialHub] 自动连接串口失败: %v", err)
		} else {
			logrus.Infof("[SerialHub] 已自动连接串口: %s", sm.GetConfig().String())
		}

		var err error
		wsSrv, err = web.NewWebSocketServer(host, wsPort, func() string {
			if sm.IsConnected() {
				return sm.GetConfig().String()
			}
			return ""
		})
		if err != nil {
			logrus.Errorf("[SerialHub] 创建 WebSocket 服务失败: %v", err)
			return
		}
		if err := wsSrv.Start(); err != nil {
			logrus.Errorf("[SerialHub] 启动 WebSocket 服务失败: %v", err)
			return
		}
		logrus.Infof("[SerialHub] WebSocket 服务已启动: %s:%d", host, wsPort)

		// 始终创建 DataBridge，即使串口未连接
		bridgeSrv, err := bridge.NewDataBridge(sm, wsSrv, buf)
		if err != nil {
			logrus.Warnf("[SerialHub] 创建数据桥接失败: %v", err)
		} else {
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

		logrus.Infof("[SerialHub] MCP HTTP 服务: http://%s/mcp", addr)
		logrus.Infof("[SerialHub] 健康检查: http://%s/health", addr)
		logrus.Infof("[SerialHub] WebSocket 端口: %d", wsPort)
	})

	trayMgr.SetOnExit(func() {
		if cancelFunc != nil {
			cancelFunc()
		}
		if wsSrv != nil {
			wsSrv.Stop()
		}
	})

	// systray.Run 必须在主线程调用，会阻塞直到 systray.Quit()
	trayMgr.Run(context.Background())

	logrus.Info("[SerialHub] 正在关闭...")
	closeLogger()
	return nil
}

func runWithoutTray(cfg *config.Config, sm *serial.SerialManager, buf *buffer.DataBuffer) error {
	// 设置配置变更回调（无托盘模式也需要保存配置）
	sm.SetConfigChangeHandler(func(serialCfg *serial.Config) {
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
	})

	// 尝试自动连接串口（如果配置了端口）
	if err := sm.Connect(); err != nil {
		logrus.Debugf("[SerialHub] 自动连接串口失败: %v", err)
	} else {
		logrus.Infof("[SerialHub] 已自动连接串口: %s", sm.GetConfig().String())
	}

	wsSrv, err := web.NewWebSocketServer(host, wsPort, func() string {
		if sm.IsConnected() {
			return sm.GetConfig().String()
		}
		return ""
	})
	if err != nil {
		return fmt.Errorf("创建 WebSocket 服务失败: %w", err)
	}
	if err := wsSrv.Start(); err != nil {
		return fmt.Errorf("启动 WebSocket 服务失败: %w", err)
	}
	logrus.Infof("[SerialHub] WebSocket 服务已启动: %s:%d", host, wsPort)

	// 始终创建 DataBridge，即使串口未连接
	bridgeSrv, err := bridge.NewDataBridge(sm, wsSrv, buf)
	if err != nil {
		logrus.Warnf("[SerialHub] 创建数据桥接失败: %v", err)
	} else {
		bridgeSrv.Start()
		logrus.Info("[SerialHub] 数据桥接已启动")
	}

	mcpSrv, err := mcp.NewMCPServer(sm, buf, wsSrv)
	if err != nil {
		return fmt.Errorf("创建 MCP 服务失败: %w", err)
	}
	if err := mcpSrv.RegisterTools(); err != nil {
		return fmt.Errorf("注册 MCP 工具失败: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", host, mcpPort)
	if _, err := mcpSrv.StartHTTPServer(addr); err != nil {
		return fmt.Errorf("启动 HTTP 服务失败: %w", err)
	}

	logrus.Infof("[SerialHub] MCP HTTP 服务: http://%s/mcp", addr)
	logrus.Infof("[SerialHub] 健康检查: http://%s/health", addr)
	logrus.Infof("[SerialHub] WebSocket 端口: %d", wsPort)
	logrus.Info("[SerialHub] 服务已启动，按 Ctrl+C 退出")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logrus.Info("[SerialHub] 正在关闭...")
	closeLogger()
	wsSrv.Stop()
	return nil
}

var logFile *os.File

func setupLogger(cfg *config.Config) {
	if debugMode || cfg.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	formatter := &logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		DisableColors:   true,
	}

	dir := cfg.LogDir
	if dir == "" {
		exePath, _ := os.Executable()
		dir = filepath.Join(filepath.Dir(exePath), "logs")
	}
	os.MkdirAll(dir, 0755)

	var err error
	logFile, err = os.OpenFile(filepath.Join(dir, "serialhub.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logrus.SetFormatter(formatter)
		return
	}

	logrus.SetOutput(io.MultiWriter(os.Stdout, logFile))
	logrus.SetFormatter(formatter)
}

func closeLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

func loadConfig() *config.Config {
	cfg := config.GetDefault()

	if logDir := os.Getenv("SERIALHUB_LOG_DIR"); logDir != "" {
		cfg.LogDir = logDir
	}

	if configPath == "" {
		exePath, err := os.Executable()
		if err == nil {
			configPath = filepath.Join(filepath.Dir(exePath), "config.toml")
		}
	}

	if configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			logrus.Warnf("[SerialHub] 加载配置文件失败: %v", err)
		} else {
			cfg = loaded
		}
	}

	if serialPort != "" {
		cfg.Serial.Port = serialPort
	}
	if baudRate != 115200 {
		cfg.Serial.BaudRate = baudRate
	}
	if host != "" && host != "127.0.0.1" {
		cfg.Host = host
	}
	if wsPort != 2323 {
		cfg.WebSocket.Port = wsPort
	}
	if mcpPort != 5000 {
		cfg.MCP.HTTPPort = mcpPort
	}

	if configPath != "" {
		if err := config.Save(configPath, cfg); err != nil {
			logrus.Warnf("[SerialHub] 保存配置失败: %v", err)
		}
	}

	return cfg
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
