package version

import "runtime/debug"

const (
	Name    = "serialhub"
	Version = "0.2.0"
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
