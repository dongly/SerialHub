package main

import "runtime/debug"

const baseVersion = "0.2.0"

func getVersion() string {
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
		return baseVersion + "-" + vcsInfo
	}
	return baseVersion + "-dev"
}