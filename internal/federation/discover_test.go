package federation

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// TestProbeMaster 验证实例发现的严格 role 判定：
// 仅 role=master 命中；worker 反代、缺失 role、非法 JSON、非 200 一律跳过。
func TestProbeMaster(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    bool
	}{
		{
			name: "主实例 role=master 命中",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"status":"ok","role":"master"}`))
			},
			want: true,
		},
		{
			name: "从实例反代 role=worker 跳过",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"status":"ok","role":"worker"}`))
			},
			want: false,
		},
		{
			name: "缺失 role 跳过（不做旧版兼容）",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"status":"ok"}`))
			},
			want: false,
		},
		{
			name: "非法 JSON 跳过",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`not-json`))
			},
			want: false,
		},
		{
			name: "非 200 跳过",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			want: false,
		},
	}

	client := &http.Client{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()
			if got := probeMaster(client, srv.URL); got != c.want {
				t.Errorf("probeMaster = %v, 期望 %v", got, c.want)
			}
		})
	}
}

// TestDiscoverMaster_Worker反代不算主实例：本机候选返回 role=worker 时
// DiscoverMaster 必须返回空串（否则同侧第三实例会误认从实例反代为主，
// 引发递归 runWorker 死循环）。
func TestDiscoverMaster_Worker反代不算主实例(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok","role":"worker"}`))
	}))
	defer worker.Close()

	// worker.URL 形如 http://127.0.0.1:PORT，提取端口作为发现候选端口
	portPart := worker.URL[strings.LastIndexByte(worker.URL, ':')+1:]
	port, err := strconv.Atoi(portPart)
	if err != nil {
		t.Fatalf("解析测试端口失败: %v", err)
	}

	if got := DiscoverMaster(port); got != "" {
		t.Errorf("DiscoverMaster 探到 worker 反代 %q，期望空串（严格 role=master 校验）", got)
	}
}
