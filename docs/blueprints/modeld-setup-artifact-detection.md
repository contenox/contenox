# Blueprint: modeld Setup Artifact Detection

> Status: draft implementation blueprint. Scope is the `contenox setup` check that
> discovers, downloads, verifies, installs, and validates a prebuilt `modeld`
> package for the current machine. Release production is covered by
> [modeld release artifacts](modeld-release-artifacts.md).

## Problem

`contenox setup` currently treats local `llama` and `openvino` providers as a
source-build-only path. Selecting either provider prints instructions to clone the
runtime repository and build `modeld` locally.

That is no longer the desired default. Official `modeld` packages are published to
the public release prefix in S3, keyed by version and platform. The `contenox` CLI
should check that public release surface first. If a package exists for the CLI
version and current OS/architecture, setup should install it. If not, setup should
fall back to the existing source-build instructions.

## Goals

- Bake the public `modeld` release base URL into the released `contenox` binary.
- Detect the current platform from `runtime.GOOS` and `runtime.GOARCH`.
- Resolve the exact expected `modeld` package for the running CLI version.
- Check public S3/HTTPS for that package without requiring AWS credentials or the
  AWS CLI.
- Download only final `modeld` packages, never native dependency bundles.
- Verify the downloaded archive against its `.sha256` file before extraction.
- Install `modeld` into a per-user location that setup and runtime can find.
- Validate the installed binary with `modeld version --json`.
- Start `modeld` normally; backend autodetection and live backend choice belong
  to `modeld`, not `contenox setup`.
- Keep source-build instructions as the fallback for unsupported platforms,
  unpublished versions, network failures, or failed validation.

## Non-Goals

- No public download of `modeld-deps/` native dependency bundles.
- No runtime rebuild of llama.cpp, OpenVINO, CUDA, HIP, or Metal.
- No service manager design in this blueprint. Starting `modeld`, background
  lifecycle, launchd/systemd/Windows service integration, and auto-restart are
  follow-up work.
- No "latest" channel. The default check is exact-version only so `contenox` and
  `modeld` stay aligned.
- No dependency on a repo-local `.env`. End users must not need environment setup.
- No setup-side backend forcing. Setup must not ask users to set
  `CONTENOX_MODELD_BACKEND` and must not use it as the normal launch path.

## Public Release Contract

The public base URL is compiled into the CLI:

```go
const defaultModeldReleaseBaseURL = "https://contenox-modeld-artifacts-573643652148.s3.amazonaws.com/modeld"
```

The released binary must use that compiled default. Tests can inject an alternate
base URL through internal package options; end users should not set environment
variables for normal setup.

Only the final package prefix is public:

```text
https://contenox-modeld-artifacts-573643652148.s3.amazonaws.com/modeld/
```

The native dependency prefix remains private:

```text
s3://contenox-modeld-artifacts-573643652148/modeld-deps/
```

`contenox setup` should use HTTPS. It should not shell out to `aws`, require AWS
credentials, list the bucket, or know anything about dependency bundle
fingerprints.

## Artifact Naming

The release path is deterministic:

```text
<base-url>/<version>/<artifact-name>
<base-url>/<version>/<artifact-name>.sha256
```

Unix artifacts:

```text
modeld-<version>-<goos>-<goarch>.tar.gz
```

Windows artifacts:

```text
modeld-<version>-windows-<goarch>.zip
```

Examples:

```text
https://contenox-modeld-artifacts-573643652148.s3.amazonaws.com/modeld/v0.32.5/modeld-v0.32.5-linux-amd64.tar.gz
https://contenox-modeld-artifacts-573643652148.s3.amazonaws.com/modeld/v0.32.5/modeld-v0.32.5-linux-amd64.tar.gz.sha256
```

The platform string is:

```go
platform := runtime.GOOS + "-" + runtime.GOARCH
```

Supported package extensions:

| GOOS | Extension |
| --- | --- |
| `linux` | `.tar.gz` |
| `darwin` | `.tar.gz` |
| `windows` | `.zip` |

Unsupported GOOS/GOARCH pairs should skip download and print source-build
instructions.

## Version Selection

The normal version source is the compiled CLI version:

```go
version := strings.TrimSpace(contenoxcli.CLIVersion())
```

The default download path should only run when `version` is an official release
tag, currently a string beginning with `v`, for example `v0.32.5`.

For development builds where the CLI version is empty, `dev`, `main`, or otherwise
not a release tag, setup should not silently download an arbitrary package. It
should fall back to source-build instructions. Tests can inject a release version
through internal package options.

## Check Algorithm

When the selected provider is `llama` or `openvino`, setup should:

1. Probe any already installed `modeld`.
2. If a compatible local `modeld` exists and has the compiled capability needed
   by the selected provider, use it.
