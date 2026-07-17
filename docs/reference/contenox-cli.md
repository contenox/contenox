# contenox CLI Reference

`contenox` is the local AI agent CLI. It runs the Contenox chain engine entirely on your machine.

![A natural-language task in the terminal: contenox reads the repo and answers](/hero.gif)

## Global Flags

Persistent flags on the root command (also shown under **Global Flags** on subcommands). Run `contenox --help` for the full list.

| Flag                             | Description                                                                                                                       |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `--model <name>`                 | Model override for this invocation; persistent default is `contenox config set default-model <name>`                               |
| `--provider <type>`              | Provider override for this invocation. See `contenox backend add --help` for supported backend types. |
| `--db <path>`                    | SQLite DB path (default: `~/.contenox/local.db`). The one global database is shared by every workspace. |
| `--data-dir <path>`              | Override the `.contenox` data directory (skips walk-up search). Used to locate the workspace's `workspace.id` and chain files; does not change the database location. |
| `--timeout`                      | Max execution time per invocation (default `5m`)                                                                                  |
| `--context`                      | Context length hint for the tokenizer                                                                                             |
| `--ollama`                       | Ollama base URL (default `http://127.0.0.1:11434`)                                                                                |
| `--no-delete-models`             | Legacy compatibility flag; a no-op in the OSS runtime (model deletion is disabled). Defaults to **true**.                          |
| `--chain <path>`                 | Chain JSON for injected `run` / chat when applicable                                                                              |
| `--input <value>`                | Input string or `@file` (chat / bare run paths)                                                                                   |
| `--trace`                        | Structured operation telemetry on stderr                                                                                          |
| `--steps`                        | Print execution steps after the result                                                                                            |
| `--think <level>`                | Set reasoning level for supported models: `auto`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`                              |
| `--raw`                          | Print full structured output (e.g. entire chat JSON)                                                                              |
| `--shell`                        | Enable `local_shell` tools (trusted environments only)                                                                             |
| `--local-exec-allowed-dir <dir>` | Restrict local filesystem access and `local_shell` executable/script paths for this invocation                                    |
| `--alt-model <name>`             | Alt model name (for chains referencing `{{var:alt_model}}`). Overrides `default-alt-model` config.                                  |
| `--alt-provider <type>`          | Alt provider type (for chains referencing `{{var:alt_provider}}`). Overrides `default-alt-provider` config.                          |
| `--max-tokens <N>`               | Response token cap (for chains referencing `{{var:max_tokens}}`). Overrides `default-max-tokens` config.                             |
| `-e, --editor`                   | Open `$EDITOR` (or `$VISUAL`, fallback `nano`) to compose the prompt.                                                             |

## Subcommands

### `contenox setup`

Runs an interactive setup wizard to configure your primary provider, model, and API key. This is the recommended first step for all new users. It ensures your global `~/.contenox/` configuration is ready for use.

```bash
contenox setup
```

The wizard will guide you through picking a provider (like a local model, OpenAI, or Gemini), entering an API key if required, and setting your first default model.

### `contenox` (bare — stateful `chat`)

If the first token is **not** a reserved subcommand (`chat`, `init`, `run`, …), the CLI **prepends `chat`**. This starts or continues a stateful, session-backed conversation. It is the default, interactive mode.

The default chat chain is resolved by name: workspace `.contenox/default-chain.json` wins when present, otherwise Contenox falls back to `~/.contenox/default-chain.json`.

```bash
contenox "what can you do?"
echo "summarise README.md" | contenox
contenox --shell "list files here"
contenox --local-exec-allowed-dir . "summarise the README"
```

### `contenox chat`

Sends a message to the **active chat session** and prints the response. History is persisted across invocations in SQLite. This is the explicit version of the bare `contenox` command.

```bash
contenox chat "what can you do?"
echo "summarise README.md" | contenox chat
contenox chat --shell "list files here"
```

| Flag                             | Description                                                            |
| -------------------------------- | ---------------------------------------------------------------------- |
| `--trim N`                       | Only send last N messages from session history to the model (0 = all)  |
| `--last N`                       | Print last N user/assistant turns after the reply (0 = only new reply) |
| `--shell`                        | Enable `local_shell` tools                                              |
| `--local-exec-allowed-dir <dir>` | Restrict local filesystem access and shell executable/script paths       |
| `--auto`                         | Disable HITL prompts for this invocation. HITL is on by default.        |

<h3 id="sessions"><code>contenox session</code></h3>

Manage named chat sessions. Each session maintains its own conversation history. `list` and `show` default to the active scope; the whole database can also be inspected across workspaces and namespaces, and any session opened directly by id — useful for recovering a session an editor lost track of.

```bash
contenox session list                    # list all sessions (* = active)
contenox session new [name]             # create a session (becomes active)
contenox session switch <name>          # switch to a different session
contenox session show                   # show active session's history
contenox session show <name>            # show any session by name
contenox session show <id>              # show any session by id (any workspace)
contenox session show --tail 10         # show last 10 messages
contenox session show --head 5          # show first 5 messages
contenox session show default --tail 6  # tail a non-active session
contenox session fork [name]            # copy the active session to a new one (becomes active)
contenox session fork --summary         # compact older history into a summary, then fork and continue
contenox session delete <name>          # delete session and all messages
```

`session fork` branches the current conversation into a new session so you can explore an alternate direction without losing the original. `--summary` first compacts the older turns into a summary (via `chain-compact.json`) before forking, which trims a long history while preserving context.

Inspect the whole database, not just the active workspace/identity:

```bash
contenox session workspaces              # list workspaces and namespaces (counts)
contenox session list --all              # every session across the whole DB
contenox session list --workspace <id>   # sessions in a workspace
contenox session list --namespace <ns>   # sessions in a namespace (e.g. jetbrainsgoland)
```

A namespace is the session-name prefix before its generated id (e.g. `jetbrainsgoland`, `zed`, `default`). To recover a session an editor abandoned: find it with `session list --namespace <ns>`, then `session show <id>`.

### `contenox run`

Executes a chain non-interactively. Unlike `chat`, `run` does not use session history. It is for stateless, one-shot chain executions.

```bash
contenox run --chain .contenox/chain-nws.json --input-type chat "how is the weather?"
contenox run --chain .contenox/my-chain.json --shell "refactor main.go"
```

- `--chain <path>`: Optional if `<resolved .contenox>/default-run-chain.json` exists; otherwise required.
- `--input-type <type>`: `string` (default), `chat`, `json`, `int` — see `contenox run --help`.
- `--shell`: Enable shell execution for this invocation (use only in trusted environments).
- `--auto`: Disable HITL approval prompts for non-interactive runs. Default is HITL on.
- `--think` / `--trace` / `--steps`: Global flags (see table above).

### `contenox doctor`

Prints local LLM setup readiness: default model, default provider, and backend reachability.

```bash
contenox doctor
contenox doctor --json          # machine-readable output
contenox doctor --skip-cycle    # faster; skips backend sync (status may be stale)
```

| Flag            | Description                                              |
| --------------- | -------------------------------------------------------- |
| `--json`        | Print results as JSON instead of human-readable text     |
| `--skip-cycle`  | Skip syncing backends before the check (faster but may show stale status) |

### `contenox model`

Manage models in the local **Model Registry** — a name-to-URL index of GGUF files that can be downloaded for local inference. See [Local Models (GGUF)](/docs/integrations/providers/local-models/) for a full walkthrough.

#### `contenox model registry-list`

List all curated and user-added registry entries. Does not require a running backend.

```bash
contenox model registry-list
```

#### `contenox model pull`

Download a curated or custom GGUF model to `~/.contenox/models/llama/<name>/model.gguf` (OpenVINO IR models, curated names ending in `-ov`, land under `~/.contenox/models/openvino/<name>/`).

```bash
contenox model pull qwen3-4b                                         # curated model
contenox model pull my-model --url https://huggingface.co/org/repo/resolve/main/model.gguf
```

After downloading, the model is ready for the `llama` backend (or `openvino` for `-ov` models). `contenox init` registers those backends, and the first pulled model becomes `default-model` on a fresh install. Local inference is served by the `modeld` daemon, which must be running in the matching backend mode.

| Flag    | Description                                          |
| ------- | ---------------------------------------------------- |
| `--url` | Direct GGUF download URL (requires a name as arg[0]) |

#### `contenox model add`

Register a custom model entry in the local registry without downloading.

```bash
contenox model add my-model --url https://huggingface.co/org/repo/resolve/main/model.gguf
contenox model add my-model --url https://... --size 4500000000
```

| Flag     | Description                                  |
| -------- | -------------------------------------------- |
| `--url`  | Source URL (required)                        |
| `--size` | File size in bytes (optional, informational) |

#### `contenox model show`

Display registry details for a model.

```bash
contenox model show qwen3-4b
```

#### `contenox model remove`

Remove a user-added registry entry by name. Curated entries cannot be removed.

```bash
contenox model remove my-model
```

#### `contenox model list`

List models currently available from all configured backends (live query, requires at least one backend).

```bash
contenox model list
```

#### `contenox model set-context`

Override the context window size for a specific model name. Useful when a backend reports a different (or no) context size than the model actually supports.

```bash
contenox model set-context qwen2.5:7b           --context 32k
contenox model set-context gpt-5-mini           --context 128k
contenox model set-context gemini-3.1-pro-preview --context 1m
```

| Flag        | Description                                                      |
| ----------- | ---------------------------------------------------------------- |
| `--context` | Context window size: bare integer or shorthand (`12k`, `128k`, `1m`). Required. |

#### `contenox model local`

List installed local model artifacts on disk (under `~/.contenox/models/`), independent of any running backend.

```bash
contenox model local
```

#### `contenox model push`

Push a local model artifact to a `modeld` backend (the local daemon or a remote node), so that node can load and serve it.

```bash
contenox model push qwen3-4b                 # push to the local modeld
contenox model push qwen3-4b --backend <name> # push to a specific modeld node
```

#### `contenox model capability`

Manage manual provider/model capability overrides — currently the reasoning (`think`) capability the runtime assumes for a given provider/model when the catalog doesn't declare it.

```bash
contenox model capability set   <provider> <model> --think   # mark the model as supporting reasoning
contenox model capability show  <provider> <model>           # show the current override
contenox model capability unset <provider> <model>           # remove the override (revert to catalog)
```

#### `contenox model snapshot`

Capture and restore local `modeld` session snapshots — the KV-cache / prefill state of a warmed model — for faster resumption.

```bash
contenox model snapshot save    <model> --out snap.bin   # warm a session, prefill, write the snapshot
contenox model snapshot restore [model] --in  snap.bin   # restore a session from a snapshot file
```

### `contenox modeld`

Manage the local `modeld` inference daemon that serves GGUF (llama) and OpenVINO models.

See the dedicated [modeld guide](/docs/integrations/providers/modeld/) for architecture and concepts.

```bash
contenox modeld install                       # download + verify the prebuilt daemon
contenox modeld install --backend openvino    # require the openvino backend
```

`install` resolves the newest protocol-compatible prebuilt build for this platform, verifies its checksum, installs it under `~/.contenox/modeld/`, and prints the `modeld serve` command to start it. It is non-interactive (the same install runs inside `contenox setup` when a local provider is selected); the installer supports it via `CONTENOX_WITH_MODELD=1`.

### `contenox tools`

Manage remote OpenAPI tools. See [Remote Tools](/docs/integrations/tools/remote) and [Tools Allowlist Patterns](/docs/integrations/tools/#how-it-works).

```bash
contenox tools add <name> --url <url>
contenox tools add <name> --url <url> --header "Authorization: Bearer $TOKEN" --inject "tenant_id=acme"
contenox tools add <name> --url <url> --spec ~/my-spec.yaml   # local file spec
contenox tools list
contenox tools show <name>
contenox tools update <name> --header <...> --inject <...> --spec <url-or-path>
contenox tools remove <name>
```

| Flag        | Description                                                                                |
| ----------- | ------------------------------------------------------------------------------------------ |
| `--url`     | Base URL of the service — where API calls are sent (required)                              |
| `--spec`    | URL or local file path of the OpenAPI v3 spec (`https://...`, `~/path`, `./path`, `/abs/path`). Local paths stored as `file://` URIs; must exist at registration time. Defaults to `<url>/openapi.json`. |
| `--header`  | HTTP header to inject on every call, e.g. `"Authorization: Bearer $TOKEN"` (repeatable)    |
| `--inject`  | Tool call argument to inject and hide from the model, e.g. `"tenant_id=acme"` (repeatable) |
| `--timeout` | Request timeout in milliseconds (default: 10000)                                           |

