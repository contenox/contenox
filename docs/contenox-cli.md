# Contenox CLI

**Contenox CLI** is the local CLI layer over the Contenox workflow runtime. It runs without Postgres, NATS, or a tokenizer service — just SQLite and an in-memory bus. Point it at local Ollama, Ollama Cloud, OpenAI, vLLM, Gemini, or local GGUF models, then run versioned chains with explicit prompts, tool policy, retries, branches, and review gates.

---

## Quick start

```bash
# From a release binary:
contenox init                          # scaffold .contenox/ with config + default chain
contenox "list files in my home dir"   # one-shot chain run using configured policy

# Or build from source:
git clone https://github.com/contenox/runtime.git
cd runtime
go build -o contenox ./cmd/contenox
contenox init
```

**Requirements (quickest local path):** Ollama running (`ollama serve`) and a model that supports tool calling:

```bash
ollama pull qwen2.5:7b
```

For hosted providers instead, use `contenox backend add ...` with `--api-key-env` as shown below. For Ollama Cloud, set `--url https://ollama.com/api --api-key-env OLLAMA_API_KEY`.

---

## Runtime posture

Contenox is useful as a stable local machine for packaged workflows. Prefer small, reviewable chains for known recurring work over broad promises of fully delegated work. The OSS runtime gives you model/backend switching, local state, OpenAPI/MCP/shell/filesystem tools, chain branching, retries, and HITL gates.

---

## Subcommands

### Bare `contenox …` — stateless run (injected `run`)

When the first argument is **not** a reserved subcommand (`chat`, `init`, `run`, …), the CLI prepends `run`. That is the same as `contenox run …`: **no chat session**; input is passed to the **default run chain** if present.

- Chain file: `<resolved .contenox>/default-run-chain.json`, where `.contenox` is discovered by walking up from the current directory (see `contenox run --help`). Override with `--chain`.
- Global settings and backends still live in `~/.contenox/local.db`; chain JSON files are project-local under `.contenox/`.

```bash
contenox "what is the current directory?"   # → contenox run … when no subcommand
contenox --input "explain this error" < build.log
echo "summarise this" | contenox
```

### `contenox chat` — stateful chain session

```bash
contenox chat "hello"
```

Input comes from positional args, `--input`, or stdin. History is stored in SQLite. Uses the configured default chain (KV `default-chain` or `.contenox/default-chain.json`); override with `--chain`.

---

### `contenox run` — run any chain, any input type

For scripting and pipeline use cases where you want full control. **`--chain` is optional** if `<resolved .contenox>/default-run-chain.json` exists (same discovery as a bare `contenox` invocation).

```bash
# String input (default)
contenox run --chain .contenox/my-chain.json "is this code safe?"

# Wrap as a chat message
cat diff.txt | contenox run --chain .contenox/review.json --input-type chat

# Read input from a file
contenox run --chain .contenox/doc-chain.json --input @main.go

# Structured JSON input
contenox run --chain .contenox/parse.json --input-type json '{"key":"value"}'
```

`--chain` is required. Supported `--input-type` values: `string` (default), `chat`, `json`, `int`, `float`, `bool`.

`contenox run` is **stateless** — no session history is loaded or saved.

---

### `contenox tools` — manage remote tools

Register external HTTP services as callable tools. The runtime fetches the service's `/openapi.json`, discovers every operation, and exposes them as named tools in chains.

**Real example: US National Weather Service** — free, no API key, OpenAPI spec at `https://api.weather.gov/openapi.json`.

```bash
# Register
contenox tools add nws --url https://api.weather.gov --timeout 15000

# Inspect — lists all discovered tools live from the schema
contenox tools show nws
# Name:    nws
# URL:     https://api.weather.gov
# Timeout: 15000ms
# Tools (60):
#   point                    Returns metadata about a given latitude/longitude point
#   alerts_active_area       Returns active alerts for the given area (state or marine area)
#   alerts_active_count      Returns info on the number of active alerts
#   gridpoint_forecast       Returns a textual forecast for a 2.5km grid area
#   ...
```

