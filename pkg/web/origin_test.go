package web

import (
	"net/http/httptest"
	"testing"
)

// TestCheckOrigin：浏览器 WebSocket 请求必须同源（防第三方页面跨站发送
// shutdown 等控制命令）；无 Origin 头的非浏览器客户端（脚本/测试）放行。
func TestCheckOrigin(t *testing.T) {
	t.Run("无Origin头放行（非浏览器客户端）", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/ws", nil)
		if !checkOrigin(r) {
			t.Fatal("无 Origin 头的请求应放行")
		}
	})

	t.Run("同源放行", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = "127.0.0.1:5050"
		r.Header.Set("Origin", "http://127.0.0.1:5050")
		if !checkOrigin(r) {
			t.Fatal("同源请求应放行")
		}
	})

	t.Run("跨站Origin拒绝", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = "127.0.0.1:5050"
		r.Header.Set("Origin", "http://evil.example")
		if checkOrigin(r) {
			t.Fatal("跨站 Origin 应被拒绝")
		}
	})

	t.Run("同主机不同端口拒绝", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = "127.0.0.1:5050"
		r.Header.Set("Origin", "http://127.0.0.1:8080")
		if checkOrigin(r) {
			t.Fatal("同主机不同端口的 Origin 应被拒绝")
		}
	})

	t.Run("Origin格式非法拒绝", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = "127.0.0.1:5050"
		r.Header.Set("Origin", "http://[::1") // url.Parse 失败
		if checkOrigin(r) {
			t.Fatal("非法 Origin 应被拒绝")
		}
	})

	t.Run("Origin为null拒绝", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = "127.0.0.1:5050"
		r.Header.Set("Origin", "null") // 沙箱 iframe 等来源，序列化后 Host 为空
		if checkOrigin(r) {
			t.Fatal("Origin: null 应被拒绝")
		}
	})

	t.Run("协议相对地址拒绝", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = "127.0.0.1:5050"
		r.Header.Set("Origin", "//127.0.0.1:5050") // url.Parse 成功但非浏览器合法 Origin
		if checkOrigin(r) {
			t.Fatal("协议相对 Origin 应被拒绝")
		}
	})

	t.Run("IPv6同源放行", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = "[::1]:5050"
		r.Header.Set("Origin", "http://[::1]:5050")
		if !checkOrigin(r) {
			t.Fatal("IPv6 同源请求应放行")
		}
	})
}
