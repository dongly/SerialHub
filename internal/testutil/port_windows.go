//go:build windows

package testutil

import (
	"fmt"

	"go.bug.st/serial/enumerator"
)

// probeTestPort 枚举串口并优先返回非 USB 端口：com0com 等虚拟对在
// 枚举中标记 IsUSB=false，避免误开真实 USB 设备触发 DTR 复位。
func probeTestPort() (string, bool, string) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return "", false, fmt.Sprintf("枚举串口失败: %v", err)
	}
	for _, p := range ports {
		if p.Name != "" && !p.IsUSB {
			return p.Name, true, ""
		}
	}
	return "", false, "未找到非 USB 虚拟串口（如 com0com 对），请安装 com0com 或设置 SERIALHUB_TEST_PORT"
}
