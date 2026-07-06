# Handlers

Every task has a `handler` field that determines what it does. This page documents all available handlers and which fields are valid for each.

## Handler types

| Handler | What it does |
|---------|-------------|
| `chat_completion` | Send messages to an LLM, receive a text/tool-call reply |
| `execute_tool_calls` | Execute the tool calls from the previous LLM reply |
| `tools` | Call a specific named tools tool directly (no LLM involved) |
| `route` | LLM picks exactly one of the declared branch labels; routing-only, input passes through unchanged |
| `raise_error` | Immediately halt the chain with an error message |
| `noop` | Pass input through unchanged |

---

## `chat_completion`

Sends the current input to the LLM and waits for a reply. If the model calls a tool, the transition evaluates to `"tool_call"`.

**Key fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `system_instruction` | No | System prompt (supports macros) |
| `execute_config.model` | Yes | Model name, e.g. `qwen2.5:7b` |
| `execute_config.provider` | Yes | `ollama`, `openai`, `anthropic`, `mistral`, `vllm`, `gemini`, `bedrock`, `vertex-google` |
| `execute_config.tools` | No | Tools allowlist: `[]`=none, `["*"]`=all, `["a","b"]`=named, `["*","!x"]`=all-except. Absent=all (backward compat). |
| `execute_config.hide_tools` | No | Tools to suppress from the model |
| `execute_config.temperature` | No | Sampling temperature (0–1) |
| `execute_config.think` | No | Reasoning effort level. `"low"`, `"medium"`, `"high"`, or `"false"`. Supported by Ollama (v0.17.5+), Gemini 2.5+, vLLM, and OpenAI o-series models. |
| `execute_config.max_tokens` | No | Cap on the model's output tokens for this task. When unset, the engine falls back to the chain's `token_limit` so providers (notably Gemini thinking models) don't burn their entire output budget on hidden reasoning and emit empty content. |
| `execute_config.shift` | No | Boolean. If true, slides the context window by dropping old messages instead of erroring on token limits. |
| `execute_config.truncate` | No | Boolean. If true, truncates the initial prompt instead of sliding the context window (Ollama-specific). |
| `execute_config.models` | No | Array of fallback model IDs tried in order when the primary model is unavailable. |
| `execute_config.providers` | No | Array of fallback provider types, paired index-for-index with `models`. |

| `execute_config.retry_policy` | No | LLM-call retry and model-fallback settings — see [`retry_policy`](#retry_policy) below. |

**Transition values:**
- `"tool_call"` — model issued one or more tool calls
- `"executed"` — model replied with text and finished (no tool calls)

(These are control tokens, not the model's text. To branch on the model's actual
answer, use the [`route`](#route) handler.)



### `retry_policy`

Controls automatic retries on transient LLM errors and optional model swapping after repeated failures.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_attempts` | int | `1` | Total attempts including the first (`0` or `1` disables retry) |
| `initial_backoff` | duration | `"500ms"` | Wait before the second attempt; doubled each retry |
| `max_backoff` | duration | — | Cap on exponential backoff |
| `jitter` | float | `0` | 0–1 fraction of backoff added as random noise |
| `rate_limit_min_wait` | duration | — | Minimum wait when the provider returns a rate-limit error |
| `fallback_model_id` | string | — | Alternate model ID to switch to after `fallback_after` consecutive failures |
| `fallback_after` | int | — | Failure count that triggers the model swap |

**Example:**
```json
{
  "id": "chat",
  "handler": "chat_completion",
  "system_instruction": "You are a helpful assistant. Today is <span v-pre>{{now:2006-01-02}}</span>.",
  "execute_config": {
    "model": "<span v-pre>{{var:model}}</span>",
    "provider": "<span v-pre>{{var:provider}}</span>",
    "tools": ["nws"]
  },
  "transition": {
    "branches": [
      { "operator": "equals", "when": "tool_call", "goto": "run_tools" },
      { "operator": "default", "when": "", "goto": "end" }
    ]
  }
}
```

---

## `execute_tool_calls`

Executes the tool calls emitted by the previous `chat_completion` task, appends the results to the chat history, and loops back.

**Key fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `input_var` | Yes | ID of the `chat_completion` task whose output to use |

**Example:**
```json
{
  "id": "run_tools",
  "handler": "execute_tool_calls",
  "input_var": "chat",
  "transition": {
    "branches": [
      { "operator": "default", "when": "", "goto": "chat" }
    ]
  }
}
```

---

## `tools`

Calls a specific tool on a named tool directly — no LLM involved. Use for deterministic side effects (e.g. writing a file, calling a fixed API endpoint).

**Key fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `tools.name` | Yes | Registered tools name (e.g. `local_shell`) |
| `tools.tool_name` | Yes | Tool/operation to call on that tool |
| `tools.args` | No | Static arguments passed to the tool |
| `output_template` | No | Go `text/template` string rendered against the tools's JSON response. Variables are the response fields (e.g. <span v-pre>`{{.exit_code}}`</span>). Output stored as a string. |

**Example:**
```json
{
  "id": "write_file",
  "handler": "tools",
  "tools": {
    "name": "local_fs",
    "tool_name": "write_file",
    "args": { "path": "/tmp/output.txt" }
  }
}
```

---

## `route`

Asks the LLM to classify the input into exactly one of the labels declared by this task's own `equals` transition branches, then routes to the matching task. The routes are the branch `when` values — there is no separate schema; the chain you can read *is* the route set.

`route` is **routing-only**: the task's input passes through to the next task unchanged. It produces a control-flow decision, never transformed data — so a router can never silently reshape what a downstream task sees. If the model's answer matches no declared label, the `default` branch is taken.

**Key fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `system_instruction` | No | Describes the classification; the engine appends the allowed labels |
| `execute_config.model` / `provider` | Yes | Model to use |
| `transition.branches` | Yes | The `equals` branches whose `when` values are the route labels; include a `default` |

---

## Template functions

The Go `text/template` fields — `prompt_template` and `output_template` — are rendered with the [Sprig](https://masterminds.github.io/sprig/) function library available in addition to the built-ins. This is what turns a structured task output into clean prompt input instead of Go's default struct formatting:

```text
<span v-pre>{{ .scan_result | toJson }}</span>            serialize a JSON/struct value as clean JSON
<span v-pre>{{ .page_text | trunc 4000 }}</span>            cap an oversized tool output
<span v-pre>{{ (last .history.Messages).Content }}</span>   pull a single field out
```

Without it, injecting a non-string task output (a tool's JSON result, a chat history) into a template renders Go syntax like `map[...]` or `{[{...}]}` that the model cannot reliably parse — which is why transforming or evaluating tool output used to mean shelling out. `toJson`, `trunc`, and the rest of Sprig remove that step.

---

## `noop`

Passes input through to the next task unchanged. Useful as an explicit routing node.

---

## `raise_error`

Immediately halts the chain with the input string as the error message. Use as a terminal error branch — no transition needed.

