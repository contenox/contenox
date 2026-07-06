# Blueprint: Decouple modeld versioning; resolve by protocol compatibility

> Status: architecture blueprint. Scope is how the `contenox` CLI/runtime selects,
> installs, validates, and discovers a prebuilt `modeld` daemon. It **supersedes**
> the version-selection model in
> [modeld setup artifact detection](setup-artifact-detection.md) — specifically
> its "exact-version only" matching, its "No latest channel" non-goal, and its
> open question on patch-release compatibility. Release production mechanics from
> [modeld release artifacts](release-artifacts.md) still apply; only the
> version/keying/resolution contract changes.

## Problem

`modeld` has no version identity of its own. The entire toolchain stamps a single
file:

```text
runtime/version/version.txt        # the only version file in the repo
  -> Makefile MODELD_VERSION         (modeld binary stamp + release artifact name)
  -> contenoxcli.CLIVersion()        (CLI version)
  -> VSCODE_VERSION                  (extension version)
```

So a `modeld` build is labeled with the **CLI's** release tag, and setup resolves
the artifact as `modeld-<CLIVersion>-<platform>`. This couples a heavy, separately
built CGO/GPU daemon (~254 MB, llama.cpp + OpenVINO + CUDA/HIP/Metal) to the CLI's
release cadence, which is wrong:

- A CLI-only or docs-only release bumps the version, and setup then demands a
  `modeld-<new>` artifact that nobody rebuilt — even though `modeld` is byte-for-byte
  unchanged.
- If that artifact is not republished, **every fresh install breaks** and falls back
  to source-build, while a perfectly usable older `modeld` sits published and is
  refused on a version-string mismatch.

This is not hypothetical: CLI `v0.32.6` shipped via `install.sh` while only
`modeld/v0.32.5/…` exists in the bucket. The exact-version resolver looked for
`modeld/v0.32.6/…`, got an S3 `403` (a missing key on a non-listable bucket reads as
`403 AccessDenied`, not `404`), and dumped users to the source-build path.

There is also **no compatibility contract**. The only thing that must actually match
between the runtime and `modeld` is the **transport/gRPC session protocol**
(`runtime/transport`). Today nothing declares or checks it; the version string is a
proxy that does not mean what it pretends to.

## Goals

- Give `modeld` an **independent version**, bumped only when `modeld` changes.
- Define a **transport protocol version** as the real compatibility axis.
- Resolve `modeld` by **"newest published build that is protocol-compatible for this
  platform and has the needed backend"** — never by version-string equality.
- Replace exact-key probing with a single authoritative **public index** so
  availability is unambiguous (no more `403`-means-maybe).
- Let the CLI and `modeld` release on **independent cadences**; a CLI bump must never
  strand installs.
- Install/validate/discover `modeld` keyed by **its own** version, surviving many CLI
  releases.

## Non-Goals

- No change to how dependency bundles / final packages are produced
  ([modeld release artifacts](release-artifacts.md)); only versioning, keying,
  the index, and resolution change.
- No semantic-version range negotiation beyond an integer protocol window.
- No runtime rebuild of native backends.
- No public bucket listing; the index is the only enumeration surface.

## The Contract

Two independent version axes and one compatibility check:

```text
modeld version         independent release tag of the daemon (e.g. v0.32.5)
transport protocol     integer; the gRPC/session wire+semantics contract
contenox build         supports a protocol window [MinProtocol .. ProtocolVersion]

compatible(modeld)  ==  MinProtocol <= modeld.protocol <= ProtocolVersion
select              ==  newest modeld version that is compatible,
                        for this platform, containing the requested backend
```

Version strings are **labels for humans and storage keys**. They are never compared
for compatibility.

## modeld's Independent Version

- New `cmd/modeld/version.txt` holds modeld's own version; bumped only when modeld
  changes (backends, llama.cpp commit, OpenVINO GenAI, native libs, or the protocol).
- `Makefile`: `MODELD_VERSION ?= $(cmd/modeld/version.txt)` (was
  `runtime/version/version.txt`). `runtime/version/version.txt` continues to drive the
  CLI and the VS Code extension only.
- `cmd/modeld/version.go` keeps stamping `version` via the existing
  `-X 'main.version=$(MODELD_VERSION)'` ldflag.

## Transport Protocol Version (the real axis)

- New `runtime/transport/protocol.go`:

  ```go
  // ProtocolVersion is the modeld gRPC/session wire+semantics contract this build
  // speaks. Bump on any breaking change to transport requests/responses or session
  // semantics. MinProtocol is the oldest daemon protocol this build still accepts.
  const ProtocolVersion = 1
  const MinProtocol      = 1

  // Supported reports whether a daemon speaking protocol p is usable by this build.
  func Supported(p int) bool { return p >= MinProtocol && p <= ProtocolVersion }
  ```

