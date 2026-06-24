package modeldinstall

import "errors"

// Internal errors callers branch on with errors.Is. All except ErrChecksumMismatch
// are "soft" — setup should fall back to source-build instructions. A checksum
// mismatch is a hard failure: the download must not be treated as merely
// unavailable.
var (
	// ErrNoOfficialVersion: the CLI version is not a release tag (dev build).
	ErrNoOfficialVersion = errors.New("modeld setup: CLI version is not an official release")
	// ErrNoPrebuiltArtifact: no package is published for this version/platform
	// (the .sha256 returned 404).
	ErrNoPrebuiltArtifact = errors.New("modeld setup: no prebuilt artifact for version/platform")
	// ErrChecksumMismatch: the archive did not match its published checksum.
	ErrChecksumMismatch = errors.New("modeld setup: checksum mismatch")
	// ErrUnsupportedPlatform: this GOOS has no published package format.
	ErrUnsupportedPlatform = errors.New("modeld setup: unsupported platform")
	// ErrBackendMissing: the installed modeld lacks the compiled backend the
	// selected provider needs.
	ErrBackendMissing = errors.New("modeld setup: installed modeld lacks required compiled backend")
	// ErrPublicAccess: the artifact exists but is not publicly readable (HTTP 403),
	// which is a release-side configuration problem, not a "no package" answer.
	ErrPublicAccess = errors.New("modeld setup: prebuilt artifact is not publicly accessible")
)
