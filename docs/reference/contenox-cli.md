# Contenox CLI Reference

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
contenox model pull gemma4-e4b                                       # curated vision model (fetches mmproj.gguf too)
contenox model pull my-model --url https://huggingface.co/org/repo/resolve/main/model.gguf
```

After downloading, the model is ready for the `llama` backend (or `openvino` for `-ov` models). `contenox init` registers those backends, and the first pulled model becomes `default-model` on a fresh install. Local inference is served by the `modeld` daemon, which must be running in the matching backend mode.

Curated vision models (shown as `chat+vision` in `model registry-list`) install every artifact image input needs in one pull: llama entries fetch the multimodal projector beside the model as `mmproj.gguf` (a failed projector download is a hard error, and re-running the pull adds a missing projector to an already-installed model); OpenVINO vision snapshots already include their vision encoder.

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

Manage manual provider/model capability overrides — the reasoning (`think`) and image-input (`vision`) capabilities the runtime assumes for a given provider/model when the catalog doesn't declare them.

```bash
contenox model capability set   <provider> <model> --think true   # mark the model as supporting reasoning
contenox model capability set   <provider> <model> --vision true  # mark the model as accepting image input
contenox model capability show  <provider> <model>                # show the current override
contenox model capability unset <provider> <model>                # remove the override (revert to catalog)
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

### `contenox agent`

