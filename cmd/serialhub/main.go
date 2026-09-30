package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/dongly/serialhub/internal/i18n"
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
		Short: i18n.CLI.RootShort,
		Long:  i18n.CLI.RootLong,
		RunE:  runServe,
		Args:  cobra.NoArgs,
		// 运行期错误（如主实例失联）不需要 Usage 帮助；错误统一由 main
		// 单行打印（SilenceErrors 关掉 cobra 自带输出，避免重复两遍）。
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&serialPort, "serial-port", "p", "", i18n.CLI.FlagSerialPort)
	rootCmd.PersistentFlags().IntVarP(&baudRate, "baud-rate", "b", 115200, i18n.CLI.FlagBaudRate)
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", i18n.CLI.FlagConfig)
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "D", false, i18n.CLI.FlagDebug)
	rootCmd.Flags().BoolVar(&logDataMode, "log-data", false, i18n.CLI.FlagLogData)
	rootCmd.Flags().IntVarP(&mcpPort, "mcp-port", "m", config.DefaultHTTPPort, i18n.CLI.FlagMCPPort)
	rootCmd.Flags().StringVar(&host, "host", "127.0.0.1", i18n.CLI.FlagHost)
	rootCmd.Flags().BoolVar(&minimized, "minimized", false, i18n.CLI.FlagMinimized)
	rootCmd.Flags().BoolVar(&noBrowser, "no-browser", false, i18n.CLI.FlagNoBrowser)
	rootCmd.Flags().BoolVar(&stdioMode, "stdio", false, i18n.CLI.FlagStdio)
	logDataFlag = rootCmd.Flags().Lookup("log-data")

	rootCmd.Version = appVersion
	rootCmd.SetVersionTemplate(fmt.Sprintf("SerialHub v%s\n", appVersion))
	rootCmd.AddCommand(newSetupCmd())
	rootCmd.AddCommand(newUninstallCmd())
	rootCmd.AddCommand(newUpgradeCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
