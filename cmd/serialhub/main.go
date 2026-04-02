package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/yourname/serialhub/internal/buffer"
	"github.com/yourname/serialhub/internal/service"
	"github.com/yourname/serialhub/pkg/bridge"
	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/mcp"
	"github.com/yourname/serialhub/pkg/serial"
	"github.com/yourname/serialhub/pkg/telnet"
)

var (
	version    = "0.1.0"
	serialPort string
	baudRate   int
	configPath string
	debugMode  bool
	telnetPort int
	mcpPort    int
	host       string
	noTray     bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "serialhub",
		Short: "SerialHub - 串口与网络连接的双向桥接器",
		Long:  "SerialHub 将 MCU 串口数据同时转发到 Telnet（人工监视）和 MCP（AI 工具程序化访问）。",
		RunE:  runServe,
		Args:  cobra.NoArgs,
	}

	rootCmd.PersistentFlags().StringVarP(&serialPort, "serial-port", "p", "", "串口名（如 COM9 或 /dev/ttyUSB0）")
	rootCmd.PersistentFlags().IntVarP(&baudRate, "baud-rate", "b", 115200, "波特率")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "配置文件路径")
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "D", false, "启用调试模式")
	rootCmd.Flags().IntVarP(&telnetPort, "telnet-port", "t", 2323, "Telnet 服务端口")
	rootCmd.Flags().IntVarP(&mcpPort, "mcp-port", "m", 5000, "MCP HTTP 服务端口")
	rootCmd.Flags().StringVar(&host, "host", "127.0.0.1", "监听地址")
	rootCmd.Flags().BoolVar(&noTray, "no-tray", false, "禁用系统托盘")

	rootCmd.Version = version
	rootCmd.SetVersionTemplate(fmt.Sprintf("SerialHub v%s\n", version))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	setupLogger()

	cfg := loadConfig()
	logrus.Infof("[SerialHub] SerialHub v%s 启动中...", version)
	logrus.Info("[SerialHub] 运行模式: serve")

	buf := buffer.NewDataBuffer()
	serialCfg := configToSerialConfig(&cfg.Serial)
	sm, err := serial.NewSerialManager(serialCfg)
	if err != nil {
		logrus.Debugf("[SerialHub] 串口管理器初始化跳过: %v", err)
		sm = nil
	}

	// 自动连接串口
	if sm != nil {
		if err := sm.Connect(); err != nil {
			logrus.Warnf("[SerialHub] 自动连接串口失败: %v", err)
		} else {
			logrus.Infof("[SerialHub] 已自动连接串口: %s", serialCfg.String())
		}
	}

	telnetSrv, err := telnet.NewTelnetServer(host, telnetPort, func() string {
		if sm != nil && sm.IsConnected() {
			return sm.GetConfig().String()
		}
		return ""
	})
	if err != nil {
		return fmt.Errorf("创建 Telnet 服务失败: %w", err)
	}
	if err := telnetSrv.Start(); err != nil {
		return fmt.Errorf("启动 Telnet 服务失败: %w", err)
	}
	logrus.Infof("[SerialHub] Telnet 服务已启动: %s:%d", host, telnetPort)

	if sm != nil {
		bridgeSrv, err := bridge.NewDataBridge(sm, telnetSrv, buf)
		if err != nil {
			logrus.Warnf("[SerialHub] 创建数据桥接失败: %v", err)
		} else {
			bridgeSrv.Start()
			logrus.Info("[SerialHub] 数据桥接已启动")
		}
	}

	mcpSrv, err := mcp.NewMCPServer(sm, buf, nil)
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
	logrus.Infof("[SerialHub] Telnet 端口: %d", telnetPort)
	logrus.Info("[SerialHub] 服务已启动，按 Ctrl+C 退出")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logrus.Info("[SerialHub] 正在关闭...")
	telnetSrv.Stop()
	return nil
}

func setupLogger() {
	if debugMode {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
}

func loadConfig() *config.Config {
	cfg := config.GetDefault()

	if configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			logrus.Warnf("[SerialHub] 加载配置文件失败: %v", err)
		} else {
			cfg = loaded
		}
	}

	// CLI 参数覆盖配置文件
	if serialPort != "" {
		cfg.Serial.Port = serialPort
	}
	if baudRate != 115200 {
		cfg.Serial.BaudRate = baudRate
	}

	if cfg.Serial.Port == "" {
		svc := service.NewServiceManager()
		lastSerial, err := svc.LoadLastSerial()
		if err != nil {
			logrus.Debugf("[SerialHub] 读取上次串口配置失败: %v", err)
		} else if lastSerial != nil && lastSerial.Port != "" {
			cfg.Serial.Port = lastSerial.Port
			cfg.Serial.BaudRate = lastSerial.BaudRate
			cfg.Serial.DataBits = lastSerial.DataBits
			cfg.Serial.Parity = lastSerial.Parity
			cfg.Serial.StopBits = int(lastSerial.StopBits)
			logrus.Infof("[SerialHub] 使用上次连接的串口: %s@%d", lastSerial.Port, lastSerial.BaudRate)
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
