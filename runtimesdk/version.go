package runtimesdk

import (
	"runtime/debug"

	"strings"
)

func GetSDKVersion() string {
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
