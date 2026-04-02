package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/yourname/serialhub/pkg/app"
	"github.com/yourname/serialhub/pkg/config"
)

func main() {
	cfg := config.GetDefault()

	application, err := app.NewApp("0.1.0", cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败: %v\n", err)
		os.Exit(1)
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := application.RunServe(ctx, 2323, 5000, false); err != nil {
		fmt.Fprintf(os.Stderr, "运行失败: %v\n", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	application.Close()
}
