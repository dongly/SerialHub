package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dongly/serialhub/pkg/config"
	"github.com/sirupsen/logrus"
)

var logFile *os.File

// defaultLogDir 返回默认日志目录：Windows 为 exe 同目录 logs/（原有行为）；
// 非 Windows 为用户配置目录 serialhub/logs/（与配置文件位置一致，旧
// logs/ 历史日志不迁移），拿不到用户配置目录时退回 exe 同目录。
func defaultLogDir() string {
	if runtime.GOOS != "windows" {
		if base := xdgConfigDir(); base != "" {
			return filepath.Join(base, "serialhub", "logs")
		}
	}
	if exePath, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exePath), "logs")
	}
	return "logs"
}

func setupLogger(cfg *config.Config) {
	// --log-data 蕴含 -D：Trace 级包含 Debug/Info 全部输出，外加数据内容日志。
	// logDataEffective 已在 loadConfig 按 显式 flag > 环境变量 > 配置文件 合并。
	switch {
	case logDataEffective:
		logrus.SetLevel(logrus.TraceLevel)
	case debugMode || cfg.Debug:
		logrus.SetLevel(logrus.DebugLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}

	formatter := &logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		DisableColors:   true,
	}

	dir := cfg.LogDir
	if dir == "" {
		dir = defaultLogDir()
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

// setLogFileOnly 将日志输出限制为仅写文件（stdio 模式下 stdout 承载 MCP 协议流，
// 绝不能混入日志）。
func setLogFileOnly() {
	if logFile != nil {
		logrus.SetOutput(logFile)
	} else {
		logrus.SetOutput(io.Discard)
	}
}