### `contenox init [provider]`

Initializes a workspace (`.contenox/`) and ensures default runtime presets exist globally (`~/.contenox/`). It's best to run `contenox setup` first for a guided configuration.

`init` creates the `.contenox/workspace.id` marker. Default chains and HITL policies are written under `~/.contenox/` unless they already exist. Workspace-local `.contenox/` files can override these global presets by name.

You can optionally specify a provider to pre-configure defaults.

```bash
contenox init                    # local-first default
contenox init gemini             # pre-configure for Gemini
contenox init openai             # pre-configure for OpenAI
contenox init --force            # overwrite existing files
contenox init --update           # refresh unchanged default files
```

| Flag        | Description                         |
| ----------- | ----------------------------------- |
| `-f, --force` | Overwrite existing preset files |
| `--update`  | Refresh unchanged default files to the latest embedded versions |

### `contenox backend`

Register and manage LLM backend endpoints.

```bash
contenox backend add ollama       --type ollama
contenox backend add ollama-cloud --type ollama --url https://ollama.com/api --api-key-env OLLAMA_API_KEY
contenox backend add llama        --type llama --url ~/.contenox/models/llama/
contenox backend add openai       --type openai  --api-key-env OPENAI_API_KEY
contenox backend add openrouter   --type openrouter --api-key-env OPENROUTER_API_KEY
contenox backend add anthropic    --type anthropic --api-key-env ANTHROPIC_API_KEY
contenox backend add mistral      --type mistral --api-key-env MISTRAL_API_KEY
contenox backend add bedrock      --type bedrock --url https://bedrock-runtime.us-east-1.amazonaws.com
contenox backend add gemini       --type gemini  --api-key-env GEMINI_API_KEY
contenox backend add myvllm       --type vllm    --url http://gpu-host:8000
contenox backend add vertex       --type vertex-google \
  --url "https://us-central1-aiplatform.googleapis.com/v1/projects/YOUR_PROJECT_ID/locations/us-central1"

contenox backend list
contenox backend show openai
contenox backend remove myvllm
```

