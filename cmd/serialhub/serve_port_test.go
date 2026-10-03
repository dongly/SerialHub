package main

import (
	"net"
	"strconv"
	"testing"
)

// occupyConsecutive 占据 count 个连续空闲端口，返回起始端口与监听器。
// 从 :0 随机起点向外扩，ephemeral 段内相邻端口可能已被系统连接占用，
// 任一失败即整段释放换起点重试；找不到整段则 Skip（不判失败）。
func occupyConsecutive(t *testing.T, count int) (int, []net.Listener) {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		probe, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("获取起始端口失败: %v", err)
		}
		p := probe.Addr().(*net.TCPAddr).Port
		probe.Close()
		if p+count-1 > 65535 {
			continue
		}
		var hs []net.Listener
		ok := true
		for i := 0; i < count; i++ {
			h, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p+i)))
			if err != nil {
				ok = false
				break
			}
			hs = append(hs, h)
		}
		if ok {
			return p, hs
		}
		for _, h := range hs {
			h.Close()
		}
	}
	t.Skipf("找不到连续 %d 个空闲端口", count)
	return 0, nil // unreachable
}

// TestResolveListenPort_FallbackWhenOccupied：请求端口被监听者占用（EADDRINUSE）
// 时应返回下一个可用端口；请求端口空闲时原样返回。
func TestResolveListenPort_FallbackWhenOccupied(t *testing.T) {
	// 占据连续两格以证明 p+1 曾空闲，随即释放 p+1，留 p 占用：
	// 直接依赖「:0 给出的 p 之后恰好 p+1 空闲」在连接密集的机器上会 flake。
	p, hs := occupyConsecutive(t, 2)
	defer func() {
		for _, h := range hs {
			if h != nil {
				h.Close()
			}
		}
	}()
	hs[1].Close()
	hs[1] = nil

	// 请求被占端口：应迁移到 +1
	got, err := resolveListenPort("127.0.0.1", p)
	if err != nil {
		t.Fatalf("resolveListenPort 失败: %v", err)
	}
	if got != p+1 {
		t.Fatalf("预期迁移到 %d，实际 %d", p+1, got)
	}

	// 空闲端口：原样返回（获取后立即释放，被抢占属极小概率，失败即报 flake）
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

// TestResolveListenPort_AllOccupiedErrors：maxPortFallback+1 个连续端口全占时应返回错误。
func TestResolveListenPort_AllOccupiedErrors(t *testing.T) {
	// 全程持有整段端口，「全占」条件由本测试自身保证，不受系统占用干扰。
	basePort, holders := occupyConsecutive(t, maxPortFallback+1)
	defer func() {
		for _, h := range holders {
			h.Close()
		}
	}()

	if _, err := resolveListenPort("127.0.0.1", basePort); err == nil {
		t.Fatal("全部端口被占时预期报错，实际成功")
	}
}