- `modeld` and the runtime both compile `runtime/transport`, so each knows the
  protocol it was built with. `modeld version --json` reports it:

  ```json
  { "version": "v0.32.5", "protocol": 1, "backends": ["llama","openvino"],
    "backend_info": { "...": {} } }
  ```

  (`versionInfo` in `cmd/modeld/version.go` gains a `Protocol int` field set from
  `transport.ProtocolVersion`.)

- Bump discipline: additive/compatible wire changes do **not** bump `ProtocolVersion`;
  breaking ones do, and `MinProtocol` advances only when old daemons are truly
  unsupportable.

## The Public Index

One authoritative document enumerates every published build. The CLI fetches exactly
this — no per-key probing, so a missing build is a clear, single signal instead of an
ambiguous `403`.

```text
https://<base>/modeld/index.json     # public-read
```

```json
{
  "schema": 1,
  "builds": [
    {
      "version":  "v0.32.5",
      "platform": "linux-amd64",
      "protocol": 1,
      "backends": ["llama", "openvino"],
      "channel":  "stable",
      "archive":  "v0.32.5/modeld-v0.32.5-linux-amd64.tar.gz",
      "sha256":   "v0.32.5/modeld-v0.32.5-linux-amd64.tar.gz.sha256",
      "size":     254622086
    }
  ]
}
```

- `archive`/`sha256` are paths relative to the `modeld/` base.
- `channel` allows a future `beta`; the default resolver uses `stable`.
- The index is regenerated and uploaded **last** by the release tooling, so it never
  references a partially-uploaded artifact.

## S3 Layout

Keyed by **modeld** version (not CLI version), plus the index:

```text
modeld/index.json
modeld/<mver>/modeld-<mver>-<platform>.tar.gz
modeld/<mver>/modeld-<mver>-<platform>.tar.gz.sha256
```

The native-dependency prefix (`modeld-deps/`) stays private and unchanged.

## Resolution Algorithm

In `runtime/internal/modeldinstall` (replacing the CLI-version exact-key logic):

```text
1. GET modeld/index.json over HTTPS (public). Missing/!public -> ErrNoIndex -> fallback.
2. candidates = builds where
       platform == runtime.GOOS+"-"+runtime.GOARCH
       && transport.Supported(build.protocol)
       && build.backends contains <selected provider>      // llama | openvino
       && channel == "stable"
3. if none: ErrNoCompatibleArtifact -> source-build fallback (honest message).
4. pick = max(candidates, by version).
5. download pick.archive + pick.sha256 -> verify sha256 -> safe-extract.
6. install to ~/.contenox/modeld/<pick.version>/<platform>/ ; write the `current` pointer.
7. validate: run `modeld version --json`; assert transport.Supported(protocol)
   && backends contains <provider>. (Version string is NOT asserted.)
```

The download/verify/safe-extract code from the artifact-detection work is reused
unchanged. The exact-CLI-version gate (`IsOfficialVersion(CLIVersion)`,
`modeld-<CLIVersion>` naming) and the `ErrPublicAccess` 403-misconfig branch are
removed; the index is the source of truth, and the artifact GET treats `403`/`404`
identically as "absent → fallback."

## Install Location and Discovery

Decoupled from the CLI version:

```text
~/.contenox/modeld/<mver>/<platform>/   modeld, modeld.bin, lib/, modeld-libs/, manifest.json
~/.contenox/modeld/current              -> pointer to the active install (path or symlink)
```

- `runtime/internal/modeldprobe` no longer derives the path from `CLIVersion`. It
  resolves in order: `CONTENOX_MODELD_BIN`, the `current` pointer, then a scan of
  `~/.contenox/modeld/*/<platform>/` choosing a build whose `version --json` reports a
  `transport.Supported` protocol, then `PATH`.
- One installed `modeld` therefore serves many CLI releases. A CLI upgrade does not
  re-trigger a download unless no compatible local build is found.

## Validation

`modeld version --json` must:

- exit 0 and parse;
- report `transport.Supported(protocol) == true`;
- include the selected provider's backend in `backends`.

Version equality is **not** a validation input. A protocol mismatch is the only
hard "incompatible" verdict; a missing backend is `ErrBackendMissing` as before.

## Build Changes

- `Makefile`: source `MODELD_VERSION` from `cmd/modeld/version.txt`; release name and
  `MODELD_RELEASE_S3_URI/<MODELD_VERSION>` keying unchanged otherwise.
- `cmd/modeld/version.go`: add `Protocol` to `versionInfo`.
- `runtime/transport/protocol.go`: new constants + `Supported`.
- `scripts/modeld-package-release.sh`: smoke gate additionally asserts the reported
  `protocol` is non-zero and `Supported`.

