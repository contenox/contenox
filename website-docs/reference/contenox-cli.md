# contenox CLI Reference

`contenox` is the local AI agent CLI. It runs the Contenox chain engine entirely on your machine.

## Global Flags

| Flag | Description |
|------|-------------|
| `--trace` | Print verbose chain execution logs |
| `--steps` | Stream intermediate task names and tool executions |
| `--think` | Stream the model's reasoning/thinking trace to stderr before the main output |
| `--model <name>` | Override the default model (set via `contenox config set default-model`) |

## Subcommands

### `contenox chat` (or just `contenox`)

Sends a message to the active chat session and prints the response. History is persisted across invocations.

```bash
contenox "what can you do?"
echo "summarise README.md" | contenox
contenox chat --shell "list files here"
contenox chat --local-exec-allowed-dir . "summarise the README"
```

| Flag | Description |
|---|---|
| `--trim N` | Only send last N messages from session history to the model (0 = all) |
| `--last N` | Print last N user/assistant turns after the reply (0 = only new reply) |
| `--shell` | Enable `local_shell` hook (use only in trusted environments) |
| `--local-exec-allowed-dir <dir>` | Allow `local_fs` tools inside this directory |
| `--local-exec-allowed-commands <cmds>` | Comma-separated allowed shell commands |

### `contenox session`

Manage named chat sessions. Each session maintains its own conversation history.

```bash
contenox session list                    # list all sessions (* = active)
contenox session new [name]             # create a session (becomes active)
contenox session switch <name>          # switch to a different session
contenox session show                   # show active session's history
contenox session show <name>            # show any session by name
contenox session show --tail 10         # show last 10 messages
contenox session show --head 5          # show first 5 messages
contenox session show default --tail 6  # tail a non-active session
contenox session delete <name>          # delete session and all messages
```

### `contenox run`

Executes a specific chain non-interactively. Useful for wiring Contenox into bash scripts or CI pipelines.

```bash
contenox run --chain .contenox/chain-nws.json --input-type chat "how is the weather?"
contenox run --chain .contenox/my-chain.json --shell "refactor main.go"
```

- `--chain <path>`: Required. Path to the chain JSON file.
- `--input-type <type>`: How to parse the positional argument. `chat` treats it as a user message. `string` treats it as raw string input. Defaults to `string`.
- `--shell`: Enable shell execution for this invocation (use only in trusted environments).
- `--think`: Stream the model's reasoning trace to stderr before outputting the final result.

### `contenox plan`

Autonomous multi-step execution using a separate "planner" model that directs an "executor" model.

```bash
contenox plan new "analyze main.go, find the bug, and write a fix to patch.diff"
contenox plan next          # execute next pending step
contenox plan next --shell  # execute next step with shell access enabled
contenox plan next --auto   # run all pending steps automatically
```

- `--shell`: Enable shell execution for the step (only applies to `plan next`, use only in trusted environments).
- `--steps`: Live-stream intermediate tool calls and their outputs during execution.
- `--think`: Stream the model's reasoning/chain-of-thought to stderr before it takes action (for thinking models).
- `--trace`: Verbose debugging output of the step-executor state machine.

### `contenox vibe`

Full terminal UI — chat, plan management, model introspection, and config in a single window with a live plan sidebar.

```bash
contenox vibe
```

The sidebar shows the active plan with real-time step status (`⟳` = executing, `✓` / `✗` = done). `Ctrl+B` cycles sidebar width; `Ctrl+C` quits.

**Chat and shell**
```
hello, what's in this repo?         ← plain text → chat (shares session with contenox chat)
$ git log --oneline -10             ← $ prefix → shell; stdout injected into LLM context
```

**Plan commands**
```
/plan new "add prometheus metrics to the HTTP server"
/plan next                          ← one step, then stop
/plan next --auto                   ← run to completion
/plan step 3                        ← show the full output recorded for step 3
/plan show                          ← all steps and statuses in the log
/plan list                          ← all plans (* = active)
/plan retry 3                       ← reset step 3 to pending
/plan skip 2                        ← mark step 2 skipped
/plan replan                        ← regenerate remaining steps with the LLM
/plan delete <name>                 ← remove a plan by name
/plan clean                         ← remove all completed plans
```

**Models and config — same handlers as the CLI**
```
/model list                         ← live query of all registered backends
/model add gpt-5-mini
/model remove gpt-5-mini
/config set default-model qwen2.5:7b
/config get default-model
```

