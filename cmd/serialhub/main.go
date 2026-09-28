package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dongly/serialhub/pkg/config"
	"github.com/dongly/serialhub/pkg/version"
)

var (
	appVersion  = version.FullVersion()
	serialPort  string
	baudRate    int
	configPath  string
	debugMode   bool
	logDataMode bool
	mcpPort     int
	host        string
	minimized   bool
	noBrowser   bool
	stdioMode   bool

	// logDataFlag 是 --log-data 的 flag 引用，供 loadConfig 判断
	// 是否被显式指定（--log-data=false 显式关闭优先于环境变量与配置文件）。
	logDataFlag *pflag.Flag
	// logDataEffective 是数据内容日志的最终生效开关：
	// 显式 flag > 环境变量 SERIALHUB_LOG_DATA > 配置文件 logData。
	// 只影响本次运行，不回写配置文件。
	logDataEffective bool
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
	rootCmd.Flags().BoolVar(&logDataMode, "log-data", false, "输出数据内容日志（500ms 时间窗聚合、单条截断 512 字节；可用 SERIALHUB_LOG_DATA=1，--log-data=false 显式关闭）")
	rootCmd.Flags().IntVarP(&mcpPort, "mcp-port", "m", config.DefaultHTTPPort, "MCP HTTP 服务端口")
	rootCmd.Flags().StringVar(&host, "host", "127.0.0.1", "监听地址")
	rootCmd.Flags().BoolVar(&minimized, "minimized", false, "由脚本启动，窗口最小化")
	rootCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "跳过自动打开浏览器")
	rootCmd.Flags().BoolVar(&stdioMode, "stdio", false, "以 stdio 模式运行（MCP 客户端本地拉起）")
	logDataFlag = rootCmd.Flags().Lookup("log-data")

	rootCmd.Version = appVersion
	rootCmd.SetVersionTemplate(fmt.Sprintf("SerialHub v%s\n", appVersion))
	rootCmd.AddCommand(newSetupCmd())
	rootCmd.AddCommand(newUninstallCmd())
	rootCmd.AddCommand(newUpgradeCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
