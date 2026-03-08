# Stateful Agents with MCP

> **Prerequisites:** `contenox init`, a backend registered, and at least one MCP server configured.

Most AI agents forget the moment a tool call ends. This recipe shows you how to wire up an
MCP server so your agent carries full context across every single step of a long-running plan —
reads, writes, decisions and all.

## What you'll accomplish

By the end of this recipe your agent will be able to:
- Read and write real files with memory that persists across tool calls
- Maintain private session state for the duration of a plan or conversation
- Pick up exactly where it left off, even after a restart

## Step 1 — Start the test server (optional but great for learning)

Contenox ships an MCP test server you can run locally:

```bash
go run ./cmd/mcp-testserver
# Server listening on http://localhost:8090
```

Every response includes a `session_token` so you can literally watch the agent staying
connected to its own session.

## Step 2 — Register an MCP server

```bash
# Local filesystem server via stdio (most common):
contenox mcp add myfiles \
  --transport stdio \
  --command npx \
  --args "-y,@modelcontextprotocol/server-filesystem,$HOME/projects"

# Or the test server via HTTP:
contenox mcp add test --transport http --url http://localhost:8090
```

Verify it's registered:

```bash
contenox mcp list
contenox mcp show myfiles
```

## Step 3 — Reference it in a chain

In any chain JSON file, list the server by name in `execute_config`:

```json
{
  "id": "explore",
  "handler": "chat_completion",
  "execute_config": {
    "model": "qwen2.5:7b",
    "provider": "ollama",
    "mcp_servers": ["myfiles"]
  }
}
```

The agent now has access to every tool that server exposes — for the entire session.

## Step 4 — Run a stateful plan

```bash
contenox plan new "explore the codebase, find all TODO comments, write a report to TODOS.md"
contenox plan next --auto
```

Each step connects to `myfiles`, performs its work, and the next step inherits the full
session context. The agent remembers what it found in step 1 when it writes in step 5.

## How session continuity works

Contenox maintains one MCP connection per registered server, per session. The session is
tied to the chat session ID (for `contenox chat`) or the plan ID (for `contenox plan`).

- Tool call responses are kept in session memory
- Session state survives individual tool calls within a plan
- On restart, sessions are resumed from the SQLite-persisted plan state

## Real-world ideas

| Goal | MCP server to use |
|---|---|
| Edit files across a multi-step plan | `@modelcontextprotocol/server-filesystem` |
| Remember decisions across days | A custom key-value MCP server |
| Query a company database | An internal MCP HTTP server |
| Manage GitHub issues | `@modelcontextprotocol/server-github` |

## Full CLI reference

```bash
# Add
contenox mcp add <name> --transport stdio --command <cmd> --args <arg1,arg2>
contenox mcp add <name> --transport sse   --url <url> --auth-type bearer --auth-env TOKEN
contenox mcp add <name> --transport http  --url <url>

# Manage
contenox mcp list
contenox mcp show <name>
contenox mcp remove <name>
```

## Further reading

- [MCP docs page](/guide/mcp) — full protocol overview and transport reference
- [Official MCP server registry](https://github.com/modelcontextprotocol/servers)
- [`contenox plan` reference](/reference/contenox-cli#contenox-plan)