Register and manage the external ACP agents the runtime can spawn and drive. An agent is an external program that speaks the [Agent Client Protocol (ACP)](https://agentclientprotocol.com/); contenox never installs one — it invokes a binary you already have, or lets a runtime fetcher (`npx`/`uvx`) pull it down. Registered agents appear in [Beam](/docs/guide/beam/)'s agent picker and over the read-only `GET /api/agents` endpoint. For the full walkthrough see [Host external ACP agents](/docs/integrations/agents/external-acp-agents/).

There are exactly two ways to register one — seed from the ACP registry catalog, or give a bare command:

```bash
# Registry: browse the catalog, then resolve + register an entry
contenox agent search                     # list the whole catalog
contenox agent search claude              # filter by id / name / description
contenox agent add claude-acp             # register a catalog entry (source: registry)
contenox agent add goose --name my-goose  # …under an alias

# Manual: everything after '--' is the argv contenox spawns
contenox agent add local-bot -- /usr/local/bin/my-acp-agent --stdio

contenox agent list                       # id, name, source, kind, enabled
contenox agent show my-goose              # provenance + run command + config_json
contenox agent check my-goose             # drive one live turn to verify it (below)
contenox agent edit my-goose              # open $EDITOR on the run config
contenox agent enable my-goose            # (and: disable)
contenox agent remove my-goose            # (alias: rm)
```

`search` caches the catalog locally (`agent-registry.json`, next to the database) and falls back to that cache when the network is unavailable; pass `--refresh` to force a re-fetch. `add`'s registry form is named after the registry id unless `--name` gives an alias; its manual form takes the name *before* `--` and the argv *after* it — there are deliberately no `--transport`/`--env`/`--args` flags, so any further customization is done by editing the config (below). `remove` deletes only the local registration; it never touches the binary or package the agent would spawn.

| Flag | Description |
| ---- | ----------- |
| `--name <alias>` | Alias for a registry agent (registry form of `add` only; defaults to the registry id). |
| `--refresh` | Force a re-fetch of the ACP registry catalog instead of using the local cache (`search`, `add`). |
| `--config-file <path>` | Replace the config from a file (or `-` for stdin) instead of opening `$EDITOR` (`edit`). |
| `--timeout <dur>` | How long the `check` turn may take before it is cancelled (default `2m`; see below). |

#### The agent run config (`config_json`)

`contenox agent edit <name>` opens the agent's `config_json` — the run spec — in `$EDITOR` (`$VISUAL`, then `nano` as fallbacks), validates it on save, and persists it. Provenance (source, registry id/version) is system-managed and never part of this JSON. For the `external_acp` kind the shape is:

| Field | Description |
| ----- | ----------- |
| `transport` | `stdio` (spawn `command`). `endpoint` (dial `url`) is a reserved value: it validates but is refused at connect time. Required. |
| `command` | Executable to spawn. Required for `stdio`. |
| `args` | Arguments passed to `command`. |
| `env` | Extra environment variables for the spawned process. |
| `cwd` | Working directory for the spawned process. |
| `url` | Endpoint URL (`endpoint` transport). |
| `mcp_servers` | Explicit allowlist of registered MCP server names (`contenox mcp list`) forwarded to this agent in ACP `session/new`. |

The `mcp_servers` allowlist is per-agent consent, named server by named server: forwarding a server hands the agent everything it needs to reach it — argv for stdio servers, URL and configured headers for http/sse — so there is deliberately no "all servers" wildcard, and contenox-side auth synthesis (`authToken`/`authEnvKey`/OAuth/injected params) is never forwarded into the payload. Empty means forward nothing. The [external ACP agents guide](/docs/integrations/agents/external-acp-agents/#forwarding-mcp-servers) covers the consent boundary in full.

```json
{
  "transport": "stdio",
  "command": "claude-code-acp",
  "mcp_servers": ["filesystem"]
}
```

#### `contenox agent check <name> [prompt...]`

Verify a registered agent by driving one live turn through it. `check` spawns the agent as an ACP subprocess and runs a full `initialize → session/new → session/prompt` turn against it — the same client-host path the runtime itself uses (`runtime/agenthost`), not a lighter fake — streaming the agent's reply to stdout as it arrives. It is how you confirm an agent actually works right after `contenox agent add`.

```bash
contenox agent check my-goose             # a plain connection check
contenox agent check claude Say hello     # everything after the name is the prompt
contenox agent check local-bot --timeout 30s
```

The turn is rooted in the current working directory and drives one plain-text prompt; with no prompt the agent is asked to confirm the connection. Agent-initiated file system and terminal callbacks are unavailable, and every permission ask is answered with a clean **denial** (a check never grants) rather than a protocol error; each denial is reported (`Denied N permission ask(s) during the turn …`). Answering a simple prompt should not need any; an agent that insists on gated work may end its turn right after the denial.

![contenox agent check driving a live turn: the forwarded MCP servers line, the streamed reply, the stop reason, and the advertised commands](/agent-check.gif)

If the agent's config declares an `mcp_servers` allowlist, `check` forwards those servers in `session/new` exactly as a real session would, printing a `Forwarding MCP servers:` line; entries the agent's advertised capabilities cannot consume are reported (`Note: MCP servers NOT forwarded …`), not silently dropped. After the reply it prints the turn's stop reason (`Turn completed (agent … stopReason=end_turn)`) and any slash commands the agent advertised (`Agent advertises N command(s): …`). A normal turn that produces no displayable output fails the check rather than reporting success on a silent agent.

| Flag | Description |
| ---- | ----------- |
| `--timeout <dur>` | How long the whole check turn may take before it is cancelled (default `2m`). Separate from the root `--timeout`. |

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

Valid global keys: `default-model`, `default-provider`, `default-alt-model`, `default-alt-provider`, `default-autocomplete-model`, `default-autocomplete-provider`, `default-max-tokens`, `default-think`, `telemetry-enabled`, `update-check`, `default-mission-agent`, `default-mission-policy`.

Valid workspace keys: `default-chain`, `hitl-policy-name`.

`default-mission-agent` and `default-mission-policy` are the fallbacks mission mode fires with when none is named — see [`contenox mission`](#contenox-mission) and the `/mission` slash command.

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
| `--workspace-root <dir>` | Directory a browser client may choose as a session workspace (repeatable). The serve directory is always allowed; also settable via `WORKSPACE_ROOTS` or as positional args. These are the launch-time roots; grant more at runtime — without a restart — via [`contenox workspace add`](#contenox-workspace) or `POST /workspace/roots`. |
| `ADDR` / `PORT` | Override the bind address/port. |
| `TOKEN` | Bearer token required on mutating API requests and cross-origin reads. |
| `BEAM_DEV_PROXY_URL` | Proxy Beam UI requests to a Vite dev server while keeping `/api` on this server. |
| `TERMINAL_ENABLED` | Terminal routes under `/api/terminal/sessions`, on by default (`false` disables). |
| `TERMINAL_ALLOWED_ROOT` | Directory terminal sessions are confined to (default: the workspace root). |
| `TERMINAL_MAX_SESSIONS` | Concurrent terminal session cap (default 8; 0 = unlimited). |
| `TERMINAL_SHELL` | Shell binary for terminal sessions (default: `$SHELL`). |
| `TERMINAL_IDLE_TIMEOUT` | Idle duration after which a terminal session is reaped. |
| `HITL_APPROVAL_TIMEOUT` | Ceiling for pending HITL approvals, a Go duration (e.g. `1h`); expired asks are auto-resolved. |
| `ALLOWED_API_ORIGINS` / `PROXY_ORIGIN` | CORS: extra allowed API origins / the trusted reverse-proxy origin. |

### `contenox fleet`

Operate the fleet — the supervised agent **units** a running `contenox serve` is hosting — from the shell. These verbs act on units (the running processes); the **work** a unit was sent to do lives under [`contenox mission`](#contenox-mission).

The fleet is not in the database — it is a set of live subprocesses owned by the serve process's in-memory manager — so, unlike `contenox state`/`session`, these verbs reach serve over its REST API. By default that is `http://127.0.0.1:32123`; override with `--server`/`--token` or `CONTENOX_SERVER_URL`/`CONTENOX_SERVER_TOKEN` (the same client `contenox approvals` uses).

```bash
contenox fleet list                       # the board: every declared agent + its live units + intent
contenox fleet list --json                # the raw /fleet records (declared agents + instances)
contenox fleet show <instance-id>         # one unit's status (state, sessions, viewers, session ids)
contenox fleet stop <instance-id>         # tear a unit down (idempotent)
contenox fleet cancel <instance-id>       # cancel every in-flight turn on the unit
contenox fleet cancel <instance-id> --session <session-id>   # cancel just that session
```

`list` renders the config+runtime join: every declared agent (idle ones included), each live instance's kind/state/session/viewer counts, and the mission `INTENT` the unit was fired with (joined from the bound mission; `-` when the unit has no mission behind it). `stop` is idempotent by the kernel contract — stopping an unknown or already-stopped instance succeeds — so a script may call it without a preceding existence check. To FIRE a new unit, use `contenox mission fire`; there is deliberately no `fleet dispatch`.

| Flag | Description |
| ---- | ----------- |
| `--server <url>` | Base URL of a running `contenox serve` (default `http://127.0.0.1:32123`; also `CONTENOX_SERVER_URL`). |
| `--token <token>` | Bearer token, when serve was started with one (also `CONTENOX_SERVER_TOKEN`). |
| `--json` | Emit the raw route response instead of a table (`list`, `show`). |
| `--session <id>` | `cancel` only this session id, instead of every session on the unit. |

### `contenox mission`

Work with an agent in **mission mode** — the dual of chat mode. In chat you prompt turn by turn and approve each gated action yourself. In mission mode you fire a one-line intent at a declared agent under an **envelope** (a HITL policy that bounds what it may do unattended) and detach; the unit acts inside the envelope and only crossing it costs your attention, in the [approvals inbox](#contenox-approvals). A mission is a subagent of whatever process fires it — the [`/mission` slash command](#the-mission-slash-command) runs one in-process inside your editor session; **these CLI verbs** are the operator's remote lever, firing onto (and reading from) a running `contenox serve` over the same REST API as `contenox fleet`.

```bash
contenox mission fire --agent reviewer --intent "triage the failing CI run" --policy hitl-policy-strict.json
contenox mission fire --intent "summarise today's commits"    # --agent/--policy from config defaults
contenox mission list                     # what is running, for whom, under what envelope, and why
contenox mission show <mission-id>        # the mission plus its reports, newest first
```

`fire` brings up a unit, hands it the intent as its first turn, and returns as soon as the session is open (the turn runs detached); the printed mission/instance/session ids let you follow it with `mission show` and `fleet show`. `--intent` is always required; `--agent` and `--policy` fall back to the config keys `default-mission-agent` / `default-mission-policy`, so a configured operator can fire with intent alone. A mission with no agent or no envelope is refused. `--cwd` roots the unit's session in an absolute directory serve allows.

`show` prints the mission record — intent, agent, envelope, status, the session/instance it spawned, the parent session that supervises it (set only when a mission is fired from a chat by `/mission`, not by an operator directly), and liveness — followed by the unit's reports, newest first.

| Flag | Description |
| ---- | ----------- |
| `--server <url>` / `--token <token>` | Reach a running `contenox serve` (as `contenox fleet`). |
| `--agent <name>` | Declared agent to fire (`fire`; default: config `default-mission-agent`). |
| `--intent <line>` | One-line mission intent — required for `fire`. |
| `--policy <name>` | Envelope: the HITL policy bounding the unit (`fire`; default: config `default-mission-policy`). |
| `--cwd <dir>` | Absolute working directory for the unit's session (`fire`; default: serve's project root). |
| `--limit <n>` | Cap the mission list (`list`) or the reports shown (`show`). |
| `--json` | Emit raw records: the dispatch ids (`fire`), the mission list (`list`), or `{mission, reports}` (`show`). |

#### The `/mission` slash command

From inside a chat (`contenox acp`, or the Beam chat) you can fire a mission without leaving the conversation:

- `/mission <intent>` — fires the configured `default-mission-agent` under the `default-mission-policy` envelope.
- `/mission <agent-name> <intent>` — fires the named agent instead.

The two forms are the same shape, so contenox resolves the first token against the declared-agent registry: a hit is the named form, a miss means the whole line is the intent for the default agent. The confirmation always states which agent was chosen and echoes the intent, so a misread is visible immediately. A mission fired this way is supervised by the calling session — its reports return there rather than to the operator inbox.

In a standalone `contenox acp` editor session the dispatch runs **in-process** by default: the fired unit is a child subprocess of the editor process itself, no running serve is needed, and the unit's reports stream live back into the firing session as they land (the operator inbox only catches a report whose firing session has already ended). Setting `CONTENOX_SERVER_URL` opts into **forwarding** the dispatch to that serve instead — for firing onto a bigger box — in which case reports land in that serve's operator inbox (`contenox approvals`), since a remote kernel cannot deliver into an editor session it does not own. The hardened `acpx` profile never offers `/mission`.

### `contenox approvals`

List and answer the pending human-in-the-loop approvals a running `contenox serve` is holding — the inbox for asks raised by an agent working with no attached session (dispatched fleet work, a headless API caller). A permission request that would otherwise hang until its policy-rule timeout, or the serve-level ceiling, is answerable here as soon as it lands.

A pending approval is a goroutine parked inside the running serve process — answering it has to reach that process, not just its database — so, unlike `contenox state`/`session`, this command talks to serve's REST API (default `http://127.0.0.1:32123`; override with `--server`/`--token` or `CONTENOX_SERVER_URL`/`CONTENOX_SERVER_TOKEN`).

```bash
contenox approvals list                     # pending asks, newest first
contenox approvals list --json              # raw records, including full diff content
contenox approvals answer <id> --approve
contenox approvals answer <id> --deny
```

`list` prints each ask's id, tool, args summary, policy and matched rule, diff presence, the agent/mission/instance/session attribution (with several units running, the row has to say **whose** action is gated; `-` for an ask raised outside the fleet), and the created/expires timestamps. `answer` requires exactly one of `--approve`/`--deny`; an id that is unknown, already answered, or expired (auto-resolved by serve's sweeper) fails with a non-zero exit status saying which — answering twice is never silently a no-op.

| Flag | Description |
| ---- | ----------- |
| `--server <url>` / `--token <token>` | Reach a running `contenox serve` (as `contenox fleet`). |
| `--limit <n>` | Cap the pending list (`list`; 0 = server default cap). |
| `--json` | Emit raw records (`list`) or the answer result (`answer`). |

### `contenox workspace`

Grant or revoke the **workspace roots** a session may run in — the directories a chat, a dispatched mission unit, or a Beam file browse may choose as its working directory. Granting a root grants everything **under** it; a directory outside every granted root (a sibling, a prefix-trick neighbour like `/home/meX` against `/home/me`, or a symlink whose real target escapes) is refused.

```bash
contenox workspace add /home/me/src        # grant a root (and everything under it)
contenox workspace add /home/me/scratch
contenox workspace list                     # the roots you have granted
contenox workspace remove /home/me/scratch  # revoke a grant
```

Unlike `fleet` / `mission`, these do **not** reach serve over REST. A grant is durable config in the shared database (`~/.contenox/local.db`), so `add`/`remove` write it directly and then ring a reload doorbell on the shared bus. A running `contenox serve` picks up the signal and swaps its live workspace-root set **without a restart**; a serve started later reads the same durable config at boot. So these verbs work whether or not serve is up, and a running serve applies them live.

`add` requires the path to be an existing directory (a workspace root must be a real directory); `remove` does not, so a grant to a since-deleted directory can be cleaned up. Both are idempotent. `list` prints the durable grants these verbs manage — serve additionally always allows its own launched default root (its served directory, or home for a bare `contenox serve`), which is not a grant and appears in the API and Beam folder picker (`GET /workspace/roots`) rather than here.

A LAN operator working only through the browser has the same two verbs over the authenticated REST surface: `POST /workspace/roots {"path": "<dir>"}` grants and `DELETE /workspace/roots?path=<dir>` revokes, each token-authed and returning the new root list — the same validation, durable write, and reload doorbell as the CLI.

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
| `CONTENOX_SERVER_URL` | Base URL of a running `contenox serve` for `fleet`/`mission`/`approvals` (instead of `--server`). In a `contenox acp` editor session, setting it is also the explicit opt-in that **forwards** `/mission` dispatches to that serve instead of running them in-process. |
| `CONTENOX_SERVER_TOKEN` | Bearer token for the serve API (instead of `--token`). |
| `CONTENOX_DEFAULT_MODEL` / `CONTENOX_DEFAULT_PROVIDER` | Process-level override of the configured default model/provider (nothing is persisted). Also the ACP `env_var` auth-method contract for non-interactive setup. |
| `CONTENOX_DEFAULT_ALT_MODEL` / `CONTENOX_DEFAULT_ALT_PROVIDER` | Same, for the alt model pair. |
| `CONTENOX_DEFAULT_MAX_TOKENS` / `CONTENOX_DEFAULT_THINK` | Same, for the response token cap and reasoning level. |
| `CONTENOX_BASE_URL` | Endpoint URL for account-specific providers whose URL cannot be defaulted (e.g. Vertex: project + region). |
