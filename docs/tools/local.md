# Local Tools

Local tools run on the same machine as Contenox. Most run directly inside the process; `local_shell` starts subprocesses when shell access is enabled. They are the fastest way to give a model controlled access to the machine it's running on.

## `local_fs` — Filesystem access

Always available. Provides read, write, search, and metadata operations scoped to a configured directory. **All paths are validated** against the allowed directory; attempts to escape with `../` are rejected.

The filesystem root is set when Contenox registers the local tool:

- `contenox run` / `contenox chat`: `--local-exec-allowed-dir <dir>` sets the root. Without a root, `local_fs` rejects file paths.
- ACP and VS Code sessions use the editor/client workspace context where available.

`tools_policies.local_fs` controls read/output limits, denied path substrings, list filtering, and can override the root for a specific task with `_allowed_dir`.

### Tools

| Tool | Parameters | Description |
|---|---|---|
| `read_file` | `path` | Read the full content of a file. Also satisfies the read-before-mutate prerequisite for `write_file` / `sed` against the same path. |
| `write_file` | `path`, `content` | Write content to a file (creates parent dirs, overwrites). For *existing* files, requires a prior full `read_file` against the same current file version in this session. |
| `list_dir` | `path` (optional) | List entries in a directory (dirs marked with `/`) |
| `read_file_range` | `path`, `start_line`, `end_line` | Read a specific line range. Satisfies targeted `sed` mutations, but not full-file `write_file` overwrites. |
| `grep` | `path`, `pattern` | Find lines containing a string (returns `line_number: content`) |
| `find_files` | `pattern`, `path` (optional) | Find paths by glob pattern under the allowed root. |
| `search_repo` | `pattern`, `path` (optional), `glob` (optional), `regex` (optional) | Search file contents across the repo or a subtree. |
| `sed` | `path`, `pattern`, `replacement` | Replace all occurrences of a string in a file. For existing files, requires a prior `read_file` or `read_file_range` of the same path in this session. |
| `count_stats` | `path` | Count lines, words, and bytes (like `wc`) |
| `stat_file` | `path` | Get file metadata: name, size, mod time, isDir |

### Read-before-write contract

`write_file` against an *existing* file is blocked unless the same session has previously called full `read_file` on that exact current file version. A line-range read is not enough for full-file overwrite because unseen content could be destroyed.

`sed` is a targeted mutator, so either `read_file` or `read_file_range` against the same path can satisfy its prerequisite. New files (paths that do not yet exist) are unaffected.

The model receives a soft denial it can act on: it sees a tool result instructing it to read the file first, then retry the mutation.

This is a deterministic guard — no LLM judgement involved — designed to prevent confabulated edits to files the model has never seen. The contract is scoped per session: a read in one `contenox session` does not satisfy a write in another. The state lives in a private `local_fs_reads` table the tool maintains itself; the chain engine has no visibility into it.

If the model uses `local_shell` (`cat`, `head`, `grep`, `sed`) instead of `local_fs.read_file`, the guard does *not* count it as a satisfying read — by design. The shell tools are not bounded the same way and broadening the guard to recognise their output reliably is impractical. Prefer `local_fs.*` tools for file inspection (the default chains include a `TOOL PREFERENCE` system-prompt addendum that nudges the model toward this).

### `tools_policies.local_fs` keys

Set per-task read/output limits and denied path substrings by adding a `tools_policies.local_fs` block to `execute_config`:

| Key | Type | Default | Description |
|---|---|---|---|
| `_allowed_dir` | path | registration root | Override the allowed filesystem root for this task. Relative paths resolve against the active workspace/cwd where available. |
| `_max_read_bytes` | int | `1048576` (1 MiB) | Max file size for `read_file` / `read_file_range` / `grep` / `sed` / `count_stats`. `0` or negative = unlimited. Larger files return an error so the model can narrow with `read_file_range`. |
| `_max_output_bytes` | int | `524288` (512 KiB) | Max byte size of any tool result returned to the model (UTF-8 bytes). Prevents listing a huge directory or grepping a large file from blowing up context. `0` or negative = unlimited. |
| `_max_list_depth` | int | `6` | Cap on `list_dir(recursive=true)`. Hard-capped at 32 regardless of policy. |
| `_max_grep_matches` | int | `5000` | Stops `grep` after this many matching lines and returns an error so the model narrows the pattern. Hard-capped at 500000. |
| `_max_find_results` | int | `200` | Caps `find_files` path results. Hard-capped at 5000. |
| `_skip_dir_names` | comma-sep | `.git,node_modules,.venv,__pycache__,.next,dist,.cache,vendor,target,.idea,.vscode` | Directory basenames omitted by recursive `list_dir`, `find_files`, and `search_repo`. Set to empty string to disable filtering. |
| `_list_extensions` | comma-sep | empty | Optional file extension filter for recursive `list_dir` output, e.g. `.go,.md,.json`. |
| `_denied_path_substrings` | comma-sep | empty | Path substrings that always reject (e.g. `node_modules,.git/,dist/`). Matched against the path relative to the allowed root. |

```json
"tools_policies": {
  "local_fs": {
    "_allowed_dir": ".",
    "_max_read_bytes": "1048576",
    "_max_output_bytes": "524288",
    "_max_list_depth": "6",
    "_max_grep_matches": "5000",
    "_max_find_results": "200",
    "_skip_dir_names": ".git,node_modules,.venv,__pycache__,.next,dist,.cache,vendor,target,.idea,.vscode",
    "_denied_path_substrings": "node_modules,.git/,dist/,/.next/,/out/,package-lock.json"
  }
}
```

Values are strings even when conceptually numeric — `tools_policies` is the chain's policy carrier and uses string values uniformly across tools. The default chains (`default-chain.json`, `default-run-chain.json`) ship with conservative limit, root, and deny-substring defaults.

### Chain example

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "tools": ["local_fs"]
}
```

---

## `webtools` — HTTP calls

Always available. Lets the model call any HTTP endpoint via per-verb tools. Unlike remote tools (which require an OpenAPI spec), `webtools` exposes six generic verb tools — the model picks the verb, URL, query params, headers, and body at call time.

> [!CAUTION]
> Because the model controls the destination URL, every request is gated by SSRF and size limits configured via `tools_policies.webtools` (see below). The defaults block link-local / loopback / cloud-metadata addresses and cap response size at 1 MiB. Mutating verbs (`web_post`, `web_put`, `web_patch`, `web_delete`) trigger a HITL approval prompt by default. Do not point chains at untrusted user input without tightening the policy further.

### Tools

| Tool | Parameters | Description |
|---|---|---|
| `web_get` | `url`, `headers?`, `query?` | HTTP GET. Use for read-only retrieval. Default-allow under HITL. |
| `web_head` | `url`, `headers?`, `query?` | HTTP HEAD. Inspect headers / status without fetching the body. Default-allow under HITL. |
| `web_post` | `url`, `headers?`, `query?`, `body?` | HTTP POST. **HITL-approve by default.** |
| `web_put` | `url`, `headers?`, `query?`, `body?` | HTTP PUT. **HITL-approve by default.** |
| `web_patch` | `url`, `headers?`, `query?`, `body?` | HTTP PATCH. **HITL-approve by default.** |
| `web_delete` | `url`, `headers?`, `query?`, `body?` | HTTP DELETE. **HITL-approve by default.** |

**Parameter shapes:**

| Parameter | Type | Description |
|---|---|---|
| `url` | string | Absolute URL. Scheme must be in `_allowed_schemes` (default `http,https`). Host is checked against `_allowed_hosts` / `_denied_hosts`. |
| `headers` | object \| string | JSON object `{"X-Foo":"bar"}` (preferred). A JSON-encoded string is also accepted for back-compat. |
| `query` | string | URL-encoded query string (e.g. `a=1&b=2`). Merged with the URL's existing query. |
| `body` | any | Used by mutating verbs only. Strings sent as-is; any other JSON value is marshalled. Capped by `_max_request_body_bytes` (default 256 KiB). |

### `tools_policies.webtools` keys

| Key | Type | Default | Description |
|---|---|---|---|
| `_allowed_hosts` | comma-sep | empty (any host) | When set, *only* listed hosts pass. |
| `_denied_hosts` | comma-sep | `169.254.169.254,169.254.170.2,localhost,127.0.0.1,0.0.0.0,::1,metadata.google.internal,metadata.azure.com` | Empty string `""` opts out of the SSRF baseline. |
| `_allowed_schemes` | comma-sep | `http,https` | Block `file://`, `gopher://`, `ftp://`, etc. |
| `_max_response_bytes` | int | `1048576` (1 MiB) | `0` or negative = unlimited. Truncated responses include a marker. |
| `_max_request_body_bytes` | int | `262144` (256 KiB) | `0` or negative = unlimited. Oversized body blocks the call before sending. |
| `_request_timeout_seconds` | int | `30` | Per-call timeout. |
| `_max_attempts` | int | `3` | Retries 5xx and transport errors only — never 4xx. |
| `_initial_backoff_ms` | int | `250` | Exponential backoff with jitter. |
| `_max_backoff_ms` | int | `5000` | Cap on the exponential backoff. |
| `_disallow_redirects` | bool-string | `"false"` | When `"true"`, blocks all 3xx redirect-following. |

