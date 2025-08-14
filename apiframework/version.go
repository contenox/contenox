package apiframework

import (
	"runtime/debug"
	"strings"
)

type AboutServer struct {
	Version        string `json:"version"`
	NodeInstanceID string `json:"nodeInstanceID"`
	Tenancy        string `json:"tenancy"`
}

func GetVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/contenox/runtime" {
			version := strings.TrimSuffix(dep.Version, "+incompatible")
			return version
		}
	}
	return "unknown"
}
