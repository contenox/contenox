# Blueprint: modeld Release Artifacts

> Status: packaging blueprint. Scope is only how official `modeld` binaries are
> built, packaged, checksummed, and uploaded. It deliberately does not define the
> later `contenox setup` installer UX, service management, or daemon lifecycle.

## Problem

`contenox` ships today; `modeld` does not.

The CLI is pure Go (`CGO_ENABLED=0`), so `.github/workflows/release.yml`
cross-compiles it to every supported platform from a single Linux runner with no
C toolchain. The release matrix even carries a comment promising that the native
inference backends "live in the separate modeld binary, which has its own CGO
release" — but that release does not exist. `modeld` is currently unreleased.

`modeld` cannot ride the `contenox` path because it is a CGO binary linked against
heavyweight native runtimes:

- llama.cpp, built from a pinned commit (`mk/llama-flags.mk` `LLAMA_CPP_COMMIT`)
  with CMake and a C++ toolchain.
- OpenVINO GenAI, resolved from a Python virtualenv and a matching source worktree
  (`mk/openvino-flags.mk`, `OPENVINO_GENAI_VERSION`).

So today the only way a user gets `modeld` is to clone the repository and run
`make deps-modeld build-llamacpp-runtime build-modeld`, which needs CMake, a C++
toolchain, optionally CUDA, and Python. `docs/modeld-source-build.md` documents
exactly that manual, single-platform, developer-built path. That is the
distribution-side "CGO wall" this blueprint removes.

Two constraints shape the release design:

1. **Native dependencies are too heavy to rebuild on every tag.** Building llama.cpp
   and OpenVINO from source, per platform, with CUDA / macOS / Windows native
   toolchains, would make every tagged release slow and fragile. Instead, native
   dependency bundles are produced out of band, and the release job only links
   Go/CGO against an extracted bundle.

2. **A release must never silently ship fewer backends.** The development build
   probes whether the OpenVINO SDK is present (`MODELD_HAVE_OPENVINO`,
   `Makefile:25`) and silently drops to llama-only build tags when it is missing.
   That is correct for local development and wrong for a release. The release
   target must fail loudly when an expected backend or library is absent.

## Goal

Each version should publish a standalone `modeld` package per supported platform to
the S3 store (not GitHub Releases), keyed by version:

```text
s3://<bucket>/<release-prefix>/vX.Y.Z/
  modeld-vX.Y.Z-linux-amd64.tar.gz   (+ .sha256)
  modeld-vX.Y.Z-linux-arm64.tar.gz   (+ .sha256)
  modeld-vX.Y.Z-darwin-amd64.tar.gz  (+ .sha256)
  modeld-vX.Y.Z-darwin-arm64.tar.gz  (+ .sha256)
  modeld-vX.Y.Z-windows-amd64.zip    (+ .sha256)
```

`modeld` remains a separate native binary. It is not embedded into `contenox`,
and normal `contenox` commands do not build or link native inference code.

## Release Build Shape

