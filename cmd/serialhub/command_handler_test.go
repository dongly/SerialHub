package main

import (
	"encoding/json"
	"testing"

	"github.com/dongly/serialhub/pkg/serial"
)

// TestCommandHandler_Shutdown：shutdown 命令广播 shutting_down 响应并触发停机回调；
// 回调为 nil 时返回错误而非 panic。
func TestCommandHandler_Shutdown(t *testing.T) {
	sm, _ := serial.NewSerialManager(&serial.Config{Port: "", BaudRate: 115200})

	t.Run("触发停机回调", func(t *testing.T) {
		called := make(chan struct{}, 1)
		handler := createCommandHandler(sm, func() { called <- struct{}{} })
		resp := handler([]byte(`{"type":"shutdown"}`))
		if resp == nil {
			t.Fatal("响应为空")
		}
		var msg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(resp, &msg); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if msg.Type != "shutting_down" {
			t.Fatalf("响应类型 = %q, 期望 shutting_down", msg.Type)
		}
		select {
		case <-called:
		default:
			t.Fatal("停机回调未被调用")
		}
	})

	t.Run("回调为nil时返回错误", func(t *testing.T) {
		handler := createCommandHandler(sm, nil)
		resp := handler([]byte(`{"type":"shutdown"}`))
		if resp == nil {
			t.Fatal("响应为空")
		}
		var msg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(resp, &msg); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if msg.Type != "error" {
			t.Fatalf("响应类型 = %q, 期望 error", msg.Type)
		}
	})

	t.Run("重复触发不阻塞", func(t *testing.T) {
		defer drainWebShutdown()
		handler := createCommandHandler(sm, requestWebShutdown) // 真实非阻塞实现
		for i := 0; i < 3; i++ {
			if resp := handler([]byte(`{"type":"shutdown"}`)); resp == nil {
				t.Fatalf("第 %d 次响应为空", i+1)
			}
		}
		// 容量 1：多次请求合并为一条通知，只应收到一次
		select {
		case <-webShutdown:
		default:
			t.Fatal("停机通知未被写入")
		}
		select {
		case <-webShutdown:
			t.Fatal("重复请求不应产生第二条通知")
		default:
		}
	})
}

// drainWebShutdown 排空停机通知，避免影响其他测试。
func drainWebShutdown() {
	for {
		select {
		case <-webShutdown:
		default:
			return
		}
	}
}

// TestRequestWebShutdown_先请求后等待仍能收到：主循环尚未开始等待时
// （启动窗口内）的首次停机请求不得丢失——webShutdown 容量必须为 1。
func TestRequestWebShutdown_先请求后等待仍能收到(t *testing.T) {
	defer drainWebShutdown()
	requestWebShutdown() // 此时无任何接收方
	requestWebShutdown() // 重复请求，应合并不堆积
	select {
	case <-webShutdown:
	default:
		t.Fatal("首次请求在接收方就绪前被丢弃（webShutdown 缓冲不足）")
	}
	select {
	case <-webShutdown:
		t.Fatal("容量 1 的通道不应缓存第二条通知")
	default:
	}
}
