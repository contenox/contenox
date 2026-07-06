# Transitions & Branching

Task chains are state machines. When a task finishes running its `handler`, the chain evaluates its `transition` rules to determine which task to execute next.

```json
"transition": {
  "branches": [
    { "operator": "equals", "when": "tool_call", "goto": "run_tools" },
    { "operator": "default", "when": "",         "goto": "end" }
  ]
}
```

## How transitions work

1. The current task returns a result string (the "eval").
2. The engine checks the `transition.branches` array from top to bottom.
3. It evaluates the `when` condition against the eval string using the `operator`.
4. The first branch that evaluates to `true` determines the next step (`goto`).

If the branch specifies `"goto": "end"`, the chain terminates successfully.

## `on_failure`

A task ID to jump to when the current task raises an error — evaluated before any branch conditions. If `on_failure` is absent and the task errors, the chain terminates.

```json
"transition": {
  "on_failure": "error_handler",
  "branches": [
    { "operator": "default", "when": "", "goto": "next_step" }
  ]
}
```

## Operators

| Operator | How it matches | Example |
|----------|---------------|---------|
| `equals` | Exact string match | `"when": "tool_call"` matches `"tool_call"` |
| `contains` | Substring match | `"when": "fail"` matches `"api_failure"` |
| `starts_with` | Prefix match | `"when": "err"` matches `"error_timeout"` |
| `ends_with` | Suffix match | `"when": "_ok"` matches `"write_ok"` |
| `edge_traversed_at_least` | Fires once an edge has been traversed N times this run; reads engine state, not task output | `"edge": "chat->run_tools", "when": "20"` |
| `default` | Always matches | Used as the fallback at the end of the array |

## What do tasks return?

Each handler returns a fixed **control token** as its eval — these are not the
model's text. To branch on what the model actually said, use `route`.

- **`chat_completion`**: `"tool_call"` (model requested tools) or `"executed"` (replied with text, no tool calls).
- **`execute_tool_calls`**: `"tools_executed"` (ran the calls), `"no_calls_found"` (model produced no tool calls), `"noop"` (empty history), or `"failed"`.
- **`tools`**: `"tools_executed"` or `"failed"` — or, when `output_template` is set, the rendered template string.
- **`route`**: the chosen label — one of this task's declared `equals` branch `when` values (the raw model answer falls through to the `default` branch). Input passes through unchanged.
- **`noop`**: passes the input through; eval is `"noop"`.
- **`raise_error`**: terminates the chain with an error — no branch is evaluated.

Place a `default` branch last as the fallback. For agentic loops, put an `edge_traversed_at_least` branch ahead of the loop branch to bound iterations.

## Reading edge counts from a prompt

The same counter that backs `edge_traversed_at_least` is exposed to `system_instruction` (and other template fields) as a macro:

```text
<span v-pre>{{edge_count:from_task_id->to_task_id}}</span>
```

It expands at every task step to the live count of how many times that edge has been traversed in the current chain run, starting at `0`. Resolves to `0` for edges that have never fired (typos won't break the prompt mid-turn).

This unlocks a **self-paced agent** pattern: instead of splitting a 20-round budget across a main agent + a recovery agent that hands off at round 10 with a frozen "10 of 20" warning, you have **one** chat task whose `system_instruction` shows the live count. The model sees the budget grow on every turn and self-paces accordingly. When the `edge_traversed_at_least` ceiling fires, the chain still routes to a tool-less terminal task for a clean wrap-up. See [Self-paced agent with dynamic budget](/docs/chains/examples#self-paced-agent-with-dynamic-budget) for the full example.
