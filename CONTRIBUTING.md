# Contributing to Contenox

Thanks for helping improve Contenox. This repository centers on the V1 runtime
surface: local CLI usage, ACP over stdio, and the VS Code extension that embeds
the same Go runtime.

## Code of Conduct

Treat contributors with respect. Keep technical disagreements concrete and
actionable.

## Architecture

Contenox is a local-first Go runtime with a thin set of host adapters:

```text
CLI / ACP / VS Code stdio bridge
    ->
Service Layer (runtime/*service/)
    ->
Task Engine (runtime/taskengine/)
    ->
Data + Integrations (lib*/ + runtime/runtimetypes/ + runtime/localtools/)
```

The Go runtime owns chains, execution state, model routing, tools, MCP worker
sessions, human-in-the-loop policy, session history, and local state. Editor
integrations should stay adapters around that runtime. They should not re-create
chain semantics in TypeScript or a separate UI server.

### V1 product boundary

The V1 public surface is:

- `contenox` CLI
- `contenox acp` for ACP clients such as Zed, JetBrains, and AionUi
- `contenox vscode-agent --stdio` through the VS Code extension

The V1 public surface does not include `contenox serve`, Beam, a browser UI,
OpenAI/Ollama-compatible proxy routes, or generated local OpenAPI docs. Do not
reintroduce those surfaces without updating `docs/blueprints/v1-feature-map.md`
and the relevant user docs first.

### Abstraction layers

**Service Layer** - each domain gets its own interface and implementation
package (`execservice`, `backendservice`, `mcpserverservice`, `stateservice`,
`hitlservice`, `localfileservice`, etc.). Services communicate through the
shared `runtimetypes.Store` interface and bus events rather than depending on
each other directly.

**Task Engine** (`runtime/taskengine/`) - the core execution model. Chains are
JSON/YAML DAGs with typed I/O (`DataType`: String, Int, JSON, ChatHistory, Any).
Task handlers (`chat_completion`, `execute_tool_calls`, `tools`, `route`,
`raise_error`, `noop`) and branch operators (`equals`, `contains`,
`starts_with`, `ends_with`, `default`, `edge_traversed_at_least`) are
declarative. New Go primitives should be rare.

**LLM Resolution** - `llmrepo.ModelRepo` handles request-side selection by
capability, provider, model, and context length. `modelrepo.Provider`
implementations handle provider-side calls for local llama.cpp, Ollama/Ollama
Cloud, OpenAI, OpenRouter, Anthropic, Mistral, Gemini, AWS Bedrock, Vertex, and
vLLM. Runtime state catalogs configured backend capabilities for selectors and
diagnostics.

**Tool System** - chains invoke tools by name and resolution happens at runtime.
Built-in providers include `local_shell`, `local_fs`, `webtools`, `echo`,
`print`, OpenAPI-backed remote tools, and MCP-backed tools. HITL policy wraps
tool execution where approval is required.

**Event-driven async** - `libbus` abstracts the local event bus. Services publish
typed events such as `task.events.step_completed`, and other services subscribe
without direct package coupling.

**Key files to orient yourself:**

| File | What it shows |
|------|---------------|
| `runtime/taskengine/tasktype.go` | Chain schema, task handlers, branch operators |
| `runtime/taskengine/taskenv.go` | Runtime tool resolution and chain execution context |
| `runtime/contenoxcli/cli.go` | CLI dispatch |
| `runtime/contenoxcli/engine.go` | CLI-local engine bootstrap |
| `modeld/` | Provider drivers and local model daemon transport |
| `runtime/vscodeagent/` | VS Code stdio bridge |
| `packages/vscode/` | VS Code extension host adapter |

## Repository structure

The `contenox` binary is the main entrypoint. Current commands include `setup`,
`init`, `chat`, `run`, `tools`, `mcp`, `backend`, `cache`, `config`, `model`,
`state`, `doctor`, `session`, `acp`, `acpx`, `vscode-agent`, `update`, and
`version`.

All AI workflow packages live under `runtime/`. Infrastructure libraries
(`libauth`, `libbus`, `libcipher`, `libdbexec`, `libkvstore`, `libroutine`,
`libtracker`) stay at the module root.

```text
cmd/contenox/          contenox binary entry point
runtime/contenoxcli/  CLI command implementations
runtime/taskengine/   chain schema and execution engine
modeld/               provider drivers and model daemon transport
runtime/llmrepo/      provider/model selection
runtime/localtools/   local shell, local filesystem, web, echo, print tools
runtime/*service/     runtime services
runtime/vscodeagent/  stdio bridge used by the VS Code extension
packages/vscode/      VS Code extension
docs/blueprints/      product and release planning notes
lib*/                 infrastructure libraries with no LLM dependency
```

