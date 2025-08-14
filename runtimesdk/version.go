package runtimesdk

import (
	"runtime/debug"

	"strings"
)

var Version string

func GetSDKVersion() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/contenox/runtime/runtimesdk" {
			version := strings.TrimSuffix(dep.Version, "+incompatible")
			return version
		}
	}
	return "unknown"
}
