package version

import "runtime/debug"

// Version 是默认版本号（fallback）。CI 发布构建通过
// -ldflags "-X github.com/dongly/serialhub/pkg/version.Version=<tag>" 注入，
// 未注入时（本地构建）以本值为准，并由 buildvcs 追加 commit 哈希。
var (
	Name    = "serialhub"
	Version = "0.5.0"
)

func FullVersion() string {
	vcsInfo := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				vcsInfo = setting.Value[:7]
				break
			}
		}
	}
	if vcsInfo != "" {
		return Version + "-" + vcsInfo
	}
	return Version + "-dev"
}
