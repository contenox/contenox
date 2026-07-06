# What are Tools?

Tools are the mechanism by which Contenox gives a model access to real-world actions. Instead of generating text, the model calls a tools to read files, run commands, query APIs, or fire HTTP requests — and gets the result back as context for its next reply.

## How it works

```
Chain starts
  └─ FetchTools: each listed tools returns its tool schemas
       └─ Schemas are sent to the model alongside the prompt
            └─ Model returns a tool call
                 └─ execute_tool_calls runs the tools
                      └─ Result appended to history → model continues
```

In your chain JSON, specify which tools the task can use via the `execute_config.tools` allowlist:

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "tools": ["local_fs", "nws", "local_shell"]
}
```

Pattern support:

| Value | Meaning |
|---|---|
| field absent / `null` | No registered tools exposed to the model |
| `[]` | No tools exposed to the model |
| `["*"]` | All registered tools |
| `["a", "b"]` | Only the named tools |
| `["*", "!local_shell"]` | All except `local_shell` |

Unknown names in an exact list are silently ignored — if `local_shell` is disabled the chain still runs.

Use <span v-pre>`{{toolservice:list}}`</span> in your `system_instruction` to inject the live tool manifest. This macro respects the task's `tools` allowlist — the model only sees what the task permits:

```json v-pre
"system_instruction": "You are a helpful assistant. Available tools: {{toolservice:list}}."
```

## Template variables

System instructions and `prompt_template` fields support the following macros:

| Macro | Returns |
|-------|---------|
| <span v-pre>`{{var:<name>}}`</span> | Value of the named template variable supplied by the caller |
| <span v-pre>`{{now}}`</span> | Current time in RFC3339 format |
| <span v-pre>`{{now:<layout>}}`</span> | Current time in Go time layout (e.g. `{{now:2006-01-02}}`) |
| <span v-pre>`{{chain:id}}`</span> | ID of the currently executing chain |
| <span v-pre>`{{toolservice:list}}`</span> | JSON object mapping tools name → array of tool names (respects task `tools` allowlist) |
| <span v-pre>`{{toolservice:tools}}`</span> | JSON array of tools names available to the task |
| <span v-pre>`{{toolservice:tools <name>}}`</span> | JSON array of tool names for a specific tool |

## Tools types

Contenox ships with built-in local tools and supports unlimited remote tools:

| Tools name | Type | Always available | What it does |
|---|---|---|---|
| `local_fs` | Local | ✅ | Read, write, and search files within a configured directory (10 verb-specific tools, read-before-write contract for mutations) |
| `webtools` | Local | ✅ | Call HTTP endpoints — `web_get`, `web_head`, `web_post`, `web_put`, `web_patch`, `web_delete`. SSRF-guarded; mutating verbs HITL-approve by default. |
| `local_shell` | Local | CLI opt-in | Run shell commands. `contenox run` and `contenox chat` require `--shell`; editor clients route shell execution through their own approval surface where supported. |
| `print` | Local | ✅ | Append a message to the chat history or return it as a string |
| `echo` | Local | ✅ | Echo the input back (useful for debugging chains) |
| _your name_ | Remote | Register with `contenox tools add` | Any OpenAPI v3 service |

## Choosing the right tools

- **`local_fs`** — best for code analysis, file editing, report generation. Prefer over `local_shell` for file ops; the read-before-write contract and sandbox guard against confabulated edits.
- **`webtools`** — when the model needs to call HTTP. Use `web_get` / `web_head` for retrieval; mutating verbs trigger HITL approval by default.
- **`local_shell`** — full power; use only in trusted, sandboxed environments. Reach for it for build / test / git, not for cat / grep / sed against project files.
- **`print`** / **`echo`** — inject messages or inspect task output during development
- **Remote tools** — turn any OpenAPI service into an agent tool; ideal for internal APIs, SaaS integrations, and team-shared tools

## Further reading

- [Remote Tools](/docs/tools/remote) — register external APIs as agent tools
- [Local Tools](/docs/tools/local) — built-in in-process tools reference