## Release Process and Gating

- `modeld` republishes **only when it changes**, on its own version. The CLI release
  no longer requires a same-named `modeld`.
- `push-modeld-release` uploads the archive + `.sha256`, then **regenerates and uploads
  `index.json`** (from an authenticated bucket listing or a tracked builds manifest)
  as the final step.
- Release gate (runbook): after push, anonymously `GET index.json` and `HEAD` the
  newest referenced archive + `.sha256`; fail the release if either is missing or not
  publicly readable. This is what would have caught the `v0.32.6` outage.
- A CLI release gate asserts the index contains at least one `stable`,
  protocol-compatible build per supported platform — otherwise the CLI ships knowing
  setup will fall back.

## Setup UX

```text
Local modeld provider selected: llama
Resolving a compatible modeld build (protocol 1, linux-amd64)...
Selected modeld v0.32.5 (protocol 1, backends: llama, openvino)
Downloading 243 MB...
Verified checksum e4fcaf5a...
Installed modeld to ~/.contenox/modeld/v0.32.5/linux-amd64/modeld
```

No compatible build:

```text
No prebuilt modeld build is compatible with this contenox (protocol 1, darwin-arm64).
Use the source-build path for now:
...
```

Index unreachable:

```text
Could not reach the modeld release index. Config is saved; rerun `contenox setup`,
or use the source-build path:
...
```

## Migration / Backward Compatibility

- Existing installs under `~/.contenox/modeld/<cliversion>/<platform>/` remain
  discoverable via the directory scan (validated by `version --json`), so no forced
  reinstall.
- The first `cmd/modeld/version.txt` can start at the current `modeld` version
  (`v0.32.5`) so the existing published artifact is immediately the resolved build.
- Until `index.json` exists, the CLI cannot resolve; ship the index alongside the
  first decoupled release. (Optional transitional fallback: if `index.json` is absent,
  probe `modeld/<MinProtocol-known-version>/…` — but the index should land first.)

## Corrections to the Artifact-Detection Blueprint

- "Version Selection … exact-version only" → replaced by protocol-compatible selection.
- Non-Goal "No 'latest' channel" → removed; a stable channel resolving newest
  compatible is the model.
- Open question "Whether CLI patch releases may use a compatible older `modeld`
  package" → answered: yes, always, via protocol compatibility.
- `.sha256` 404-vs-403 availability semantics → obsolete; availability comes from the
  index, and artifact `403`/`404` are treated identically as absent.

## Tests

- `transport.Supported` boundary table (below `MinProtocol`, in-window, above
  `ProtocolVersion`).
- Resolver picks newest compatible build; rejects wrong platform, wrong protocol, and
  missing-backend entries; ignores the version string entirely.
- `httptest` index + fake archive (built with the release-shaped top-level dir and a
  stub `modeld` reporting `{version, protocol, backends}`) → installs the chosen build,
  validates protocol+backend.
- Protocol-incompatible-only index → `ErrNoCompatibleArtifact` → source-build retained.
- Missing/`403` index → `ErrNoIndex` → graceful fallback (no scary message).
- `modeldprobe` discovers an install via the `current` pointer and via directory scan,
  independent of `CLIVersion`.

## Phased Plan

### Phase 0: Versioning + protocol
- Add `cmd/modeld/version.txt`; repoint `MODELD_VERSION`.
- Add `runtime/transport/protocol.go`; report `protocol` in `modeld version --json`;
  extend the release smoke gate.
- Proof: `modeld version --json` shows an independent version + a supported protocol.

### Phase 1: Index + resolver
- Define `index.json`; have `push-modeld-release` regenerate it.
- Rewrite `internal/modeldinstall` to fetch the index and select by compatibility;
  drop the exact-version gate and `ErrPublicAccess`.
- Proof: v0.32.6 CLI installs the published v0.32.5 modeld with no republish.

### Phase 2: Discovery + UX
- `current` pointer + version-independent `modeldprobe` resolution.
- Setup messaging for selected/none/unreachable.
- Proof: a CLI upgrade reuses the existing install with no re-download.

### Phase 3: Release gating
- Anonymous index/artifact verification in the runbook; CLI-release index assertion.
- Proof: a release that fails to publish a compatible modeld fails its own gate.

## Open Questions

- Should the index be per-platform (`modeld/index-<platform>.json`) to shrink the
  fetch, or one global document?
- Should `beta`/`edge` channels exist at launch, or only `stable`?
- Should the `current` pointer be a symlink (POSIX) with a file fallback (Windows), or
  a file everywhere?
- Where does the protocol version live so both the OSS runtime and modeld share it
  without a heavier dependency than `runtime/transport`?
- Should the CLI cache the index briefly to avoid refetching on repeated setup runs?