**Sessions**
```
/session list                       ← all sessions (* = active)
/session new [name]                 ← create a new session
/session switch <name>              ← switch active session
/session delete <name>              ← delete a session and its history
/session show                       ← current session ID + message count
```

**Backends, hooks, MCP**
```
/backend list                       ← list registered backends
/backend show <name>                ← show backend details
/hook list                          ← list remote hooks
/hook show <name>                   ← hook details + live tool list
/mcp list                           ← list MCP servers
/mcp show <name>                    ← MCP server config
/mcp                                ← refresh MCP workers in sidebar
```

**Utility**
```
/clear                              ← wipe the viewport log
/run --chain .contenox/review.json "summarise this"   ← stateless chain run
/help                               ← in-TUI command reference
```

> [!NOTE]
> `/model list`, `/config get`, `/session list`, `/backend list` etc. call the exact same cobra handler code as their CLI counterparts — the TUI captures their output into the viewport with no logic duplication.

### `contenox hook`

Manage remote OpenAPI hooks. See [Remote Hooks](/hooks/remote) and [Hook Allowlist Patterns](/hooks/#how-it-works).

```bash
contenox hook add <name> --url <url>
contenox hook add <name> --url <url> --header "Authorization: Bearer $TOKEN" --inject "tenant_id=acme"
contenox hook list
contenox hook show <name>
contenox hook update <name> --header <...> --inject <...>
contenox hook remove <name>
```

| Flag | Description |
|---|---|
| `--url` | Base URL of the OpenAPI service (required) |
| `--header` | HTTP header to inject on every call, e.g. `"Authorization: Bearer $TOKEN"` (repeatable) |
| `--inject` | Tool call argument to inject and hide from the model, e.g. `"tenant_id=acme"` (repeatable) |
| `--timeout` | Request timeout in milliseconds (default: 10000) |

### `contenox init`

Initializes a new `.contenox/` directory with default chain files.

```bash
$ contenox init
  Created .contenox/default-chain.json
  Created .contenox/default-run-chain.json
Done.
```

After init, register a backend:

```bash
contenox backend add local --type ollama
contenox config set default-model qwen2.5:7b
```

### `contenox backend`

Register and manage LLM backend endpoints.

```bash
contenox backend add local   --type ollama
contenox backend add openai  --type openai  --api-key-env OPENAI_API_KEY
contenox backend add gemini  --type gemini  --api-key-env GEMINI_API_KEY
contenox backend add myvllm --type vllm    --url http://gpu-host:8000

contenox backend list
contenox backend show openai
contenox backend remove myvllm
```

| Flag | Description |
|---|---|
| `--type` | Backend type: `ollama`, `openai`, `gemini`, `vllm` |
| `--url` | Base URL (auto-inferred for openai/gemini) |
| `--api-key-env` | Environment variable holding the API key (preferred) |
| `--api-key` | API key literal (avoid — use `--api-key-env`) |

### `contenox config`

Manage persistent CLI defaults stored in SQLite.

```bash
contenox config set default-model    qwen2.5:7b
contenox config set default-provider ollama
contenox config set default-chain    .contenox/default-chain.json

contenox config get default-model
contenox config list
```

### `contenox mcp`

Register and manage MCP (Model Context Protocol) servers.

```bash
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

contenox mcp list
contenox mcp show myserver
contenox mcp update myserver --inject "tenant_id=newvalue"
contenox mcp remove myserver
```

| Flag | Description |
|---|---|
| `--transport` | Server transport: `stdio`, `sse`, `http` |
| `--command` | Command to execute (stdio only) |
| `--args` | Comma-separated command arguments |
| `--url` | Remote endpoint URL (sse, http) |
| `--auth-type` | Authentication type (e.g. `bearer`) |
| `--auth-env` | Environment variable holding auth token (preferred) |
| `--auth-token` | Auth token literal (avoid — use `--auth-env`) |
| `--header` | Additional HTTP header for SSE/HTTP connections, e.g. `"X-Tenant: acme"` (repeatable) |
| `--inject` | Tool call argument to inject and hide from the model, e.g. `"tenant_id=acme"` (repeatable) |

> [!NOTE]
> `mcp update --header` and `mcp update --inject` each **replace** the entire corresponding map. Pass all required values in a single update call.
