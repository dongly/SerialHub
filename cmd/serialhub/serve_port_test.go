package main

import (
	"net"
	"strconv"
	"testing"
)

// TestResolveListenPort_端口被占时自动迁移：请求端口被监听者占用（EADDRINUSE）
// 时应返回下一个可用端口；请求端口空闲时原样返回。
func TestResolveListenPort_端口被占时自动迁移(t *testing.T) {
	// 先占住一个空闲端口
	occ, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占用测试端口失败: %v", err)
	}
	defer occ.Close()
	occPort := occ.Addr().(*net.TCPAddr).Port

	// 请求被占端口：应迁移到 +1
	got, err := resolveListenPort("127.0.0.1", occPort)
	if err != nil {
		t.Fatalf("resolveListenPort 失败: %v", err)
	}
	if got != occPort+1 {
		t.Fatalf("预期迁移到 %d，实际 %d", occPort+1, got)
	}

	// 空闲端口：原样返回（occPort+1 已被上一轮验证可用）
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("获取空闲端口失败: %v", err)
	}
	freePort := free.Addr().(*net.TCPAddr).Port
	free.Close()

	got, err = resolveListenPort("127.0.0.1", freePort)
	if err != nil {
		t.Fatalf("resolveListenPort 失败: %v", err)
	}
	if got != freePort {
		t.Fatalf("空闲端口预期原样返回 %d，实际 %d", freePort, got)
	}
}

// TestResolveListenPort_全部被占时报错：maxPortFallback+1 个连续端口全占时应返回错误。
func TestResolveListenPort_全部被占时报错(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("获取起始端口失败: %v", err)
	}
	basePort := base.Addr().(*net.TCPAddr).Port
	holders := []net.Listener{base}
	defer func() {
		for _, h := range holders {
			h.Close()
		}
	}()
	for i := 1; i <= maxPortFallback; i++ {
		h, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(basePort+i)))
		if err != nil {
			t.Fatalf("占位 %d 失败: %v", i, err)
		}
		holders = append(holders, h)
	}

	if _, err := resolveListenPort("127.0.0.1", basePort); err == nil {
		t.Fatal("全部端口被占时预期报错，实际成功")
	}
}
