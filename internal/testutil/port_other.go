//go:build !linux && !windows

package testutil

// probeTestPort 在其余平台不提供自动探测，统一返回不可用原因。
func probeTestPort() (string, bool, string) {
	return "", false, "当前平台不支持自动探测测试串口，请设置 SERIALHUB_TEST_PORT"
}
