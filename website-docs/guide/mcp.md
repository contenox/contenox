# Model Context Protocol (MCP)

Contenox is a native MCP client. Every chat session, plan step, and `contenox run` invocation
can talk to any MCP-compatible server — local processes spawned as child processes, remote
services over SSE, or HTTP-streaming endpoints.

## What is MCP?

The [Model Context Protocol](https://modelcontextprotocol.io/) is an open standard for
connecting AI agents to real-world tools, memory, and data sources. Originally created by
Anthropic and donated to the [AI Agentic Foundation](https://aaif.ai/) at the Linux Foundation,
MCP defines a universal wire format that any agent and any tool can speak.

Think of it as USB-C for AI: one standard plug, infinite devices.

## What makes Contenox MCP different

Most clients treat MCP as a glorified one-shot API call. Contenox does something richer:
it maintains **persistent, session-scoped connections** to every registered MCP server.

- Each chat session or plan gets its own connection to every server
- State is preserved across all tool calls inside that session
- Sessions survive node restarts and can be resumed

This means your agent doesn't just *call* a tool — it builds a persistent relationship with it.

## Register an MCP server

```bash
# Local process spawned via stdio:
contenox mcp add myfiles \
  --transport stdio \
  --command npx \
  --args "-y,@modelcontextprotocol/server-filesystem,/home/user/projects"

# Remote SSE endpoint:
contenox mcp add memory \
  --transport sse \
  --url https://mcp.example.com/sse \
  --auth-type bearer \
  --auth-env MCP_TOKEN

# Remote HTTP streaming endpoint:
contenox mcp add internal \
  --transport http \
  --url http://internal-host:8090
```

All registered servers are automatically available in every chain that lists them in
`execute_config.mcp_servers`. Manage them like backends:

```bash
contenox mcp list
contenox mcp show myfiles
contenox mcp remove myfiles
```

## Try the built-in test server

Contenox ships an MCP test server you can run locally to explore the protocol:

```bash
go run ./cmd/mcp-testserver
# listening on http://localhost:8090

contenox mcp add test --transport http --url http://localhost:8090
```

The test server returns a `session_token` in every response so you can watch the session
staying alive across tool calls.

## Use MCP servers in a chain

Reference servers by name in `execute_config`:

```json
{
  "id": "ask_with_memory",
  "handler": "chat_completion",
  "execute_config": {
    "model": "qwen2.5:7b",
    "provider": "ollama",
    "mcp_servers": ["myfiles", "memory"]
  }
}
```

The chain engine connects to each server at the start of the task, makes all tools available
to the model, and keeps the connection open for the duration of the session.

## Supported transports

| `--transport` | How it works | When to use |
|---|---|---|
| `stdio` | Spawns a child process, talks via stdin/stdout | Local MCP servers (filesystem, databases) |
| `sse` | Connects to a remote Server-Sent Events endpoint | Cloud or remote team servers |
| `http` | Connects via HTTP streaming | Production deployments, internal services |

## Combine with `contenox plan`

MCP servers shine brightest with autonomous plans. An agent running `contenox plan next --shell`
can use an MCP filesystem server to read files, an MCP memory server to store intermediate
results, and a remote API server — all within the same plan, all with full session continuity:

```bash
contenox plan new "explore the codebase, identify the top 3 bugs, write a report"
contenox plan next --auto
```

Each step picks up exactly where the last one left off. No context lost. No state reset.

## Security notes

- Use `--auth-env` rather than `--auth-token` for API keys — keeps secrets out of shell history
- `stdio` servers run as child processes of the CLI — scope their filesystem access carefully
- Session tokens returned by MCP servers are specific to the CLI session and not stored in plain text

## Further reading

- [Official MCP specification](https://modelcontextprotocol.io/)
- [MCP server registry](https://github.com/modelcontextprotocol/servers)
- [CLI reference: `contenox mcp`](/reference/contenox-cli#contenox-mcp)