Run a query using the included example chain:

```bash
contenox run --chain .contenox/chain-nws.json --input-type chat \
  "how many active weather alerts are there right now?"
```

Manage tools:

```bash
contenox tools list                                    # NAME  URL  TIMEOUT
contenox tools update nws --timeout 30000              # update timeout
contenox tools update nws --header "X-App: myapp"      # add a header
contenox tools remove nws                              # remove
```

**Use in any chain** — reference by name in `execute_config.tools`:

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "tools": ["nws"]
}
```

The `tools` array is an **allowlist** with pattern support:

| Value                    | Meaning                                        |
| ------------------------ | ---------------------------------------------- |
| field absent (`null`)    | All registered tools (backward compat default) |
| `[]`                     | No tools exposed to the model                  |
| `["*"]`                  | All registered tools (explicit)                |
| `["nws", "local_shell"]` | Only the named tools                           |
| `["*", "!local_shell"]`  | All except `local_shell`                       |

Unknown names in an exact list are silently ignored (e.g. if `local_shell` is disabled the chain still runs).

Header values are never echoed back (`tools show` prints header keys only). If the service is unreachable at registration time, the tool is still saved and validated at execution time.

> **NWS note:** Forecast lookups require two calls — the model first calls `point` with lat/lon to get the grid reference, then `gridpoint_forecast` with that reference. The included `chain-nws.json` explains this in its system prompt.

---

## Configuration

Contenox stores all configuration in SQLite (`.contenox/local.db`, or `~/.contenox/local.db` globally).
No YAML file — use CLI commands to register backends and set defaults.

### Register a backend

```bash
contenox backend add local   --type ollama
contenox backend add ollama-cloud --type ollama --url https://ollama.com/api --api-key-env OLLAMA_API_KEY
contenox backend add openai    --type openai    --api-key-env OPENAI_API_KEY
contenox backend add anthropic --type anthropic --api-key-env ANTHROPIC_API_KEY
contenox backend add mistral   --type mistral   --api-key-env MISTRAL_API_KEY
contenox backend add gemini    --type gemini    --api-key-env GEMINI_API_KEY
contenox backend add bedrock   --type bedrock   --url https://bedrock-runtime.us-east-1.amazonaws.com
contenox backend add myvllm    --type vllm      --url http://gpu-host:8000

contenox backend list
contenox backend show openai
contenox backend remove myvllm
```

### Set persistent defaults

```bash
contenox model list                         # confirm the runtime can see a model first
contenox config set default-model    qwen2.5:7b
contenox config set default-provider ollama
contenox config set default-think    high
contenox config set default-chain    .contenox/default-chain.json

contenox config list   # review current settings
```

`default-think` controls the requested reasoning level for models whose effective runtime capability has `think` enabled. Valid values are `auto`, `off`, `minimal`, `low`, `medium`, `high`, and `xhigh`. CLI and ACP sessions default to `high` when no config is set; `--think <level>` overrides it for one CLI invocation. If a provider does not advertise thinking support, add a provider/model override with `contenox model capability set ... --think true`.

### Supported backends

| `--type` | Provider | Notes                                                                                                     |
| -------- | -------- | --------------------------------------------------------------------------------------------------------- |
| `local`  | llama.cpp | Embedded inference, no server. `--url` takes a GGUF path or huggingface.co URL. Registered by `init`.    |
| `ollama` | Ollama   | Local: run `ollama serve` first. Hosted: use `--url https://ollama.com/api --api-key-env OLLAMA_API_KEY`. |
| `openai` | OpenAI   | Use `--api-key-env OPENAI_API_KEY`. Base URL inferred.                                                    |
| `anthropic` | Anthropic | Claude (direct API). Use `--api-key-env ANTHROPIC_API_KEY`. Base URL inferred.                         |
| `mistral` | Mistral | La Plateforme. Use `--api-key-env MISTRAL_API_KEY`. Base URL inferred.                                     |
| `gemini` | Gemini   | Use `--api-key-env GEMINI_API_KEY`. Base URL inferred.                                                    |
| `bedrock` | AWS Bedrock | Converse API. `--url` carries the region. Auth: ambient AWS chain, or static-keys JSON via `--api-key-env`. |
| `vllm`   | vLLM     | Self-hosted OpenAI-compatible endpoint, requires `--url`                                                  |
| `vertex-google` | Vertex AI | Gemini on GCP. Requires `--url` with project + region. Auth: service-account JSON via `--api-key-env`, or ADC. |

