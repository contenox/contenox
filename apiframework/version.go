package apiframework

import (
	"runtime/debug"
	"strings"
)

var Version string

type AboutServer struct {
	Version        string `json:"version"`
	NodeInstanceID string `json:"nodeInstanceID"`
	Tenancy        string `json:"tenancy"`
}

func GetVersion() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	if info.Main.Path == "github.com/contenox/runtime" {
		version := strings.TrimSuffix(info.Main.Version, "+incompatible")
		return version
	}

	for _, dep := range info.Deps {
		if dep.Path == "github.com/contenox/runtime" {
			version := strings.TrimSuffix(dep.Version, "+incompatible")
			return version
		}
	}

	return "unknown"
}