For the reasons in [Problem](#problem), the release pipeline has two layers:

1. Native dependency bundles, produced outside the normal release job.
2. Final `modeld` packages, produced during the release job by linking Go/CGO
   against the extracted native dependency bundle.

This avoids rebuilding heavyweight native runtimes on every release while still
letting the release job produce the final versioned `modeld` executable. The split
is also what lets the release target be deterministic: it consumes a fixed,
versioned dependency tree instead of whatever happens to be installed on the runner.

## Native Dependency Bundles

A native dependency bundle exists per supported platform/variant. These are build
inputs, not user-facing packages.

Bundle production is **decentralized**: no single device can build every variant — a
Linux box cannot produce the Windows or macOS native deps, and only a CUDA host can
build the CUDA llama plugin. So each device builds the variants it *can*
(`make bundle-modeld-deps`, dispatching to the per-OS `scripts/modeld-deps-bundle-<os>.sh`)
and pushes them to S3 (`make push-modeld-deps`). S3 accumulates the union of all contributed variants, and
the release job downloads whatever it needs per platform — including variants the
release runner itself cannot build.

Bundles are stored on S3 as **plain files (not archived)** via `aws s3 sync`, keyed by
platform and a fingerprint of the build inputs:

```text
s3://<bucket>/<prefix>/<platform>/<fingerprint>/
  manifest.json
  bundle.env
  llama/...
  openvino/...
  licenses/...
```

The fingerprint (see [Fingerprinting](#fingerprinting-and-dedup)) is a hash of the
pinned inputs — llama.cpp commit, OpenVINO GenAI version, platform, accelerator
profile, runtime ABI, build type. It is computed from identifiers only, so a device
can compute *another* platform's fingerprint and locate that variant on S3 without
being able to build it. A fingerprint already present on S3 is skipped, so we never
rebuild or re-upload a version we already have.

Each extracted bundle is a platform sysroot with the headers and libraries needed
to build and package `modeld`. The OpenVINO GenAI subtree preserves the source-
relative paths the CGo flags reference (`src/cpp/...`, `build/_deps/...`) so
`package-modeld-release` only has to re-point the root variables at the bundle:

```text
modeld-deps-<platform>-<variant>/
  manifest.json
  bundle.env

  llama/
    ref/
      common/
      vendor/
    runtime/
      include/
      lib/
        libcommon.a
        libllama.*
        libggml.*
        libggml-base.*
        libggml-cpu*
        libggml-cuda*        # linux-amd64 only (see CUDA note below)

  openvino/
    openvino/
      include/
      libs/
        libopenvino.*
    genai/
      libopenvino_genai.*               # OPENVINO_GENAI_PKG (the prebuilt lib)
      src/cpp/include/                  # OPENVINO_GENAI_SRC: headers the bridge -I's
      src/cpp/src/
      build/_deps/
        minja-src/include/
        gguflib-src/
    tokenizers/
      lib/
        libopenvino_tokenizers.*

  licenses/
    llama.cpp/
    openvino/
    openvino-genai/
    openvino-tokenizers/
```

Platform library suffixes follow the platform: `.so` on Linux, `.dylib` on
macOS, and `.dll` plus import libraries where required on Windows.

The dependency bundle should not contain:

```text
modeld
contenox
downloaded AI model weights
Python virtualenvs
full native build directories
Go module cache
toolkit installers
```

### CUDA note

The `linux-amd64` bundle ships the CUDA ggml plugin (`libggml-cuda.so`) but does
**not** bundle the CUDA runtime (`libcudart.so.12`); see
[Decisions → CUDA](#cuda). The bundle therefore depends on a host CUDA runtime and
driver for GPU execution. Because llama.cpp is built with `GGML_BACKEND_DL=ON`
(`Makefile.llamacpp-direct:31`), the CUDA backend is a runtime-loaded plugin: when
`libcudart.so.12` or a usable driver is absent, loading `libggml-cuda.so` must fail
**non-fatally** and `modeld` must continue on the CPU backend. Graceful CPU fallback
on a CUDA-built bundle is a release acceptance requirement, not an optimization.

## Dependency Manifest

Each native dependency bundle includes `manifest.json`:

```json
{
  "platform": "linux-amd64",
  "variant": "cuda-hip",
  "fingerprint": "74009410359c1a223e0bc0e7556c24ddedae0aafada90b554a3984f37752d7f8",
  "llama_cpp_commit": "ee3a5a10adf9e83722d1914dddc56a0623ececaf",
  "openvino_genai_version": "2026.2.0.0",
  "accelerator": { "cuda": true, "hip": true },
  "openvino": true,
  "libraries": [
    "llama",
    "ggml",
    "openvino",
    "openvino_genai",
    "openvino_tokenizers"
  ]
}
```

`llama_cpp_commit` and `openvino_genai_version` must match the pinned values in
`mk/llama-flags.mk` and `mk/openvino-flags.mk`. The `accelerator` block records
which ggml accelerator plugins the bundle was built with, mirroring the llama
runtime build stamp (`cuda=ON/OFF hip=ON/OFF`, written at
`Makefile.llamacpp-direct:105`); it documents per-platform whether `libggml-cuda.*`
or `libggml-hip.*` is expected to be present. `fingerprint` and `variant` identify
the exact build inputs (see [Fingerprinting](#fingerprinting-and-dedup)). A
machine-readable `bundle.env` companion carries the same fields for shell consumers
(the release path sources it instead of parsing JSON).

The release job verifies this manifest before building. If a platform's official
artifact is expected to include OpenVINO support, missing OpenVINO metadata or
libraries must fail the release build. Likewise, if `accelerator.cuda` is true, the
CUDA plugin must be present in the bundle or the release fails.

### Fingerprinting and dedup

`scripts/modeld-deps-fingerprint.sh` (target: `make modeld-deps-fingerprint`) hashes
the pinned build inputs in a fixed canonical order: platform, llama.cpp commit, llama
build type, runtime ABI, CUDA on/off, HIP on/off, OpenVINO on/off, and OpenVINO GenAI
version. Two properties matter:

- **Pin-only.** It uses identifiers, not built artifacts, so it can be evaluated
  before the expensive runtime build — and a device can compute the fingerprint for a
  platform it cannot build (override `MODELD_PLATFORM` / `MODELD_FP_*`) to address that
  variant's S3 location.
- **Single definition.** The producer (from the llama runtime stamp) and the pin-only
  target compute the same value, so the manifest's fingerprint always matches what a
  lookup computes.

This is what makes the decentralized model safe: `push-modeld-deps` skips a fingerprint
already on S3 (no rebuild/re-upload of a version we have), and the release resolves each
platform's expected fingerprint to fetch the right variant.

## Makefile Contract

Add a release packaging target, `package-modeld-release`, that consumes an extracted
native dependency bundle instead of rebuilding native dependencies:

```bash
MODELD_DEPS_ROOT=/path/to/modeld-deps-linux-amd64 make package-modeld-release
```

### Per-OS targets and backend matrix

Native library names, link flags, wrapper, and archive format differ per OS, so both
the bundle producer and the packager have one target per OS, in separate scripts
(`scripts/modeld-deps-bundle-<os>.sh`); the bare targets dispatch to the host OS:

```text
bundle-modeld-deps[-linux|-darwin|-windows]
package-modeld-release[-linux|-darwin|-windows]
```

The compiled backend set is per platform — and OpenVINO is **not** universal:

| Platform | Backends | Accelerator | OpenVINO |
| --- | --- | --- | --- |
| linux-amd64 | llama.cpp + OpenVINO | CUDA / HIP (DL plugins) | required (`MODELD_RELEASE_OPENVINO=1`) |
| darwin-arm64 | llama.cpp | Metal | **off** — OpenVINO GenAI is not supported on Apple Silicon |
| windows-amd64 | llama.cpp + OpenVINO | CUDA | required (MinGW toolchain) |

Apple Silicon is llama + Metal: the darwin producer is llama-only and the darwin
packager defaults `MODELD_RELEASE_OPENVINO=0`, so the Mac path is never gated on
OpenVINO. CUDA/HIP/Metal are not separate artifacts — they ride the llama runtime the
device built (recorded as the bundle `variant` and `accelerator`).

This target is a **variant of the existing `package-modeld`** (`Makefile:153`).
`package-modeld` already produces the relocatable bundle described in
[Final Package Layout](#final-package-layout), but it derives its native inputs from
local CMake output and Python-virtualenv introspection
(`OPENVINO_PKG` via `python -c 'import openvino'`), and it computes
`MODELD_HAVE_OPENVINO` by probing. The release variant differs in two ways:

1. It maps the build variables to the fixed, versioned dependency tree under
   `MODELD_DEPS_ROOT` instead of venv/local introspection:

   ```text
   LLAMA_CPP_REF_DIR       = $(MODELD_DEPS_ROOT)/llama/ref
   LLAMA_RUNTIME_DIR       = $(MODELD_DEPS_ROOT)/llama/runtime
   LLAMA_RUNTIME_LIB_DIR   = $(MODELD_DEPS_ROOT)/llama/runtime/lib

   OPENVINO_PKG            = $(MODELD_DEPS_ROOT)/openvino/openvino
   OPENVINO_GENAI_SRC      = $(MODELD_DEPS_ROOT)/openvino/genai
   OPENVINO_GENAI_PKG      = $(MODELD_DEPS_ROOT)/openvino/genai
   OPENVINO_TOKENIZERS_LIB = $(MODELD_DEPS_ROOT)/openvino/tokenizers/lib
   ```

2. It **hard-fails** on a missing expected backend instead of silently degrading the
   build tags.

The release target must:

1. Refuse to run when `MODELD_DEPS_ROOT` is unset.
2. Verify required files from the dependency bundle exist, and fail if an expected
   backend (per the bundle manifest) is missing — never fall back to a reduced tag set.
3. Build `./cmd/modeld` with the expected build tags.
4. Package `modeld` with the runtime libraries needed at execution time.
5. Produce one archive and one checksum file under `dist/`.

The normal development targets continue to build native dependencies locally and may
silently reduce the backend set — that is acceptable for development only:

```bash
make build-modeld
make run-modeld
make package-modeld
```

The release target is separate precisely so release builds are deterministic and do
not silently fall back to a reduced backend set.

## Final Package Layout

Unix packages should extract to one directory. This matches the current
`package-modeld` output (wrapper script, native binary, llama.cpp runtime, and the
OpenVINO libraries when compiled in):

```text
modeld-vX.Y.Z-linux-amd64/
  modeld                 # POSIX sh wrapper: sets LD_LIBRARY_PATH + rpath, execs modeld.bin
  modeld.bin             # native daemon
  lib/
    llamacpp/
      ...                # libllama, libggml*, ggml backend plugins
  modeld-libs/
    ...                  # OpenVINO runtime libraries (present when OpenVINO compiled in)
  manifest.json
  LICENSES/
```

The Unix `modeld` wrapper is what makes the bundle relocatable: it resolves its own
directory and exports `LD_LIBRARY_PATH` / `CONTENOX_LLAMA_BACKEND_DIR` before exec'ing
`modeld.bin` (`Makefile:163-174`). Keep the whole directory together; running the
wrapper alone does not work.

Windows has no shell-wrapper equivalent, so the Windows package must solve DLL
resolution explicitly:

```text
modeld-vX.Y.Z-windows-amd64/
  modeld.exe             # native daemon
  lib/
    llamacpp/
      ...                # *.dll for llama.cpp / ggml backends
  modeld-libs/
    ...                  # OpenVINO *.dll (present when OpenVINO compiled in)
  manifest.json
  LICENSES/
```

On Windows, the loader searches the directory of the running `.exe` first, so the
package must either (a) place the required DLLs directly beside `modeld.exe`, or (b)
ship a small launcher (`modeld.cmd` / a wrapper `modeld.exe`) that prepends `lib\llamacpp`
and `modeld-libs` to `PATH` (or calls `SetDllDirectory`/`AddDllDirectory`) before
launching the daemon. The packaging target must produce one of these so the Windows
bundle is relocatable without the user editing `PATH`.

The final package `manifest.json` records the modeld version, platform, compiled
backends, native dependency versions, accelerator profile, and archive checksum inputs
used to build the package.

## Release Procedure

modeld is **not** released through GitHub Actions / `release.yml`. That workflow builds
the pure-Go `contenox` CLI and the VS Code extension. modeld is a device-driven flow over
the S3 store: devices build native dep bundles for the variants they can and push them;
packaging pulls a bundle and links against it; final packages are pushed back to S3. No
CI release job orchestrates it.

The CGo link cannot meaningfully cross-compile, so the final package for a platform is
built on a device of that platform (or with that platform's cross toolchain) — which has,
or pulls from the store, that platform's dep bundle. The store is what lets a platform's
package be built even though no single device can build every platform's deps.

Per platform, the procedure is:

1. Have the dep bundle: `make bundle-modeld-deps` on a device that can build it, or
   `make pull-modeld-deps` to fetch it from the store. `pull-modeld-deps` computes the
   expected fingerprint from the pin profile and fails clearly if that variant is not in
   the store yet ("build it on a `<platform>` device first").
2. `make check-modeld-deps-bundle MODELD_DEPS_ROOT=...` verifies the bundle manifest and
   required libraries (hard-fails if an expected backend is absent).
3. `make package-modeld-release MODELD_DEPS_ROOT=...` links against the bundle and
   smoke-tests the package (`modeld version --json`), asserting the packaged binary runs
   **and reports the expected backend set** (see below).
4. `make push-modeld-release` uploads the package + checksum to the store under the
   version key.

Every step except the literal upload runs locally with no S3: the store backend is chosen
by URI scheme (`scripts/modeld-store.sh`), so pointing `MODELD_DEPS_S3_URI` /
`MODELD_RELEASE_S3_URI` at a local directory exercises the full push → dedup → pull →
package → push round-trip without credentials. Only the `aws s3` transfer itself needs a
real bucket.

### Smoke test must prove the backend set

`modeld status --data-root <tmpdir>` (the flag exists at `cmd/modeld/main.go:50`)
only proves the binary links and runs — `status` just inspects the lease file and needs
no running daemon. It does **not** prove that OpenVINO or CUDA were actually compiled in,
which is exactly the silent-degradation failure mode this blueprint exists to prevent.

`modeld version --json` (`cmd/modeld/version.go`) reports the stamped release version and
the exact compiled-in backend set without loading native libraries or claiming the lease:

```json
{
  "version": "v0.32.5",
  "backends": ["llama", "openvino"],
  "backend_info": {
    "llama": { "llama_cpp_commit": "ee3a5a10..." },
    "openvino": { "openvino_genai_version": "2026.2.0.0" }
  }
}
```

The release smoke step runs `modeld version --json` against the freshly packaged binary
and asserts `backends` (and `backend_info`) match the bundle manifest, so a build can
never silently ship fewer backends or a mismatched native version than the manifest
claims. The version, llama.cpp commit, and OpenVINO GenAI version are stamped at link time
by the Makefile ldflags (`MODELD_VERSION_LD_FLAGS`, `MODELD_LLAMA_LD_FLAGS`,
`MODELD_OPENVINO_LD_FLAGS`).

The important contract is that packaging consumes a prebuilt native dependency bundle; it
never rebuilds llama.cpp or OpenVINO from source.

### Relationship to the contenox release

This is separate from the `contenox` / VS Code release (`.github/workflows/release.yml`
and `docs/blueprints/ci-release-hardening.md`, which cover the GitHub-Actions tag-gated
publish of the pure-Go CLI and the extension). modeld binaries are not GitHub-Release
assets; they live in the S3 store. The two share the version in `runtime/version/version.txt`
but are produced and published by different mechanisms.

## Decisions

These were open questions in earlier drafts. The blueprint now commits to a direction;
the remaining deferred item (platform code signing) is called out as a follow-up. The
`modeld version --json` smoke command is implemented (`cmd/modeld/version.go`).

### Bundle storage and versioning

Store native dependency bundles on **S3 as plain files** (not archives), keyed by
`<platform>/<fingerprint>/` and uploaded with `aws s3 sync` (`make push-modeld-deps`).
S3 — not GitHub Releases — because production is decentralized: each device contributes
the variants it can build (a Linux box cannot build the Windows/macOS native deps; only
a CUDA host builds the CUDA plugin), and S3 holds the accumulated union the release
draws from. Plain files let the release fetch only the variant it needs per platform.

The fingerprint of the pinned inputs (see
[Fingerprinting](#fingerprinting-and-dedup)) is the identity: a variant already present
on S3 is never rebuilt or re-uploaded, and a device can compute another platform's
fingerprint to locate that variant without building it. Bundles are rebuilt only when a
pinned input (commit, version, accelerator profile, ABI) changes — never per tag.

### Final package storage

Final `modeld` packages are stored on **S3, not GitHub Releases**, under a version key
(`make push-modeld-release`). The transfer goes through the same store wrapper
(`scripts/modeld-store.sh`), so the publish flow is testable against a local directory and
only the literal `aws s3` call needs a bucket. Distribution to end users (the eventual
`contenox setup` installer) reads from this store; that UX is out of scope here.

### CUDA

CUDA ships in the **default** `linux-amd64` bundle as a single artifact — there is no
separate CUDA variant. The trade-off is that the default bundle is not self-contained: a
GPU host must provide `libcudart.so.12` and a compatible driver. CPU-only hosts run the
same bundle unchanged because the CUDA plugin is loaded lazily and CPU fallback is
non-fatal (see [CUDA note](#cuda-note)). The packaging and smoke tests must verify that
fallback path.

### Code signing

Public distribution requires platform signing, recorded here as a requirement and
deferred to a follow-up:

- macOS (`darwin-*`): Developer ID signing + notarization (Gatekeeper).
- Windows (`windows-amd64`): Authenticode signing.
- Linux: rely on published checksums, optionally Sigstore/cosign signatures.

### Windows toolchain

Pin one CGO-friendly toolchain — MinGW-w64 UCRT64 — and use it for **both** the native
dependency bundle and the final `modeld` link, so the C/C++ ABI matches across the two
layers.
