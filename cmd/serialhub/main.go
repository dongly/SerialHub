package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version    = getVersion()
	serialPort string
	baudRate   int
	configPath string
	debugMode  bool
	mcpPort    int
	host       string
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
	rootCmd.Flags().IntVarP(&mcpPort, "mcp-port", "m", 5000, "MCP HTTP 服务端口")
	rootCmd.Flags().StringVar(&host, "host", "127.0.0.1", "监听地址")
	rootCmd.Flags().BoolVar(&minimized, "minimized", false, "由脚本启动，窗口最小化")

	rootCmd.Version = version
	rootCmd.SetVersionTemplate(fmt.Sprintf("SerialHub v%s\n", version))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}