3. Resolve the desired release artifact from CLI version and platform.
4. Fetch `<artifact>.sha256` over HTTPS.
5. If `.sha256` returns `404`, report that no prebuilt package exists for this
   version/platform and print source-build instructions.
6. If `.sha256` returns another HTTP or network error, report the download check
   failed and print source-build instructions.
7. Download the archive to a temporary file under the user cache/temp directory.
8. Verify the archive SHA-256 against the downloaded `.sha256`.
9. Extract into a versioned per-user install directory.
10. Run installed `modeld version --json`.
11. Verify compiled capability:
    - provider `llama`: `backends` must include `llama`.
    - provider `openvino`: `backends` must include `openvino`.
12. Persist enough state for future CLI runs to find the installed binary.
13. Start `modeld` normally, or print the normal start command if service/startup
    management has not landed yet.
14. Continue setup by showing model-pull choices for the selected provider.

The `.sha256` file is the availability check. Do not list S3. Listing public S3 is
unnecessary, slower, and makes the CLI depend on bucket-specific behavior.

Setup does not choose the live backend. A package may contain both `llama` and
`openvino`; `modeld serve` probes the host and selects its live backend internally.
Setup and doctor can report what the live daemon selected after startup, but they
must not preselect it.

## Existing modeld Probe

Before downloading, setup should check for an already usable binary:

1. `CONTENOX_MODELD_BIN`, if set.
2. Previously installed Contenox-managed `modeld`.
3. `modeld` on `PATH`.

For each candidate:

```bash
modeld version --json
```

The JSON must parse and include:

```json
{
  "version": "v0.32.5",
  "backends": ["llama", "openvino"]
}
```

Compatibility rules:

- Prefer exact version match with the CLI version.
- Reject a candidate that does not report the compiled capability needed by the
  selected provider.
- For setup UX, an older/newer user-provided `CONTENOX_MODELD_BIN` can be reported
  clearly instead of overwritten automatically.

## Download And Verification

Use the standard Go HTTP client with a timeout. The implementation should:

- Send a clear `User-Agent`, for example `contenox/<version> modeld-setup`.
- Limit redirect behavior to normal HTTPS redirects.
- Stream archive downloads to disk, not memory.
- Write to a temp file and atomically move only after checksum verification.
- Parse `.sha256` as the first hex token on the first non-empty line.
- Compute SHA-256 over the archive bytes.
- Reject mismatches and delete the temp archive.

Pseudo-code:

```go
sumText, err := getSmallText(ctx, sumURL)
if notFound(err) {
    return ErrNoPrebuiltArtifact
}
want := parseSHA256(sumText)

archivePath, err := downloadToTemp(ctx, archiveURL)
if err != nil {
    return err
}
got, err := sha256File(archivePath)
if err != nil {
    return err
}
if got != want {
    return fmt.Errorf("modeld checksum mismatch: got %s want %s", got, want)
}
```

## Install Location

The default install root should be per-user and independent of workspace
`.contenox/` directories:

```text
~/.contenox/modeld/<version>/<platform>/
```

Example:

```text
~/.contenox/modeld/v0.32.5/linux-amd64/
  modeld
  modeld.bin
  lib/llamacpp/...
  modeld-libs/...
  manifest.json
  LICENSES/...
```

Windows:

```text
%USERPROFILE%\.contenox\modeld\v0.32.5\windows-amd64\
  modeld.cmd
  modeld.exe
  lib\llamacpp\...
  modeld-libs\...
```

After extraction, setup should persist or derive the installed binary path. A
simple first step is to set the local process path for the current setup run and
print the path:

```text
~/.contenox/modeld/<version>/<platform>/modeld
```

Follow-up work can decide whether to persist a `modeld-bin` config key, generate a
shim on PATH, or start a background service.

## Safe Extraction

Archive extraction must reject unsafe paths:

- absolute paths
- empty paths
- `..` path traversal
- symlinks or hardlinks that escape the destination
- files that would overwrite outside the destination

The extractor should create a fresh staging directory:

```text
~/.contenox/modeld/.staging/<version>-<platform>-<random>/
```

Then atomically replace or rename into:

```text
~/.contenox/modeld/<version>/<platform>/
```

On Windows, replacement may fail if a process is using the old binary. In that
case setup should leave the existing install in place and explain that `modeld`
must be stopped before updating.

## Validation

After extraction, run:

```bash
<install>/modeld version --json
```

For Windows:

```text
<install>\modeld.cmd version --json
```

Validation must assert:

- command exits 0
- JSON parses
- `version` matches the CLI version, unless a dev override is active
- `backends` contains `llama` for provider `llama`
- `backends` contains `openvino` for provider `openvino`

If validation fails, setup should not mark the binary as installed. It should keep
the error visible and print source-build fallback instructions.