| Flag            | Description                                                                               |
| --------------- | ----------------------------------------------------------------------------------------- |
| `--type`        | Backend type: `llama`, `openvino`, `modeld`, `ollama`, `openai`, `openrouter`, `anthropic`, `mistral`, `gemini`, `bedrock`, `vllm`, `vertex-google`. (`local` is a legacy alias for `llama`.) |
| `--url`         | Base URL (auto-inferred for openai/openrouter/anthropic/mistral/gemini; required for vllm, bedrock, and vertex-google) |
| `--api-key-env` | Environment variable holding the API key (preferred)                                      |
| `--api-key`     | API key literal (avoid — use `--api-key-env`)                                             |

### `contenox config`

Manage persistent CLI defaults stored in SQLite.

```bash
contenox config set default-provider llama
contenox config set default-model    granite-3.2-2b
contenox config set default-alt-model gemini-2.5-flash
contenox config set default-alt-provider gemini
contenox config set default-autocomplete-model qwen2.5-coder:7b
contenox config set default-autocomplete-provider ollama
contenox config set default-max-tokens 8192
contenox config set default-think high
contenox config set default-chain    .contenox/default-chain.json
contenox config set hitl-policy-name hitl-policy-strict.json

contenox config get default-model
contenox config list
```

Valid global keys: `default-model`, `default-provider`, `default-alt-model`, `default-alt-provider`, `default-autocomplete-model`, `default-autocomplete-provider`, `default-max-tokens`, `default-think`, `telemetry-enabled`, `update-check`.