### Makefile overview

Run `make help` at the repo root for the full list.

| Prefix | Purpose |
|--------|---------|
| `build-*` | CLI, modeld, llama.cpp runtime, and VS Code builds |
| `package-*` | relocatable modeld bundle and VS Code VSIX packages |
| `test-*` | Go unit tests, explicit integration suites, direct llama.cpp shim checks, and CLI help checks |
| `dev-*` | local binary and VS Code extension install helpers |
| `deps-*` | modeld dependencies, pinned llama.cpp source, OpenVINO SDK/GenAI deps, and VS Code extension dependencies |
| `clean*` | remove generated binaries, native runtime bundles, and VS Code packaging artifacts |

Version bumps and release notes for the **contenox runtime** live in
`Makefile.version` (`make -f Makefile.version help`), which edits
`runtime/version/version.txt`. The `modeld` daemon has a separate version in
`cmd/modeld/version.txt` — see [Releasing a modeld version](#releasing-a-modeld-version).

### Surface docs

Keep these files in sync when changing public surface area:

- `docs/blueprints/v1-feature-map.md`
- `docs/contenox-cli.md`
- `docs/development/modeld-source-build.md`
- `packages/vscode/README.md`
- `docs/blueprints/acp/registry-submission/README.md`

## Local development setup

### Prerequisites

- [Go](https://go.dev/doc/install) 1.25+
- `make`
- `curl`, `git`, `gcc`, `g++`, and `cmake` for direct llama.cpp builds
- Node.js 22+ and npm for the VS Code extension
- Optional: Python 3 for OpenVINO backend dependency setup
- Optional: CUDA toolkit with `nvcc` on `PATH` for direct llama.cpp CUDA builds
- Optional: an LLM provider key or local server. The default local path is a GGUF model under `~/.contenox/models/`.

### Go binary path

`go install` puts binaries in `$GOPATH/bin`, usually `~/go/bin`. Add it to your
shell:

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

Add that line to `~/.bashrc` or `~/.zshrc` if you want it permanently.

### Building the CLI

```bash
# Build the binary into ./bin/contenox
make build-contenox

# Run an example
./bin/contenox "list files in this repository"
```

Optional: `make dev-install` symlinks `contenox` to
`~/.local/bin/contenox` for development.

### Building modeld with local LLM inference

The `contenox` CLI build stays CGo-free. Native local inference lives in the
separate `modeld` daemon. The llama backend uses the Contenox-owned direct
llama.cpp shim and links against generated `.llamacpp-runtime/<profile>`
libraries. The OpenVINO backend uses the `.openvino` virtualenv plus matching
OpenVINO GenAI C++ headers.

On Debian/Ubuntu:

```bash
sudo apt-get install -y curl git gcc g++ cmake python3 python3-venv
```

For a llama daemon:

```bash
make build-modeld
CONTENOX_MODELD_BACKEND=llama make run-modeld
```

For a CUDA-backed llama daemon, install the CUDA toolkit first (`nvcc` must be
on `PATH`). The same targets include the CUDA llama.cpp backend when it is
available:

```bash
make build-modeld
CONTENOX_MODELD_BACKEND=llama make run-modeld
```

For an OpenVINO daemon:

```bash
make deps-modeld
make build-modeld
CONTENOX_MODELD_BACKEND=openvino make run-modeld
```

Relocatable daemon bundles are built from the same root Makefile:

```bash
make package-modeld
```

The bundle includes llama.cpp and adds OpenVINO when the SDK/GenAI dependencies
are present. CUDA support follows the generated llama.cpp runtime.

#### Releasing a modeld version

modeld carries its own version in `cmd/modeld/version.txt`, separate from the
runtime's `runtime/version/version.txt`. Bump each on its own; the runtime's
`make -f Makefile.version bump-*` does not touch modeld's.

| Version file | Owns |
| --- | --- |
| `runtime/version/version.txt` | contenox runtime / CLI / VS Code, git tags, CI release |
| `cmd/modeld/version.txt` | the `modeld` daemon release line |

To cut a modeld release:

1. Bump `cmd/modeld/version.txt` to a new value on modeld's own line (don't copy the
   runtime number). Every published build needs a fresh version.
2. Build and package for the target platform (below), then `make push-modeld-release`.
   All the build/package/push targets read `MODELD_VERSION` from this file.
3. Check it landed: `curl -fsS "$MODELD_RELEASE_BASE_URL/index.json"`.

Store versions are immutable — `modeld/<version>/` archives are checksum-pinned in
`index.json` and `contenox setup` verifies them. Bump to a new version rather than
overwriting one in place.

#### Cross-platform release packaging

Official release packaging is per-OS and device-driven (build native dep bundles on a
device, push to an S3 store, link a package against a pulled bundle). The native library
names and backends differ per OS, so there is one producer/packager per OS — the bare
targets dispatch to the host:

| OS | Backends | Status |
| --- | --- | --- |
| linux | llama.cpp (CPU/CUDA/HIP) + OpenVINO | verified |
| darwin (Apple Silicon) | llama.cpp + Metal (no OpenVINO) | scripts in place, native build chain unported |
| windows | llama.cpp (CPU/CUDA) + OpenVINO (MinGW or Clang/MSVC) | scripts in place, unverified |

The Windows packager selects its toolchain with `MODELD_WINDOWS_TOOLCHAIN=mingw`
(default) or `msvc` (links the OpenVINO/llama import libraries with Clang/`lld`).

For Windows, bundle the VC++ runtime so the package runs on a clean machine: copy
`msvcp140.dll`, `vcruntime140.dll`, `vcruntime140_1.dll`, and `vcomp140.dll` into
`modeld-libs/` (the launcher already has it on `PATH`). `package-modeld-release-windows`
does this from `MODELD_MSVC_REDIST_DIR`; a hand-rolled device build script must copy
them too. To check a package, run `llvm-readobj --coff-imports` over `modeld.exe` and
the bundled DLLs — every import should resolve inside the package except OS DLLs
(`kernel32`, `ntdll`, `advapi32`, `ole32`, `shell32`, `ws2_32`, `setupapi`,
`api-ms-win-crt-*`) and `opencl.dll` (provided by the GPU driver).

```bash
make bundle-modeld-deps        # this host's dep bundle (-> bundle-modeld-deps-<os>)
make package-modeld-release MODELD_DEPS_ROOT=<bundle>   # (-> package-modeld-release-<os>)
```

Point `MODELD_DEPS_S3_URI` / `MODELD_RELEASE_S3_URI` at a local directory to test the
push/pull/package flow without AWS. See
[`docs/blueprints/modeld/release-artifacts.md`](docs/blueprints/modeld/release-artifacts.md)
for the design and `docs/development/modeld-source-build.md` for source-build details.

#### Reusing prebuilt dependency bundles (dev fast path)

Compiling llama.cpp and OpenVINO GenAI from source is the slow part of a modeld
build, and you do not have to pay it on every machine. Native dependency bundles are
content-addressed by a **fingerprint** of their build inputs (llama.cpp commit, build
type, runtime ABI, CUDA/HIP/OpenVINO flags, and OpenVINO GenAI version) and stored
per `platform/fingerprint`. A device that can compile a given variant builds the
bundle once and pushes it; every other machine — including CI and dev boxes without a
native toolchain — pulls that bundle and links a package against it instead of
rebuilding. Pushes are deduplicated: a fingerprint already in the store is skipped.

Inspect what this machine needs, without building anything:

```bash
make modeld-deps-profile      # print platform, fingerprint, and the store key it maps to
make check-modeld-deps-store  # exit 0 only if that exact bundle is already in the store
```

Package a daemon from a prebuilt bundle (no local C/C++ build):

```bash
make deps-modeld-prebuilt     # preflight + pull the matching bundle for this platform
make package-modeld-prebuilt  # pull (if needed) + link the release package against it
```

Produce and publish bundles/packages (only on a device that can compile the variant):

```bash
make deps-modeld bundle-modeld-deps push-modeld-deps    # build + upload native deps (dedup by fingerprint)
make package-modeld-release MODELD_DEPS_ROOT=<pulled-bundle>
make push-modeld-release                                # upload final packages + refresh the public index.json
```

The store backend is chosen by URI scheme (`scripts/modeld-store.sh`): an `s3://…`
URI uses the `aws` CLI, anything else is treated as a local directory — so the whole
push/pull/package/index flow can be exercised against a local dir with no cloud
credentials. The final-package prefix is public (consumed by `contenox setup` over
plain HTTPS via `modeld/index.json`); the dependency-bundle prefix stays private.

#### Publishing artifacts built on another device

The build device and the machine with store credentials are often not the same box
(e.g. the Windows/Intel builder has no `aws` and no `make`; the Linux dev box has
both). Build the artifact on the capable device, copy it to a `make` + store host, and
publish with the push targets below. Publish through the push targets rather than
`aws s3 cp` directly — they write the `.sha256`/`build.json` sidecars and refresh
`index.json` in step.

Native dependency bundle (`bundle-modeld-deps` output — a directory with `bundle.env`
+ `manifest.json` + `llama/` + `openvino/`):

```bash
# copy <device>/…/modeld-deps-<platform>-<variant>/ into bin/modeld-deps/ here, then:
make push-modeld-deps          # reads each bin/modeld-deps/*/bundle.env, uploads to
                               # modeld-deps/<platform>/<fingerprint>/ (skips if present)
```

Final package (the `dist/modeld-<version>-<platform>.{tar.gz,zip}` plus its `.sha256`
and `.build.json` sidecars produced by `scripts/modeld-package-release.sh`):

```bash
# copy the archive + .sha256 + .build.json into dist/ here, then:
make push-modeld-release       # validates sidecars, uploads modeld/<version>/…,
                               # then refreshes modeld/index.json
```

Notes:
- `MODELD_VERSION` (hence the `modeld/<version>/` key) comes from
  `cmd/modeld/version.txt`; see [modeld versioning and publishing](#modeld-versioning-and-publishing).
- The bundle fingerprint depends on the **llama runtime ABI**. The Windows MSVC
  toolchain builds a distinct ABI (`dl-v1-msvc`), so a consumer that wants to pull it
  must request the same ABI (`MODELD_EXPECT_RUNTIME_ABI=dl-v1-msvc`); the default
  (`dl-v1`) computes a different fingerprint and will not find an MSVC bundle.
- The Windows box has no `make`, so its package is built with the checked-in device
  script rather than `make package-modeld-release`; the dep-bundle production and all
  publishing still go through the standard targets from a `make`-capable host.

### VS Code extension development

The extension packages a platform-specific `bin/contenox` runtime and talks to it
over header-framed JSON-RPC stdio.

Common local checks:

```bash
make deps-vscode
make package-vscode
```

Lower-level extension checks:

```bash
cd packages/vscode
npm ci
npm run check
npm run package
npm run package:check -- artifacts/<generated-vsix-name>.vsix
```

Install the local VSIX into VS Code:

```bash
make dev-install-vscode
```

The Marketplace build and publish workflow is `.github/workflows/vscode-marketplace.yml`.
It expects `VSCE_PAT` to be present as a GitHub Actions secret until Marketplace
publishing moves away from PATs.

## Running tests

Before submitting a pull request, run the checks that match your change.

Fast path, matching CI:

```bash
make test-unit
```

Full Go suite, including any system tests that are not separately gated:

```bash
make test
```

Targeted system suites are explicit because some use local services or containers:

```bash
make test-system
```

vLLM container tests are hidden behind an opt-in gate:

```bash
make test-vllm
```

Direct llama.cpp shim check:

```bash
make test-llamacpp-direct
```

CLI package and help drift checks:

```bash
make test-contenox-verbose
make test-contenox-help
```

Optional race detector:

```bash
go test -race ./... -run '^TestUnit_'
```

For VS Code extension changes, run:

```bash
make package-vscode
```

If command names, flags, README examples, or user-facing help changed, also run
`make test-contenox-help` and update the relevant docs.

## Pull request guidelines

1. Open an issue first for major feature or architecture changes.
2. Branch from `main` with a descriptive name such as `feature/xyz`,
   `fix/abc`, or `docs/def`.
3. Use clear commit messages. Conventional Commit prefixes are preferred.
4. Run `gofmt` on Go changes.
5. Keep docs and blueprints in sync with public-surface changes.
6. Keep generated artifacts out of commits unless the release process requires
   them.

## Code conventions

### Go style

- Prefer self-documenting code. Add short comments only for non-obvious
  invariants, protocol behavior, security boundaries, or tricky edge cases.
- Service constructors accept interfaces. Wire concrete implementations in
  `runtime/contenoxcli/engine.go`.
- Keep chain behavior declarative. Business logic belongs in chain definitions
  unless a new primitive is genuinely needed in `taskengine`.
- Tool exposure must be explicit. `execute_config.tools` omitted, `null`, or
  `[]` exposes no registry tools. Use `["*"]` only when a chain intentionally
  opts into all registered tools, and prefer narrow allowlists such as
  `["local_fs"]`.
- Runtime allowlists may restrict task allowlists but must not expand them.
- Wide interfaces are a smell. New code should accept the narrowest interface
  slice it actually needs.
- Do not reintroduce `contenox serve`, Beam/browser UI, HTTP model proxy
  compatibility routes, generated local OpenAPI docs, or API test harnesses
  without an explicit roadmap change.
