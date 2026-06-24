// Package modeldinstall discovers, downloads, verifies, installs, and validates a
// prebuilt modeld package for the current machine. It is the implementation behind
// the `contenox setup` check for the local llama/openvino providers: see
// docs/blueprints/modeld-setup-artifact-detection.md.
//
// It speaks plain HTTPS to the public release prefix — no AWS SDK, no bucket
// listing, no credentials. The .sha256 file is the availability probe; the archive
// is verified against it before extraction, and the extracted binary is validated
// with `modeld version --json` before it is moved into place.
package modeldinstall

import (
	"fmt"
	"strings"
)

// DefaultBaseURL is the public modeld release prefix compiled into the released
// CLI. Only the final-package prefix is public; the native dependency prefix
// (modeld-deps/) stays private. Tests inject an alternate via Options.BaseURL.
const DefaultBaseURL = "https://contenox-modeld-artifacts-573643652148.s3.amazonaws.com/modeld"

// Platform is the release platform string, e.g. "linux-amd64".
func Platform(goos, goarch string) string { return goos + "-" + goarch }

// archiveExt returns the published archive extension for goos, or "" if the OS
// has no published package format.
func archiveExt(goos string) string {
	switch goos {
	case "linux", "darwin":
		return ".tar.gz"
	case "windows":
		return ".zip"
	default:
		return ""
	}
}

// LauncherName is the entry-point script inside the package for goos. The launcher
// (not modeld.bin/modeld.exe) is what callers run: it sets the native library
// search path before exec'ing the real binary.
func LauncherName(goos string) string {
	if goos == "windows" {
		return "modeld.cmd"
	}
	return "modeld"
}

// IsOfficialVersion reports whether v is an official release tag the default
// download path may use. Dev builds ("", "dev", "main", "unknown", commit shas)
// are not official: setup must fall back to source-build instead of silently
// downloading an arbitrary package.
func IsOfficialVersion(v string) bool {
	return strings.HasPrefix(strings.TrimSpace(v), "v")
}

// artifact holds the deterministic release names and URLs for a version+platform.
type artifact struct {
	version    string
	platform   string
	name       string // modeld-<version>-<platform>.<ext>
	archiveURL string
	sumURL     string
}

// topLevelDir is the single directory the archive unpacks to:
// modeld-<version>-<platform> (the name without its archive extension). The
// release packer tars/zips that directory, so extraction yields exactly one entry.
func (a artifact) topLevelDir() string {
	return fmt.Sprintf("modeld-%s-%s", a.version, a.platform)
}

// resolveArtifact builds the release names/URLs, or returns ErrNoOfficialVersion /
// ErrUnsupportedPlatform when no package could exist for this version/platform.
func resolveArtifact(baseURL, version, goos, goarch string) (artifact, error) {
	version = strings.TrimSpace(version)
	if !IsOfficialVersion(version) {
		return artifact{}, ErrNoOfficialVersion
	}
	ext := archiveExt(goos)
	if ext == "" {
		return artifact{}, ErrUnsupportedPlatform
	}
	plat := Platform(goos, goarch)
	name := fmt.Sprintf("modeld-%s-%s%s", version, plat, ext)
	base := strings.TrimRight(baseURL, "/")
	archiveURL := fmt.Sprintf("%s/%s/%s", base, version, name)
	return artifact{
		version:    version,
		platform:   plat,
		name:       name,
		archiveURL: archiveURL,
		sumURL:     archiveURL + ".sha256",
	}, nil
}
