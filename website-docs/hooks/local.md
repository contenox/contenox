# Local Hooks

Local hooks run directly inside the Contenox process — no network, no subprocess, no protocol overhead. They are the fastest way to give a model access to the machine it's running on.

## `local_fs` — Filesystem access

Always available. Provides read, write, search, and metadata operations scoped to a configured directory. **All paths are validated** against the allowed directory; attempts to escape with `../` are rejected.

Configure the allowed root via `contenox config set local-exec-allowed-dir /path/to/project` or the `--allowed-dir` flag.

### Tools

| Tool | Parameters | Description |
|---|---|---|
| `read_file` | `path` | Read the full content of a file |
| `write_file` | `path`, `content` | Write content to a file (creates parent dirs, overwrites) |
| `list_dir` | `path` (optional) | List entries in a directory (dirs marked with `/`) |
| `read_file_range` | `path`, `start_line`, `end_line` | Read a specific line range |
| `grep` | `path`, `pattern` | Find lines containing a string (returns `line_number: content`) |
| `sed` | `path`, `pattern`, `replacement` | Replace all occurrences of a string in a file |
| `count_stats` | `path` | Count lines, words, and bytes (like `wc`) |
| `stat_file` | `path` | Get file metadata: name, size, mod time, isDir |

### Chain example

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "hooks": ["local_fs"]
}
```

---

## `webhook` — Arbitrary HTTP calls

Always available. Lets the model call any HTTP endpoint directly. Unlike remote hooks (which require an OpenAPI spec), `webhook` is a single generic tool — the model decides the URL, method, query params, and headers at call time.

> [!CAUTION]
> Because the model controls the destination URL, only use the `webhook` hook in chains where the model prompt restricts the scope (e.g. "only call endpoints on `api.internal`"). Do not use it in chains exposed to untrusted user input.

### Tool

**`webhook`**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `url` | string | ✅ | The URL to call |
| `method` | string | — | HTTP method: `GET`, `POST`, `PUT`, `PATCH`, `DELETE` (default: `POST`) |
| `query` | string | — | Query string (e.g. `q=foo&limit=10`) |
| `headers` | object | — | JSON object of headers to add |

### Chain example

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "hooks": ["webhook"]
}
```

---

## `js_execution` — JavaScript sandbox

Always available. Runs JavaScript code in an isolated [Goja](https://github.com/dop251/goja) sandbox. Useful for lightweight computation, data transformation, or scripted logic that doesn't require shell access.

### Tool

**`execute_js`**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `code` | string | ✅ | JavaScript code to execute |

The sandbox has no access to the filesystem, network, or host processes. It provides a safe environment for computation-only tasks.

### Chain example

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "hooks": ["js_execution"]
}
```

---

## `local_shell` — Shell command execution

> [!CAUTION]
> `local_shell` gives the model direct access to run arbitrary commands on your machine. **Never enable it in public-facing deployments or when processing untrusted user input.**

Opt-in only — disabled by default. Enable per-invocation with the `--shell` flag:

```bash
contenox run --shell "clean up unused imports in the codebase"
contenox plan next --auto --shell
```

**Command policy is set in the chain, not on the CLI.** Add a `hook_policies` block to `execute_config`:

```json
"execute_config": {
  "model": "{{var:model}}",
  "provider": "{{var:provider}}",
  "hooks": ["local_shell"],
  "hook_policies": {
    "local_shell": {
      "_allowed_commands": "git,go,make,ls,cat",
      "_denied_commands":  "sudo,su,dd,mkfs"
    }
  }
}
```

- `_allowed_commands` — comma-separated list of permitted command names. When set, any command not on this list is rejected before it runs.
- `_denied_commands` — comma-separated commands that are always blocked, regardless of the allowlist.
- `_allowed_dir` — if set, the command (or its absolute path) must reside under this directory.

The default chains (`default-chain.json`, `default-run-chain.json`) ship with sensible defaults: common dev tools allowed, privilege-escalation and raw-disk commands denied.

To use `local_shell` with **no policy restrictions** (fully open), omit `hook_policies` entirely. Only do this in fully-trusted, local-only environments — the HITL approval gate in `contenox vibe` is still active regardless.

### Tool

**`local_shell`**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `command` | string | ✅ | Executable path or name |
| `args` | string | — | Space-separated arguments |
| `cwd` | string | — | Working directory |
| `timeout` | string | — | Duration e.g. `30s` |
| `shell` | boolean | — | Run via `/bin/sh -c` (allows pipes, redirects, `$VAR`). **Disabled when `_allowed_commands` or `_allowed_dir` is set.** |

---

## Adding custom local hooks

Adding new local hook types requires modifying the Contenox Go source code and implementing the `taskengine.HookRepo` interface. For custom capabilities without writing Go, build a small HTTP service (FastAPI, Express, etc.) and register it as a [Remote Hook](/hooks/remote) instead — no code changes required.