### Chain example

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "tools": ["webtools"],
  "tools_policies": {
    "webtools": {
      "_allowed_hosts": "api.github.com,api.openai.com",
      "_max_response_bytes": "524288",
      "_request_timeout_seconds": "20"
    }
  }
}
```

---

## `local_shell` — Shell command execution

> [!CAUTION]
> `local_shell` gives the model direct access to run arbitrary commands on your machine. **Never enable it in public-facing deployments or when processing untrusted user input.**

For direct CLI use, `local_shell` is opt-in. Enable it per invocation with `--shell`:

```bash
contenox run --shell "clean up unused imports in the codebase"
contenox chat --shell "run the tests and fix anything that breaks"
```

**Command policy is set in the chain, not on the CLI.** Add a `tools_policies` block to `execute_config`:

```json
"execute_config": {
  "model": "{{var:model}}",
  "provider": "{{var:provider}}",
  "tools": ["local_shell"],
  "tools_policies": {
    "local_shell": {
      "_allowed_commands": "git,go,make,ls,cat",
      "_denied_commands":  "sudo,su,dd,mkfs"
    }
  }
}
```

- `_allowed_commands` — comma-separated list of permitted command names. When set, any command not on this list is rejected before it runs.
- `_denied_commands` — comma-separated commands that are always blocked, regardless of the allowlist.
- `_allowed_dir` — if set, the command executable or script path must reside under this directory. The global `--local-exec-allowed-dir` flag sets the same executable/script boundary for an invocation.

The default chains (`default-chain.json`, `default-run-chain.json`) ship with sensible defaults: common dev tools allowed, privilege-escalation and raw-disk commands denied.

To use `local_shell` with **no policy restrictions** (fully open), omit `tools_policies` entirely. Only do this in fully trusted, local-only environments. Review tool use in your chain and enable shell only when you intend to grant command execution.

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

## `print` — Append to conversation

Always available. Appends a message to the chat history as a system message, or returns the message as a plain string when no chat history is active.

### Tool

**`print`**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `message` | string | ✅ | Text to append or return |

### Chain example

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "tools": ["print"]
}
```

---

## `echo` — Debug passthrough

Always available. Echoes the input back, prefixed with `"Echo: "`. Useful for verifying what a task receives during chain development.

### Tool

**`echo`**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `input` | string | ✅ | Text to echo |

### Chain example

```json
"execute_config": {
  "model": "qwen2.5:7b",
  "provider": "ollama",
  "tools": ["echo"]
}
```

---

## Adding custom local tools

Adding new local tools types requires modifying the Contenox Go source code and implementing the `taskengine.HookRepo` interface. For custom capabilities without writing Go, build a small HTTP service (FastAPI, Express, etc.) and register it as a [Remote Tools](/docs/tools/remote) instead — no code changes required.
