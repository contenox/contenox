# Remote Hooks

Remote hooks turn any external HTTP service into a set of callable tools for your AI agent. Contenox fetches the service's OpenAPI v3 spec, discovers every operation, and makes them available to the model as named tools — no client code required.

## Register a hook

```bash
contenox hook add <name> --url <endpoint>

# Example: US National Weather Service (free, public API)
contenox hook add nws --url https://api.weather.gov --timeout 15000

# Example: internal API with auth and hidden tenant context
contenox hook add myapi --url https://api.example.com \
  --header "Authorization: Bearer $MY_TOKEN" \
  --inject "tenant_id=acme" \
  --inject "env=production"
```

Contenox probes the endpoint at registration time to count available tools. If the service is unreachable at that moment, it is still registered and re-probed at chain execution time.

### Flags

| Flag | Description |
|---|---|
| `--url` | Base URL of the service (required) |
| `--header` | HTTP header to inject on every call, e.g. `"Authorization: Bearer $TOKEN"` (repeatable) |
| `--inject` | Tool call argument to inject and hide from the model, e.g. `"tenant_id=acme"` (repeatable) |
| `--timeout` | Request timeout in milliseconds (default: 10000) |

## Inspect tools

```bash
contenox hook show nws
```

Lists the hook's URL, timeout, registered headers (keys only — values are never shown), injected params (keys only — values hidden), and all tools discovered from its OpenAPI spec.

## Manage hooks

```bash
contenox hook list                               # show all registered hooks
contenox hook update nws --timeout 30000         # update timeout
contenox hook update nws --header "X-App: v2"    # replace ALL headers
contenox hook update nws --inject "tenant_id=newvalue"  # replace ALL inject params
contenox hook remove nws
```

> [!IMPORTANT]
> `hook update --header` **replaces** the entire header set for the hook. `hook update --inject` **replaces** the entire inject param map. Pass all required values in a single update call.

## Authentication and secret injection

Pass authentication headers at registration time:

```bash
contenox hook add myapi --url https://api.example.com \
  --header "Authorization: Bearer $MY_TOKEN" \
  --header "X-Tenant: acme"
```

These headers are stored in SQLite and injected transparently into every HTTP call made to that service. **The model never sees them** — they are stripped from the tool schema before it reaches the LLM.

## Injecting tool call arguments (hidden from model)

Beyond HTTP headers, you can also inject named parameters directly into every tool call — completely hidden from the model's tool schema:

```bash
contenox hook add myapi --url https://api.example.com \
  --inject "tenant_id=acme" \
  --inject "correlation_id=trace-123"
```

Specifically, the engine:
1. Removes injected parameter names from the tool manifest the model sees (`properties` + `required`)
2. Merges them back into every tool call **after** the model-provided args (injected values always win)

This is the right pattern for: tenant IDs, correlation/trace IDs, session context, environment tags, and any other infrastructure concern that the model shouldn't reason about.

## Tool naming

Contenox derives a tool name for each API operation in this priority order:

1. **`operationId`** from the OpenAPI spec (recommended)
2. **`x-tool-name`** extension on the operation
3. **Fallback**: `<last_path_segment>_<method>` (e.g. `alerts_get`)

For the best experience, set `operationId` on every operation in your OpenAPI spec.

## Excluded paths

The following paths are automatically excluded from tool discovery:

- `/health`, `/healthz` — health checks
- `/ready`, `/readyz` — readiness probes
- `/metrics` — Prometheus metrics

## Use in a chain

Add the hook's name to `execute_config.hooks`:

```json
{
  "id": "weather_task",
  "handler": "chat_completion",
  "system_instruction": "You are a weather assistant. Available tools: {{hookservice:list}}.",
  "execute_config": {
    "model": "qwen2.5:7b",
    "provider": "ollama",
    "hooks": ["nws"]
  },
  "transition": {
    "branches": [
      { "operator": "equals", "when": "tool-call", "goto": "run_tools" },
      { "operator": "default", "when": "", "goto": "end" }
    ]
  }
},
{
  "id": "run_tools",
  "handler": "execute_tool_calls",
  "input_var": "weather_task",
  "transition": {
    "branches": [
      { "operator": "default", "when": "", "goto": "weather_task" }
    ]
  }
}
```

## Building your own hook with FastAPI

FastAPI serves an `/openapi.json` spec automatically, making it a perfect fit. Every endpoint becomes a tool the moment you register the service.

```python
from fastapi import FastAPI

app = FastAPI()

@app.get("/summarize", operation_id="summarize_text")
def summarize(text: str) -> dict:
    """Return a short summary of the provided text."""
    return {"summary": text[:100] + "..."}
```

```bash
# Start the service
uvicorn main:app --port 8080

# Register it
contenox hook add myapp --url http://localhost:8080
contenox hook show myapp   # → 1 tool: summarize_text
```

The model can now call `summarize_text` directly from any chain that includes `myapp` in its `hooks` list.