Valid workspace keys: `default-chain`, `hitl-policy-name`.

### `contenox mcp`

Register and manage MCP (Model Context Protocol) servers.

```bash
# Shorthand: name + URL (transport defaults to http)
contenox mcp add notion https://mcp.notion.com/mcp --auth-type oauth

# Stdio transport (local process)
contenox mcp add myserver --transport stdio --command npx \
  --args "-y,@modelcontextprotocol/server-filesystem,/tmp"

# SSE transport (remote) with bearer auth
contenox mcp add remote --transport sse --url https://mcp.example.com/sse \
  --auth-type bearer --auth-env MCP_TOKEN

# Inject hidden params into every tool call (model never sees them)
contenox mcp add myserver --transport http --url http://localhost:8090 \
  --header "X-Tenant: acme" \
  --inject "tenant_id=acme" --inject "env=production"

# OAuth with pre-issued client credentials (HubSpot, Salesforce, MS Graph,
# any vendor MCP without RFC 7591 dynamic registration)
contenox mcp add hubspot --transport http --url https://mcp.hubspot.com/ \
  --auth-type oauth \
  --oauth-client-id <client_id from vendor UI> \
  --oauth-client-secret-env HUBSPOT_MCP_CLIENT_SECRET

# For OAuth servers, run the authorization flow AFTER adding (opens a browser).
# This is a required, separate step — `mcp add --auth-type oauth` only registers
# the server; it does not authenticate it. Re-run only when the token expires.
contenox mcp auth notion

contenox mcp list
contenox mcp show myserver
contenox mcp update myserver --inject "tenant_id=newvalue"
contenox mcp remove myserver
```

