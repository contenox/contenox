# What are Hooks?

Hooks are the mechanism by which Contenox gives a model access to real-world actions. Instead of generating text, the model calls a hook to read files, run commands, query APIs, or fire HTTP requests — and gets the result back as context for its next reply.

## How it works

```
Chain starts
  └─ FetchTools: each listed hook returns its tool schemas
       └─ Schemas are sent to the model alongside the prompt
            └─ Model returns a tool call
                 └─ execute_tool_calls runs the hook
                      └─ Result appended to history → model continues
```

In your chain JSON, specify which hooks the task can use:

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "hooks": ["local_fs", "nws", "local_shell"]
}
```

Use `{{hookservice:list}}` in your `system_instruction` to inject the live tool manifest into the system prompt so the model knows exactly what tools are available:

```json
"system_instruction": "You are a helpful assistant. Available tools: {{hookservice:list}}."
```

## Hook types

Contenox ships with four built-in local hooks and supports unlimited remote hooks:

| Hook name | Type | Always available | What it does |
|---|---|---|---|
| `local_fs` | Local | ✅ | Read, write, and search files within a configured directory |
| `webhook` | Local | ✅ | Call any HTTP endpoint |
| `js_execution` | Local | ✅ | Execute sandboxed JavaScript |
| `local_shell` | Local | Opt-in (`--shell`) | Run arbitrary shell commands |
| _your name_ | Remote | Register with `contenox hook add` | Any OpenAPI v3 service |

## Choosing the right hook

- **`local_fs`** — best for code analysis, file editing, report generation
- **`webhook`** — when the model needs to call a specific URL you control
- **`js_execution`** — lightweight computation or data transformation without shell access
- **`local_shell`** — full power; use only in trusted, sandboxed environments
- **Remote hooks** — turn any OpenAPI service into an agent tool; ideal for internal APIs, SaaS integrations, and team-shared tools

## Further reading

- [Remote Hooks](/hooks/remote) — register external APIs as agent tools
- [Local Hooks](/hooks/local) — built-in in-process hooks reference
