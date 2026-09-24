package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/dongly/serialhub/pkg/config"
	"github.com/sirupsen/logrus"
)

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

// setLogFileOnly 将日志输出限制为仅写文件（stdio 模式下 stdout 承载 MCP 协议流，
// 绝不能混入日志）。
func setLogFileOnly() {
	if logFile != nil {
		logrus.SetOutput(logFile)
	} else {
		logrus.SetOutput(io.Discard)
	}
}
