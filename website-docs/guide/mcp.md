# Model Context Protocol (MCP)

Contenox is a full native MCP client. Every chat session, plan step, and `contenox run` invocation can connect to any MCP-compatible server—local child processes, remote SSE streams, or HTTP endpoints.

## What is MCP?

The [Model Context Protocol](https://modelcontextprotocol.io/) is an open standard (originally created by Anthropic and donated to the [AI Agentic Foundation](https://aaif.ai/) at the Linux Foundation) that lets AI agents talk to tools, memory stores, and data sources using a universal wire format.

Think of it as USB-C for AI: one standard connection, unlimited devices.

## What makes Contenox's MCP implementation different

Most clients treat MCP as a one-shot API call. Contenox does more: it keeps **persistent, session-scoped connections** to every registered MCP server.

- Each chat session or plan gets its own dedicated connections.  
- State is preserved across all tool calls within that session.  

Your agent doesn't just call a tool—it builds a lasting relationship with it.

## Register an MCP server

```bash
# Local stdio server (spawned as child process)
contenox mcp add myfiles \
  --transport stdio \
  --command npx \
  --args "-y,@modelcontextprotocol/server-filesystem,/home/user/projects"

# Remote SSE endpoint with bearer auth
contenox mcp add memory \
  --transport sse \
  --url https://mcp.example.com/sse \
  --auth-type bearer \
  --auth-env MCP_TOKEN

# Remote HTTP endpoint with injected context (hidden from model)
contenox mcp add internal \
  --transport http \
  --url http://internal-host:8090 \
  --header "X-Tenant: acme" \
  --inject "tenant_id=acme" --inject "env=production"
```

Manage servers like any other backend:

```bash
contenox mcp list
contenox mcp show myfiles
contenox mcp update myfiles --inject "tenant_id=newvalue"
contenox mcp remove myfiles
```

## Try the built-in test server

Contenox includes a simple MCP test server for experimentation:

```bash
go run ./cmd/mcp-testserver
# → listening on http://localhost:8090

contenox mcp add test --transport http --url http://localhost:8090
```

The test server returns a `session_token` in every response so you can see the persistent session in action.

## Use MCP servers in a chain

Reference them by name in `execute_config.hooks`:

```json
{
  "id": "ask_with_memory",
  "handler": "chat_completion",
  "system_instruction": "Available tools: {{hookservice:list}}.",
  "execute_config": {
    "model": "qwen2.5:7b",
    "provider": "ollama",
    "hooks": ["myfiles", "memory"]
  }
}
```

The chain engine automatically connects to each server at task start and keeps the connections open for the entire session.

## Supported transports

| Transport | How it works                              | Best for                              |
|-----------|-------------------------------------------|---------------------------------------|
| `stdio`   | Spawns child process, communicates via stdin/stdout | Local tools (filesystem, databases) |
| `sse`     | Connects to remote Server-Sent Events endpoint | Cloud or shared team servers         |
| `http`    | Connects via HTTP streaming               | Production and internal services     |

## Subprocess lifetime & persistence

`stdio` servers are started when a `contenox` command begins and killed when it ends. This applies only to the `stdio` transport — HTTP and SSE servers are external processes you manage yourself and are unaffected. Each invocation is clean and reproducible, with no leftover server state from previous runs.

For state that must survive across runs, choose servers that persist on their own:
- `@modelcontextprotocol/server-memory` writes its graph to disk  
- Remote HTTP/SSE servers you manage can hold any state you need

## Combine with `contenox plan`

MCP really shines in autonomous plans. A single plan can read files, store results, and call APIs with full continuity:

```bash
contenox plan new "explore the codebase, identify the top 3 bugs, write a report"
contenox plan next --auto
```

All steps in an `--auto` run share the same MCP connections. Manual steps (`contenox plan next` without `--auto`) start fresh processes, so only disk-backed or remote servers retain state between steps.

## Injecting hidden parameters

You can inject key-value pairs into every MCP tool call — and they will be **completely invisible to the model**. The model's tool schema never shows them; Contenox merges them in silently on every call.

```bash
contenox mcp add myserver --transport http --url http://localhost:8090 \
  --inject "tenant_id=acme" \
  --inject "correlation_id=trace-xyz"

# Update inject params without recreating the server
contenox mcp update myserver --inject "tenant_id=newvalue"
```

Injected values always override any same-named args the model might provide. Use this for: tenant context, correlation IDs, session tags, environment identifiers, or any infrastructure parameter the model doesn't need to reason about.

For HTTP request headers (SSE/HTTP transports), use `--header` instead:

```bash
contenox mcp update myserver --header "X-Tenant: acme" --header "X-Version: 2"
```

> [!NOTE]
> `mcp update --header` and `mcp update --inject` each replace the **entire** corresponding map. Pass all required values in one call.

## Security notes

- Use `--auth-env` instead of `--auth-token` to keep secrets out of shell history.  
- `--inject` and `--header` values are stored in SQLite and **never logged or shown by `mcp show`** (keys are shown, values are masked).  
- `stdio` servers run as child processes—limit their filesystem access.  
- Session tokens are scoped to the current CLI session and never stored in plain text.

## Further reading

- [Official MCP specification](https://modelcontextprotocol.io/)  
- [MCP server registry](https://github.com/modelcontextprotocol/servers)  
- [CLI reference: `contenox mcp`](/reference/contenox-cli#contenox-mcp)
