# Contenox CLI

**Contenox CLI** is the local CLI layer over the Contenox task engine. It runs without Postgres, NATS, or a tokenizer service — just SQLite and an in-memory bus. Point it at local Ollama, Ollama Cloud, OpenAI, vLLM, or Gemini, and run AI workflows from the terminal: interactive chat or arbitrary chain pipelines.

---

## Quick start

```bash
# From a release binary:
contenox init                          # scaffold .contenox/ with config + default chain
contenox "list files in my home dir"   # natural language → shell → response

# Or build from source:
git clone https://github.com/contenox/agent.git
cd contenox
go build -o contenox ./cmd/contenox
contenox init
```

**Requirements (quickest local path):** Ollama running (`ollama serve`) and a model that supports tool calling:

```bash
ollama pull qwen2.5:7b
```

For hosted providers instead, use `contenox backend add ...` with `--api-key-env` as shown below. For Ollama Cloud, set `--url https://ollama.com/api --api-key-env OLLAMA_API_KEY`.

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

### `contenox chat` — interactive chat (session history)

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
contenox backend add openai  --type openai  --api-key-env OPENAI_API_KEY
contenox backend add gemini  --type gemini  --api-key-env GEMINI_API_KEY
contenox backend add myvllm --type vllm    --url http://gpu-host:8000

contenox backend list
contenox backend show openai
contenox backend remove myvllm
```

### Set persistent defaults

```bash
contenox model list                         # confirm the runtime can see a model first
contenox config set default-model    qwen2.5:7b
contenox config set default-provider ollama
contenox config set default-chain    .contenox/default-chain.json

contenox config list   # review current settings
```

### Supported backends

| `--type` | Provider | Notes                                                                                                     |
| -------- | -------- | --------------------------------------------------------------------------------------------------------- |
| `ollama` | Ollama   | Local: run `ollama serve` first. Hosted: use `--url https://ollama.com/api --api-key-env OLLAMA_API_KEY`. |
| `openai` | OpenAI   | Use `--api-key-env OPENAI_API_KEY`                                                                        |
| `vllm`   | vLLM     | Self-hosted OpenAI-compatible endpoint, requires `--url`                                                  |
| `gemini` | Gemini   | Use `--api-key-env GEMINI_API_KEY`                                                                        |

### Model management

```bash
contenox model list                              # query live backends (runtime-observed inventory)

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

### Global flags reference

| Flag                       | Purpose                                                                                          |
| -------------------------- | ------------------------------------------------------------------------------------------------ |
| `--chain`                  | Path to chain JSON (overrides `config default-chain`)                                            |
| `--db`                     | SQLite path (default: `.contenox/local.db`)                                                      |
| `--data-dir`               | Override the `.contenox` data directory (skips walk-up search; DB defaults to `<path>/local.db`) |
| `--provider`               | Provider type override                                                                           |
| `--model`                  | Model name override                                                                              |
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

| Flag        | Effect                                                                           |
| ----------- | -------------------------------------------------------------------------------- |
| _(default)_ | Quiet: "Thinking…" on stderr while running, result on stdout                     |
| `--trace`   | Structured operation telemetry on stderr (op_id, duration, model selected, etc.) |
| `--steps`   | Print task list with handler and duration after the result                       |
| `--raw`     | Print the full output value (e.g. full chat history JSON)                        |

---

## Chains

Chains are JSON files that define the LLM workflow: which model, which tools, how to branch based on output. Place them in `.contenox/` and reference by path.

### Macros in chains

Chain fields like `system_instruction` and `prompt_template` support macros expanded before execution:

| Macro                          | Expands to                                                                       |
| ------------------------------ | -------------------------------------------------------------------------------- |
| `{{var:model}}`                | Current model name                                                               |
| `{{var:provider}}`             | Current provider name                                                            |
| `{{var:chain}}`                | Chain ID                                                                         |
| `{{var:NAME}}`                 | Value from `template_vars_from_env` config (contenox only)                       |
| `{{now}}` / `{{now:layout}}`   | Current time                                                                     |
| `{{chain:id}}`                 | Chain ID (same as `{{var:chain}}`)                                               |
| `{{toolservice:list}}`         | All **allowed** tools + their function names as JSON, filtered by this task's `tools` allowlist |
| `{{toolservice:tools}}`        | Allowed tool group names only                                                          |
| `{{toolservice:tools <name>}}` | Tool names for a specific tool group (empty if not in allowlist)                  |

## Build from source

```bash
git clone https://github.com/contenox/agent.git
cd contenox
make build-contenox
# binary: ./bin/contenox
contenox init
```

The release version string is **`runtime/version/version.txt`**, embedded at compile time through `version.Get()` and shown in `contenox --help`, `contenox --version`, and the root command `Short` line. Optional link-time override: `-ldflags "-X github.com/contenox/agent/contenoxcli.Version=…"`.

### Check that CLI help still works

After changing Cobra commands or flags, run:

```bash
make test-contenox-help
```

This rebuilds the binary and smoke-tests `contenox <command> --help` for each primary subcommand. If you maintain a second copy of this reference elsewhere, keep behavior descriptions aligned when you change defaults or chain resolution.
