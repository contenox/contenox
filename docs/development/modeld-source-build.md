# Modeld Source Build and Packaging

`modeld` is the native local inference daemon for Contenox. It currently hosts
local adapters for llama.cpp GGUF compatibility and OpenVINO IR execution, while
the `contenox` CLI stays pure Go. Strategic accelerator work belongs in
chip-vendor runtime adapters; this document describes the source-build path for
the currently shipped native inputs.

Current distribution status:

- Normal CLI release assets ship `contenox`, not `modeld`.
- VS Code packages ship `bin/contenox`, not `modeld`.
- Local llama/OpenVINO providers therefore require a source-built `modeld`
  daemon for now.
- The dev `package-modeld` target is Linux-oriented (`.so`, rpath, shell wrapper).
  Official release packaging has per-OS targets (`bundle-modeld-deps-<os>`,
  `package-modeld-release-<os>`) for linux/darwin/windows — see
  [Cross-Platform Release Bundles](#cross-platform-release-bundles). The Linux path is
  verified end-to-end; the darwin/windows native build chain still needs porting work.

## Prerequisites

For Linux source builds:

```bash
sudo apt-get update
sudo apt-get install -y git make gcc g++ cmake python3 python3-venv
```

For CUDA-backed llama.cpp, install the CUDA toolkit so `nvcc` is on `PATH`
before building. If `nvcc` is absent, the llama.cpp runtime is CPU-only.

**Common trap:** installing the CUDA toolkit (via `apt`, runfile, etc.) does
not put `nvcc` on `PATH` by default — it typically lands in
`/usr/local/cuda/bin`, which most shells don't add automatically. `make
run-modeld` / `make build-modeld` detect this with a plain `command -v nvcc`
(`Makefile.llamacpp-direct:13`) and **silently fall back to a CPU-only
build** if it's missing — no warning, no error, the build just succeeds
without GPU support. Symptom: `nvidia-smi` shows a GPU, but the runtime
panel / `contenox doctor` reports the loaded device as CPU/`system`/`ram`
instead of the GPU.

Check before building:

```bash
command -v nvcc || echo "nvcc not on PATH — GPU build will silently be skipped"
```

Fix (adjust the path to your CUDA install, e.g. `/usr/local/cuda-13.3/bin`):

```bash
export PATH="/usr/local/cuda/bin:$PATH"
```

Add that line to your shell rc file (`~/.bashrc`/`~/.zshrc`) to make it
persistent. Once `nvcc` is on `PATH`, just re-run `make run-modeld` /
`make build-modeld` — the llama.cpp runtime build is cached by a stamp file
keyed on the CUDA flag (`Makefile.llamacpp-direct:74-84`), so a PATH change
alone is enough to trigger an automatic rebuild with `-DGGML_CUDA=ON`; no
`make clean` needed. Confirm it worked by checking for `direct llama.cpp
runtime (cuda=ON ...)` in the build output.

## Clone the Matching Source

Use the same tag as your installed `contenox` CLI when possible:

```bash
VERSION="$(contenox version | awk '{print $3}')"
git clone --branch "$VERSION" --depth 1 https://github.com/contenox/runtime.git contenox-runtime
cd contenox-runtime
```

For unreleased development, use `main` instead:

```bash
git clone --depth 1 https://github.com/contenox/runtime.git contenox-runtime
cd contenox-runtime
```

## Build the CLI

This is the easy, pure-Go binary:

```bash
make build-contenox
./bin/contenox --version
```

The release-style command is:

```bash
VERSION="$(tr -d '\r\n' < runtime/version/version.txt)"
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X github.com/contenox/runtime/runtime/contenoxcli.Version=$VERSION" \
  -o bin/contenox \
  ./cmd/contenox
```

## Run Modeld for llama.cpp GGUF

In one terminal:

```bash
CONTENOX_MODELD_BACKEND=llama make run-modeld
```

This builds the pinned llama.cpp runtime, builds `bin/modeld`, and starts:

```bash
bin/modeld serve
```

Leave it running. In another terminal:

```bash
contenox init llama
contenox model registry-list
contenox model pull qwen3-8b
contenox model local
contenox model list
contenox doctor
```

`model local` shows installed files. `model list` shows models that the live
daemon can describe/load.

Starter llama models:

| VRAM | Model | Q4 size | Notes |
| --- | --- | --- | --- |
| ~2 GB | `granite-3.2-2b` | ~1.5 GB | |
| ~3 GB | `qwen3-4b` | ~3 GB | |
| ~3 GB | `phi-4-mini` | ~2.5 GB | |
| ~5 GB | `gemma4-e4b` | ~5 GB | native tool format |
| ~5 GB | `qwen3-8b` | ~5 GB | |
| ~5 GB | `deepseek-r1-0528-qwen3-8b` | ~5 GB | |
| ~8 GB | `gemma4-12b` | ~8 GB | |
| ~12 GB | `gpt-oss-20b` | ~12 GB | |
| ~19 GB | `qwen3-coder-30b-a3b` | ~19 GB | |

## Run Modeld for OpenVINO IR

OpenVINO needs its Python-wheel SDK and GenAI sources prepared first:

```bash
make deps-modeld
CONTENOX_MODELD_BACKEND=openvino make run-modeld
```

Leave it running. In another terminal:

```bash
contenox init openvino
contenox model pull qwen2.5-coder-0.5b-ov
contenox model local
contenox model list
contenox doctor
```

OpenVINO device selection is controlled by OpenVINO/modeld environment. Start
with defaults unless you are validating a specific CPU/GPU/NPU setup.

Starter OpenVINO models:

| Model | Size | Notes |
| --- | --- | --- |
| `qwen2.5-coder-0.5b-ov` | ~350 MB | fastest smoke test |
| `qwen2.5-coder-1.5b-ov` | ~900 MB | small coding model |
| `qwen3-4b-ov` | ~2.3 GB | |
| `qwen3-8b-ov` | ~4.9 GB | |
| `phi-4-mini-ov` | ~2.4 GB | |
| `gpt-oss-20b-ov` | ~12.6 GB | |

## Use Local Modeld for VS Code Autocomplete

VS Code autocomplete has its own provider/model defaults. You can keep chat on
OpenAI, Gemini, Mistral, OpenRouter, or another provider and route only ghost
text to local modeld.

For llama.cpp GGUF:

```bash
contenox model pull qwen3-coder-30b-a3b
contenox config set default-autocomplete-provider llama
contenox config set default-autocomplete-model qwen3-coder-30b-a3b
```

For OpenVINO IR:

```bash
contenox model pull qwen2.5-coder-1.5b-ov
contenox config set default-autocomplete-provider openvino
contenox config set default-autocomplete-model qwen2.5-coder-1.5b-ov
```

Then run `Contenox: Enable Autocomplete` and `Contenox: Test Autocomplete At
Cursor` in VS Code. Autocomplete uses the FIM chain with tools disabled, so
tool-call support is not required for this path; coder/FIM quality and latency
matter more.

## Choose the Backend Mode

One `modeld` process serves one local backend mode at a time:

```bash
CONTENOX_MODELD_BACKEND=llama make run-modeld
CONTENOX_MODELD_BACKEND=openvino make run-modeld
```

If `CONTENOX_MODELD_BACKEND` is unset and several backends are compiled in,
`modeld` chooses an accelerated backend when one is detected, otherwise it falls
back to its built-in preference.

## Build a Relocatable Linux Modeld Bundle

For a shippable Linux bundle:

```bash
MODELD_DIST_DIR="$PWD/bin/modeld-linux-amd64" make package-modeld
tar -C bin -czf bin/modeld-linux-amd64.tar.gz modeld-linux-amd64
```

The output directory contains:

- `modeld`: wrapper script
- `modeld.bin`: native daemon
- `lib/llamacpp/`: llama.cpp runtime and ggml backend plugins
- `modeld-libs/`: OpenVINO runtime libraries when OpenVINO was compiled in

Do not copy only the `modeld` wrapper. Keep the whole directory together.

Run the packaged daemon:

```bash
bin/modeld-linux-amd64/modeld serve
```

Install locally:

```bash
mkdir -p "$HOME/.local/share/contenox/modeld" "$HOME/.local/bin"
tar -xzf bin/modeld-linux-amd64.tar.gz \
  -C "$HOME/.local/share/contenox/modeld" \
  --strip-components=1
ln -sf "$HOME/.local/share/contenox/modeld/modeld" "$HOME/.local/bin/modeld"
```

If `modeld` is not on `PATH`, point the runtime at it:

```bash
export CONTENOX_MODELD_BIN="$HOME/.local/share/contenox/modeld/modeld"
```

## Cross-Platform Release Bundles

Official `modeld` release work has two separate roles. Dependency producer
devices build the native dependency variants they can and push those plain-file
bundles to an S3 store. A later release assembly step pulls one of those bundles
and links/packages `modeld` from it.

The native library names and backends differ per OS, so there is one dependency
producer per OS (`scripts/modeld-deps-bundle-<os>.sh`); the bare targets dispatch
to the host OS.

Use [the modeld release runbook](modeld-release-runbook.md) for the complete
maintainer procedure: S3 bucket setup, repo-local `.env`, cross-device
dependency handoff, release assembly, upload, and verification. See
[the release blueprint](blueprints/modeld/release-artifacts.md) for the design.

| OS | Backends | Notes |
| --- | --- | --- |
| linux | llama.cpp (CPU/CUDA/HIP) + OpenVINO | verified end-to-end |
| darwin (Apple Silicon) | llama.cpp + Metal | **llama-only — no OpenVINO** (not supported on Apple Silicon) |
| windows | llama.cpp (CPU/CUDA) + OpenVINO | MinGW/UCRT or MSVC toolchain; `.dll` + DLL-next-to-exe; unverified. See `docs/development/windows-development.md` for Windows dev setup and the SSH "just build the bundle" flow on a Windows worker. |

The Makefile automatically loads a repo-root `.env` when present. A dependency
producer device needs:

```bash
AWS_REGION=us-east-1
AWS_DEFAULT_REGION=us-east-1
MODELD_DEPS_S3_URI=s3://bucket/modeld-deps
MODELD_EXPECT_OPENVINO=1
```

The release assembly checkout also needs:

```bash
MODELD_RELEASE_S3_URI=s3://bucket/modeld
```

Then, on the matching dependency producer device:

```bash
make bundle-modeld-deps
make push-modeld-deps
```

Verify the uploaded bundle:

```bash
for envf in bin/modeld-deps/*/bundle.env; do
  (
    . "$envf"
    aws s3 ls "$MODELD_DEPS_S3_URI/$MODELD_BUNDLE_PLATFORM/$MODELD_BUNDLE_FINGERPRINT/manifest.json"
  )
done
```

On a dev or release consumer machine, precheck the exact dependency profile before
building anything heavy:

```bash
make modeld-deps-profile
make check-modeld-deps-store
```

If the prebuilt bundle exists, pull and validate it instead of rebuilding
llama.cpp/OpenVINO locally:

```bash
make deps-modeld-prebuilt
```

For a local package built from the pulled prebuilt dependencies:

```bash
make package-modeld-prebuilt
```

The later release assembly step, not the dependency producer step, consumes a
prebuilt bundle and publishes the final package:

```bash
make pull-modeld-deps
DEPS_ROOT="$(make -s modeld-deps-pull-dir)"

make package-modeld-release MODELD_DEPS_ROOT="$DEPS_ROOT"
make push-modeld-release
```

For darwin, OpenVINO is off by default; override with
`MODELD_RELEASE_OPENVINO=1` only if you have OpenVINO GenAI working on the target.
For consumer preflight, override `MODELD_EXPECT_*` when you intentionally want a
different variant from the defaults, such as `MODELD_EXPECT_CUDA=OFF` or
`MODELD_PLATFORM=darwin-arm64 MODELD_EXPECT_OPENVINO=0`.
Point the S3 URIs at local directories to exercise the dependency upload and
release assembly flows without AWS credentials.

## Useful Commands

```bash
modeld status
modeld status --json
modeld serve --mem-max 8GiB --mem-reserve 2GiB
CONTENOX_MODELD_BACKEND=llama modeld serve
modeld version --json   # report the compiled-in backends
```

The daemon writes a lease under the Contenox data root, normally:

```text
~/.contenox/modeld.lease
```

The runtime reads that lease to find the active daemon.

## Common Failures

`modeld is not installed`

`contenox` cannot find `modeld` on `PATH` and `CONTENOX_MODELD_BIN` is unset.
Install the bundle or export `CONTENOX_MODELD_BIN`.

`modeld is not running`

The binary exists, but no live daemon owns the lease. Start `modeld serve`.

`No loadable models found on any live backend`

The daemon is stopped, serving the other backend mode, or cannot describe the
installed model. Run `contenox model local`, then start modeld in the matching
mode and run `contenox model list`.

`requested "openvino", this daemon serves llama`

The daemon is running in the wrong backend mode. Stop it and restart with:

```bash
CONTENOX_MODELD_BACKEND=openvino modeld serve
```