For OAuth servers the full sequence is: `contenox mcp add <name> ... --auth-type oauth`, then `contenox mcp auth <name>` to complete the OAuth 2.1 PKCE flow in the browser. The token is stored locally and reused until it expires.

| Flag           | Description                                                                                |
| -------------- | ------------------------------------------------------------------------------------------ |
| `[url]`        | URL as a second positional arg — sets `--url` and defaults `--transport` to `http`         |
| `--transport`  | Server transport: `stdio`, `sse`, `http`                                                   |
| `--command`    | Command to execute (stdio only)                                                            |
| `--args`       | Comma-separated command arguments                                                          |
| `--url`        | Remote endpoint URL (sse, http)                                                            |
| `--auth-type`                | Authentication type: `bearer` or `oauth`                                                         |
| `--auth-env`                 | Environment variable holding auth token (preferred over `--auth-token`)                          |
| `--auth-token`               | Auth token literal (avoid — use `--auth-env`)                                                    |
| `--oauth-client-id`          | Pre-issued OAuth `client_id` for vendors without RFC 7591 dynamic registration (HubSpot, etc.)   |
| `--oauth-client-secret-env`  | Env var holding the pre-issued OAuth `client_secret` (only the var name is stored locally)       |
| `--header`                   | Additional HTTP header for SSE/HTTP connections, e.g. `"X-Tenant: acme"` (repeatable)            |
| `--inject`                   | Tool call argument to inject and hide from the model, e.g. `"tenant_id=acme"` (repeatable)       |

