package apiframework

import (
	_ "embed"
	"strings"
)

//go:embed version.txt
var versionFile string

type AboutServer struct {
	Version        string `json:"version"`
	NodeInstanceID string `json:"nodeInstanceID"`
	Tenancy        string `json:"tenancy"`
}

var version string

func GetVersion() string {
	return version
}

func init() {
	version = strings.TrimSpace(versionFile)
	if version == "" {
		version = "unknown"
	}
}
