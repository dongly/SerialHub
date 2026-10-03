package testutil

import (
	"os"
	"sync"
)

// 测试串口解析结果：sync.Once 进程内只探测一次，保证同一测试包内
// 多次调用返回同一端口（Linux 下 pty 主端随之常驻到进程退出）。
var (
	testPortOnce sync.Once
	testPortVal  string
	testPortOK   bool
	testPortWhy  string
)

// TestPort 解析硬件在环测试用的可打开串口：优先取 SERIALHUB_TEST_PORT
// 环境变量；未设置时自动探测（Linux 纯 Go 创建 pty，Windows 取 com0com 类
// 非 USB 虚拟串口）。ok=false 时 why 是不可用原因，调用方应 t.Skip 而非
// 判失败。
func TestPort() (port string, ok bool, why string) {
	testPortOnce.Do(func() {
		if p := os.Getenv("SERIALHUB_TEST_PORT"); p != "" {
			testPortVal, testPortOK = p, true
			return
		}
		testPortVal, testPortOK, testPortWhy = probeTestPort()
	})
	return testPortVal, testPortOK, testPortWhy
}
