package main

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// TestStartWorkerProxy_停止后端口可重绑 覆盖晋升端口释放回归：
// 从实例反代占住 host:mcpPort 后，stop() 必须完整释放端口，
// 同一地址立即可被晋升后的主服务重新监听（修复前反代无关闭机制，
// 晋升必 EADDRINUSE 且进程静默半死）。
func TestStartWorkerProxy_停止后端口可重绑(t *testing.T) {
	// 取一个空闲端口
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("探测端口失败: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	oldHost, oldPort := host, mcpPort
	host, mcpPort = "127.0.0.1", port
	defer func() { host, mcpPort = oldHost, oldPort }()

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// 反代目标随意（本测试不触发转发，只验证监听生命周期）
	stop := startWorkerProxy("http://127.0.0.1:1")
	if stop == nil {
		t.Fatal("stop 函数为 nil")
	}

	// startWorkerProxy 返回时监听已建立，可立即拨通
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("反代未按预期监听 %s: %v", addr, err)
	}
	c.Close()

	stop()
	stop() // 幂等：重复调用不应阻塞或 panic

	// 停止后同地址立即可重新监听
	ln2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("停止后同地址重绑失败（晋升场景回归）: %v", err)
	}
	ln2.Close()
}

// TestStartWorkerProxy_监听失败降级：端口已被占时返回 no-op 停止函数，
// 不 panic、不阻塞（同侧主实例占端口的正常场景）。
func TestStartWorkerProxy_监听失败降级(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占用端口失败: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	oldHost, oldPort := host, mcpPort
	host, mcpPort = "127.0.0.1", port
	defer func() { host, mcpPort = oldHost, oldPort }()

	stop := startWorkerProxy("http://127.0.0.1:1")
	if stop == nil {
		t.Fatal("监听失败也应返回 no-op 停止函数")
	}
	stop() // no-op，立即返回
}