### Model management

```bash
contenox model list                              # query live backends and effective capabilities

# Store a provider-scoped thinking capability override.
# Use this when the backend does not advertise thinking support, or to suppress it.
contenox model capability set openai gpt-5-mini --think true
contenox model capability set vllm Qwen/Qwen3-32B --think false
contenox model capability show openai gpt-5-mini
contenox model capability unset openai gpt-5-mini

# Store a local context override for a model that already has a local row.
# Accepts a bare integer or a k/m shorthand (case-insensitive):
#   k = ×1 000  →  12k = 12 000
#   m = ×1 000 000  →  1m = 1 000 000
contenox model set-context gpt-5-mini            --context 128k
contenox model set-context gemini-3.1-pro-preview --context 1m
contenox model set-context qwen2.5:7b             --context 32k
```

OSS no longer exposes model CRUD. The runtime discovers models from registered backends; use
`contenox backend add ...`, provider configuration, and `contenox model list` to manage what is available.
`contenox model list` shows effective capabilities, including manual `model capability` overrides.

### Global flags reference

| Flag                       | Purpose                                                                                          |
| -------------------------- | ------------------------------------------------------------------------------------------------ |
| `--chain`                  | Path to chain JSON (overrides `config default-chain`)                                            |
| `--db`                     | SQLite path (default: `.contenox/local.db`)                                                      |
| `--data-dir`               | Override the `.contenox` data directory (skips walk-up search; DB defaults to `<path>/local.db`) |
| `--provider`               | Provider type override                                                                           |
| `--model`                  | Model name override                                                                              |
| `--think <level>`          | Per-invocation reasoning level override: `auto`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh` |
| `--context`                | Context length in tokens — bare int or shorthand (`12k`, `128k`, `1m`)                           |
| `--shell`                  | Enable `local_shell` tool (opt-in; policy is set in the chain, not here)                         |
| `--local-exec-allowed-dir` | Restrict `local_fs` to this directory                                                            |
| `--trace`                  | Emit structured operation telemetry to stderr                                                    |
| `--steps`                  | Print execution steps after result                                                               |
| `--raw`                    | Print full output instead of last assistant message                                              |

---

## The `local_shell` tool

Runs commands on your local machine — real side effects. **Opt-in only.**

Enable with `--shell`. Policy (which commands are allowed or denied) is declared **in the chain**, not as CLI flags:

```bash
contenox chat --shell "run the tests"
contenox run --shell --chain .contenox/my-chain.json "build the project"
```

The default chains (`default-chain.json`, `default-run-chain.json`) ship with a sensible baseline:

- **Allowed:** `ls`, `cat`, `echo`, `git`, `go`, `python3`, `node`, `npm`, `make`, `cargo`, `curl`, `wget`, `jq`, and common read-only tools
- **Denied:** `sudo`, `su`, `dd`, `mkfs`, `fdisk`, `parted`, `shred`

To customise for a chain, add a `tools_policies` block to `execute_config`:

```json
"execute_config": {
  "tools": ["local_shell"],
  "tools_policies": {
    "local_shell": {
      "_allowed_commands": "git,go,make",
      "_denied_commands": "sudo,su,dd"
    }
  }
}
```

`--local-exec-allowed-dir` still restricts `local_fs` to a directory; it does **not** affect `local_shell` command policy.

When `--shell` is not passed, the `local_shell` tool is simply not registered — chains that reference it will run without it.

---

## Output and flags

| Flag              | Effect                                                                                              |
| ----------------- | --------------------------------------------------------------------------------------------------- |
| _(default)_       | Uses `default-think` or hard default `high`; result goes to stdout                                  |
| `--think off`     | Disable provider reasoning controls for this invocation                                             |
| `--think auto`    | Omit explicit provider reasoning controls and use provider defaults                                 |
| `--think <level>` | Set reasoning to `minimal`, `low`, `medium`, `high`, or `xhigh`; reasoning chunks print to stderr   |
| `--trace`         | Structured operation telemetry on stderr (op_id, duration, model selected, etc.)                    |
| `--steps`         | Print task list with handler and duration after the result                                          |
| `--raw`           | Print the full output value (e.g. full chat history JSON)                                           |

---

## ACP slash commands

ACP sessions can run the same local chains from an editor and advertise the same runtime controls through slash commands:

```text
/model [model-name]
/provider [provider-name]
/think [level|off|auto]
/capability show <provider> <model>
/capability set <provider> <model> --think true|false
/capability unset <provider> <model>
```

`/think` changes only the current ACP session's requested reasoning level. `/capability` persists the provider/model capability override in the same KV store used by the CLI, so it affects future backend cycles and sessions.

---

## Chains

Chains are JSON files that package workflow behavior: which model to use, which tools are exposed, how retries work, when to pause, and how to branch based on output. Place them in `.contenox/` and reference by path.

### Macros in chains

Chain text fields and `execute_config.model`, `execute_config.provider`, and `execute_config.think` support macros expanded before execution:

| Macro                          | Expands to                                                                       |
| ------------------------------ | -------------------------------------------------------------------------------- |
| `{{var:model}}`                | Current model name                                                               |
| `{{var:provider}}`             | Current provider name                                                            |
| `{{var:think}}`                | Effective reasoning level                                                        |
| `{{var:chain}}`                | Chain ID                                                                         |
| `{{var:NAME}}`                 | Value from `template_vars_from_env` config (contenox only)                       |
| `{{now}}` / `{{now:layout}}`   | Current time                                                                     |
| `{{chain:id}}`                 | Chain ID (same as `{{var:chain}}`)                                               |
| `{{toolservice:list}}`         | All **allowed** tools + their function names as JSON, filtered by this task's `tools` allowlist |
| `{{toolservice:tools}}`        | Allowed tool group names only                                                          |
| `{{toolservice:tools <name>}}` | Tool names for a specific tool group (empty if not in allowlist)                  |

For `execute_tool_calls` tasks, `execute_config.tools` is also an execution-time restriction when the field is explicitly present. Omit it to preserve legacy chain-wide tool-call resolution; set it to `[]` to execute no registry tools in that phase. `hide_tools` blocks matching namespaced tools such as `local_fs.write_file`.

## Build from source

```bash
git clone https://github.com/contenox/runtime.git
cd runtime
make build-contenox
# binary: ./bin/contenox
contenox init
```

The release version string is **`runtime/version/version.txt`**, embedded at compile time through `version.Get()` and shown in `contenox --help`, `contenox --version`, and the root command `Short` line. Optional link-time override: `-ldflags "-X github.com/contenox/runtime/runtime/contenoxcli.Version=…"`.

### Check that CLI help still works

After changing Cobra commands or flags, run:

```bash
make test-contenox-help
```

This rebuilds the binary and smoke-tests `contenox <command> --help` for each primary subcommand. If you maintain a second copy of this reference elsewhere, keep behavior descriptions aligned when you change defaults or chain resolution.

### Check the local HTTP API

After changing `contenox serve` or any `/api/*` route, run:

```bash
make test-api
```

This builds the local binary, starts `contenox serve` with an isolated temporary
HOME/workspace/DB, runs the recovered Python API smoke tests, and shuts the
server down. Use `PYTEST_ARGS` to narrow the run, for example:

```bash
make test-api PYTEST_ARGS="-k mcp"
```