> [!NOTE]
> `mcp update --header` and `mcp update --inject` each **replace** the entire corresponding map. Pass all required values in a single update call.

### `contenox serve`

Starts the Contenox HTTP server and serves the Beam web UI. Foundation routes live at `/health` and `/version`; the product API is under `/api`; chat (with its HITL approvals and execution-state replay) runs over the `/acp` WebSocket; the Beam UI is served at `/`.

```bash
contenox serve                                  # binds 127.0.0.1:32123 by default
contenox serve ./repo ./another-repo            # extra allowed session workspace roots
```

Binds `127.0.0.1:32123` by default (override with `ADDR`/`PORT`). Set `TOKEN` to require a bearer token on mutating/cross-origin requests (mandatory when `ADDR` is not loopback). A configured model is required — run `contenox setup` first. Terminal routes are on by default under `/api/terminal/sessions` (disable with `TERMINAL_ENABLED=false`).

| Flag / env | Description |
| ---------- | ----------- |
| `--workspace-root <dir>` | Directory a browser client may choose as a session workspace (repeatable). The serve directory is always allowed; also settable via `WORKSPACE_ROOTS` or as positional args. |
| `ADDR` / `PORT` | Override the bind address/port. |
| `TOKEN` | Bearer token required on mutating API requests and cross-origin reads. |
| `BEAM_DEV_PROXY_URL` | Proxy Beam UI requests to a Vite dev server while keeping `/api` on this server. |

### `contenox code [vscode args...]`

Launches VS Code with Contenox's proposed-API extension enabled. Extra arguments are passed through to `code`.

```bash
contenox code .
```

### `contenox state`

Inspects captured execution state from past chain runs — the per-task steps, handlers, transitions, and timings recorded for each request.

```bash
contenox state list             # list request IDs with captured execution state
contenox state show <reqID>     # print the captured steps for a request
contenox state show <reqID> --raw   # print the raw captured state as JSON
```

### `contenox cache clear`

Clears cached backend model lists so the next `chat`/`run` refetches them from the live backends. Use it after adding models to a backend that the runtime hasn't picked up yet.

```bash
contenox cache clear
```

### `contenox update`

Updates `contenox` to the latest release, or just checks for one.

```bash
contenox update             # download and install the latest release
contenox update check       # report whether a newer version exists, without installing
```

### `contenox acp` / `contenox acpx`

Run Contenox as an [ACP](https://agentclientprotocol.com/) agent over stdio, for editor/desktop clients (Zed, JetBrains, AionUi, OpenClaw). `acp` uses the standard editor profile (gated tools route through the client's approval UI); `acpx` uses the hardened headless / untrusted-driver profile.

```bash
contenox acp                 # standard editor profile
contenox acp --auto          # unattended: disable HITL permission prompts
contenox acpx                # headless / untrusted-driver profile
```

The chain each profile loads is overridable via `CONTENOX_ACP_CHAIN_PATH` (acp) and `CONTENOX_ACPX_CHAIN_PATH` (acpx). See the [editor integration guides](/docs/integrations/editors/zed/) for client setup.

### `contenox vscode-agent`

Runs the Contenox VS Code bridge over stdio. This is launched by the VS Code extension, not typically invoked by hand.

```bash
contenox vscode-agent --stdio
```

### `contenox version`

Prints the current binary version and exits.

```bash
contenox version
```

## Environment variables

| Variable | Description |
|---|---|
| `CONTENOX_ACP_CHAIN_PATH` | Override the chain file used by ACP sessions |
| `CONTENOX_ACPX_CHAIN_PATH`| Override the chain file used by headless ACPX sessions |
