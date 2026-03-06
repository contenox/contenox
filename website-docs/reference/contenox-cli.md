# contenox CLI Reference

`contenox` is the local AI agent CLI. It runs the Contenox chain engine entirely on your machine.

## Global Flags

| Flag | Description |
|------|-------------|
| `--trace` | Print verbose chain execution logs |
| `--steps` | Stream intermediate task names and tool executions |
| `--think` | Stream the model's reasoning/thinking trace to stderr before the main output |
| `--model <name>` | Override the model defined in `.contenox/config.yaml` |

## Subcommands

### `contenox chat` (or just `contenox`)

Starts an interactive chat session using the default chain (`.contenox/default-chain.json`).

```bash
contenox "what is the capital of France?"
contenox   # enters interactive REPL mode
contenox "list files here" --shell  # enable shell execution
contenox "explain recursion" --think  # show the model's reasoning process
```

### `contenox run`

Executes a specific chain non-interactively. Useful for wiring Contenox into bash scripts or CI pipelines.

```bash
contenox run --chain .contenox/chain-nws.json --input-type chat "how is the weather?"
contenox run --chain .contenox/my-chain.json --shell "refactor main.go"
```

- `--chain <path>`: Required. Path to the chain JSON file.
- `--input-type <type>`: How to parse the positional argument. `chat` treats it as a user message. `string` treats it as raw string input. Defaults to `string`.
- `--shell`: Enable shell execution for this invocation (use only in trusted environments).
- `--think`: Stream the model's reasoning trace to stderr before outputting the final result.

### `contenox plan`

Autonomous multi-step execution using a separate "planner" model that directs an "executor" model.

```bash
contenox plan new "analyze main.go, find the bug, and write a fix to patch.diff"
contenox plan next          # execute next pending step
contenox plan next --shell  # execute next step with shell access enabled
contenox plan next --auto   # run all pending steps automatically
```

- `--shell`: Enable shell execution for the step (only applies to `plan next`, use only in trusted environments).
- `--steps`: Live-stream intermediate tool calls and their outputs during execution.
- `--think`: Stream the model's reasoning/chain-of-thought to stderr before it takes action (for thinking models).
- `--trace`: Verbose debugging output of the step-executor state machine.

### `contenox hook`

Manage remote OpenAPI hooks. See [Remote Hooks](/hooks/remote).

```bash
contenox hook add <name> --url <url>
contenox hook list
contenox hook show <name>
contenox hook remove <name>
```

### `contenox init`

Initializes a new `.contenox/` configuration directory in the current path.