This validation is about compiled package capability only. It is not backend
selection. Live backend selection happens when `modeld serve` starts.

## Setup UX

For local providers, `setupProviders` labels should stop saying "source build" as
the primary path. They should communicate local `modeld`:

```text
Llama.cpp GGUF (local modeld)
OpenVINO IR (local modeld)
```

Happy path output:

```text
Local modeld provider selected: openvino
Checking prebuilt modeld package for v0.32.5 linux-amd64...
Found modeld-v0.32.5-linux-amd64.tar.gz
Downloading 243 MB...
Verified checksum e4fcaf5a...
Installed modeld to ~/.contenox/modeld/v0.32.5/linux-amd64/modeld
Validated modeld version v0.32.5 with compiled backends: llama, openvino
```

No artifact path:

```text
No prebuilt modeld package is published for v0.32.5 darwin-arm64 yet.
Use the source-build path for now:
...
```

Network failure path:

```text
Could not check prebuilt modeld packages: request timed out.
Config is saved. You can rerun `contenox setup` or use the source-build path:
...
```

Checksum failure path:

```text
Downloaded modeld package failed checksum verification.
The package was not installed.
```

## Local Provider Registration

The existing setup flow does not register local `llama` or `openvino` backends in
the same way hosted providers are registered. That can remain unchanged for the
artifact detection step if runtime discovery continues to use `modeld` leases.

However, after installation setup should make the next action clear:

```text
Start modeld:
  ~/.contenox/modeld/v0.32.5/linux-amd64/modeld serve

Install a local model:
  contenox model pull qwen2.5-coder-0.5b-ov
```

`modeld` autodetects the live backend at startup. Service management can replace
this manual start command later.

## Error Semantics

Suggested internal errors:

```go
var (
    ErrNoOfficialVersion = errors.New("modeld setup: CLI version is not an official release")
    ErrNoPrebuiltArtifact = errors.New("modeld setup: no prebuilt artifact for version/platform")
    ErrChecksumMismatch = errors.New("modeld setup: checksum mismatch")
    ErrUnsupportedPlatform = errors.New("modeld setup: unsupported platform")
    ErrBackendMissing = errors.New("modeld setup: installed modeld lacks required compiled backend")
)
```

`ErrNoOfficialVersion`, `ErrNoPrebuiltArtifact`, `ErrUnsupportedPlatform`, and
transient network errors should fall back to source-build instructions.

`ErrChecksumMismatch` should be treated as a hard failure: do not silently continue
as if the package is unavailable.

## Package Boundary

Create an internal package so setup code stays readable:

```text
runtime/internal/modeldinstall/
  artifact.go      # version/platform/name/url resolution
  remote.go        # HTTPS .sha256 check and archive download
  checksum.go      # sha256 parsing and verification
  extract_tar.go   # safe tar.gz extraction
  extract_zip.go   # safe zip extraction
  install.go       # staging, atomic install, validation
  probe.go         # existing modeld candidates and version --json
```

`runtime/contenoxcli/setup_cmd.go` should call this package from the `llama` and
`openvino` branches instead of printing source-build instructions immediately.

## Tests

Unit tests:

- Artifact name resolution for linux/darwin/windows.
- Official-version detection.
- Base URL override only affects tests/dev.
- `.sha256` parser accepts the current checksum format.
- Checksum mismatch is rejected.
- 404 on `.sha256` maps to `ErrNoPrebuiltArtifact`.
- 403 on `.sha256` reports a public download configuration problem.
- Safe tar extraction rejects absolute paths and `..`.
- Safe zip extraction rejects absolute paths and `..`.
- Capability validation accepts `llama` when backends are `["llama", "openvino"]`.
- Capability validation rejects `openvino` when backends are only `["llama"]`.

Setup tests:

- Local provider setup prints prebuilt-check text before source-build fallback.
- With a fake HTTP server and fake archive, setup installs and validates modeld.
- With no fake artifact, setup keeps the existing source-build guidance.
- Current model choices remain visible after install/fallback.

Manual verification:

```bash
curl -fsSL \
  https://contenox-modeld-artifacts-573643652148.s3.amazonaws.com/modeld/v0.32.5/modeld-v0.32.5-linux-amd64.tar.gz.sha256
```

Expected current checksum:

```text
e4fcaf5a6ffba8c2e28d7182914c48aeb767b7c46a3ffa0d60b19267487e0066  modeld-v0.32.5-linux-amd64.tar.gz
```

## Open Questions

- Whether setup should persist a `modeld-bin` config key or derive the path from
  version/platform every time.
- Whether setup should start `modeld serve` immediately after install or only
  print the normal start command until service management lands.
- Whether CLI patch releases may use a compatible older `modeld` package, or
  whether exact version matching remains mandatory.
- Whether public downloads should move from direct S3 URLs to CloudFront before
  wide release.
