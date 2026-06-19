# Contributing to Contenox

Thanks for helping improve Contenox. This repository is being shaped around the
V1 runtime surface: local CLI usage, ACP over stdio, and the VS Code extension
that embeds the same Go runtime.

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

The current V1 direction explicitly does not include `contenox serve`, Beam, a
browser UI, OpenAI/Ollama-compatible proxy routes, or generated local OpenAPI
docs. Do not reintroduce those surfaces without updating `ROADMAP.md` and the
blueprints first.

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
`init`, `chat`, `run`, `tools`, `mcp`, `backend`, `config`, `model`, `doctor`,
`session`, `acp`, `acpx`, and `vscode-agent`.

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
docs/blueprints/      release, product, and pruning plans
lib*/                 infrastructure libraries with no LLM dependency
```

### Makefile overview

Run `make help` at the repo root for the full list.

| Prefix | Purpose |
|--------|---------|
| `build-*` | CLI, modeld, backend runtime, and VS Code builds |
| `package-*` | relocatable modeld bundles and VS Code VSIX packages |
| `test-*` | Go unit tests, explicit integration suites, backend shim checks, and CLI help checks |
| `dev-*` | local binary and VS Code extension install helpers |
| `deps-*` | direct llama.cpp headers/source, OpenVINO SDK/GenAI deps, and VS Code extension dependencies |
| `clean*` | remove generated binaries, native runtime bundles, and VS Code packaging artifacts |

Version bumps and release notes for maintainers live in `Makefile.version`
(`make -f Makefile.version help`).

### Planning docs

Keep these files in sync when changing public surface area:

- `ROADMAP.md`
- `docs/blueprints/v1-feature-map.md`
- `docs/blueprints/vscode-marketplace-release.md`
- `docs/blueprints/runtime-prune-http-ui-proxy.md`

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

For a CPU llama daemon:

```bash
make build-modeld-llama
make run-modeld-llama
```

For a CUDA llama daemon, install the CUDA toolkit (`nvcc` must be on `PATH`) and
run:

```bash
make build-modeld-llama-gpu
make run-modeld-llama-gpu
```

For an OpenVINO daemon:

```bash
make deps-openvino
make build-modeld-openvino
make run-modeld-openvino
```

Relocatable daemon bundles are built from the same root Makefile:

```bash
make package-modeld-llama
make package-modeld-llama-gpu
make package-modeld-openvino
```

`make package-modeld` builds the combined llama + OpenVINO bundle. `make
package-modeld-gpu` builds the combined bundle with the CUDA llama runtime.

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
npm run package:check -- runtime-$(tr -d '\r\n' < ../../runtime/version/version.txt).vsix
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
make test-llamacpp-direct-cpu
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
