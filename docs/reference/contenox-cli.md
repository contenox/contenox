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
| `--no-delete-models`             | Do not delete undeclared Ollama models (default **true** for CLI)                                                                 |
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
contenox session delete <name>          # delete session and all messages
```

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
- `--input-type <type>`: `string` (default), `chat`, `json`, `int`, `float`, `bool` — see `contenox run --help`.
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

Download a curated or custom GGUF model to `~/.contenox/models/<name>/model.gguf`.

```bash
contenox model pull qwen3-4b                                         # curated model
contenox model pull my-model --url https://huggingface.co/org/repo/resolve/main/model.gguf
```

After downloading, the model is ready for the built-in `local` backend. `contenox init` creates that backend, and the first pulled model becomes `default-model` on a fresh install.

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
contenox backend add embedded     --type local --url ~/.contenox/models/
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
| `--type`        | Backend type: `ollama`, `openai`, `openrouter`, `anthropic`, `mistral`, `gemini`, `bedrock`, `vllm`, `local`, `vertex-google` |
| `--url`         | Base URL (auto-inferred for openai/openrouter/anthropic/mistral/gemini; required for vllm, bedrock, and vertex-google) |
| `--api-key-env` | Environment variable holding the API key (preferred)                                      |
| `--api-key`     | API key literal (avoid — use `--api-key-env`)                                             |

### `contenox config`

Manage persistent CLI defaults stored in SQLite.

```bash
contenox config set default-provider local
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

contenox mcp list
contenox mcp show myserver
contenox mcp update myserver --inject "tenant_id=newvalue"
contenox mcp remove myserver
```

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
