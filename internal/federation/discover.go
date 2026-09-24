package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// discoverTimeout 单个候选地址的 /health 探测超时。
const discoverTimeout = 800 * time.Millisecond

// IsWSL 判断当前是否运行在 WSL 环境中。
func IsWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	// 旧版 WSL 无 WSL_DISTRO_NAME，读内核版本描述
	if b, err := os.ReadFile("/proc/version"); err == nil {
		return strings.Contains(strings.ToLower(string(b)), "microsoft")
	}
	return false
}

// LocalSide 返回本实例所在侧标识（windows/wsl）。
func LocalSide() string {
	if IsWSL() {
		return SideWSL
	}
	return SideWindows
}

// DefaultGateway 解析 /proc/net/route 返回默认路由网关 IP
// （WSL NAT 模式下即 Windows 宿主地址）；失败返回空串。
func DefaultGateway() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n")[1:] { // 跳过表头
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[1] != "00000000" {
			continue // 只看默认路由（Destination == 0.0.0.0）
		}
		return hexGatewayToIP(fields[2])
	}
	return ""
}

// hexGatewayToIP 将 /proc/net/route 的小端十六进制网关转为点分 IP。
func hexGatewayToIP(hex string) string {
	if len(hex) != 8 {
		return ""
	}
	var bytes [4]string
	for i := 0; i < 4; i++ {
		b, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			return ""
		}
		bytes[3-i] = strconv.FormatUint(b, 10) // 小端反转
	}
	return strings.Join(bytes[:], ".")
}

// healthInfo /health 端点的 JSON 响应结构。
type healthInfo struct {
	Status string `json:"status"`
	Role   string `json:"role"`
}

// probeMaster 探测单个候选地址是否为主实例：
// 仅 200 且 role=master 命中。从实例反代的 /health 返回 role=worker，须跳过；
// 缺失 role、非法 JSON、非 ok 状态一律不认（主从同版本发布，不做旧版兼容）。
func probeMaster(client *http.Client, base string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var h healthInfo
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return false
	}
	return h.Status == "ok" && h.Role == "master"
}

// DiscoverMaster 探测主实例：候选 127.0.0.1:port；WSL 环境补探 Windows 宿主:port。
// 严格只认 /health 返回 role=master 的实例；命中失败的候选继续探测后续地址。
// 返回主实例基址（如 "http://172.20.0.1:5000"）或空串（未发现）。
func DiscoverMaster(port int) string {
	var candidates []string
	candidates = append(candidates, fmt.Sprintf("http://127.0.0.1:%d", port))
	if IsWSL() {
		if gw := DefaultGateway(); gw != "" {
			gwAddr := fmt.Sprintf("http://%s:%d", gw, port)
			// 避免与 127.0.0.1 重复（mirrored 网络模式下网关探测会得到本机）
			if gwAddr != candidates[0] {
				candidates = append(candidates, gwAddr)
			}
		}
	}

	client := &http.Client{Timeout: discoverTimeout}
	for _, base := range candidates {
		if probeMaster(client, base) {
			return base
		}
	}
	return ""
}
