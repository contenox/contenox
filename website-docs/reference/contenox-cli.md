# contenox CLI Reference

`contenox` is the local AI agent CLI. It runs the Contenox chain engine entirely on your machine.

## Global Flags

| Flag | Description |
|------|-------------|
| `--trace` | Print verbose chain execution logs |
| `--steps` | Stream intermediate task names and tool executions |
| `--enable-local-exec` | Opt-in to allow the model to run shell commands (`local_shell` hook) |
| `--model <name>` | Override the model defined in `.contenox/config.yaml` |

## Subcommands

### `contenox run` (or just `contenox`)

Starts an interactive chat session using the default chain (`.contenox/default-chain.json`).

```bash
contenox "what is the capital of France?"
contenox   # enters interactive REPL mode
```

### `contenox exec`

Executes a specific chain non-interactively. Useful for wiring Contenox into bash scripts or CI pipelines.

```bash
contenox exec --chain .contenox/chain-nws.json --input-type chat "how is the weather?"
```

- `--chain <path>`: Required. Path to the chain JSON file.
- `--input-type <type>`: How to parse the positional argument. `chat` treats it as a user message. `string` treats it as raw string input. Defaults to `string`.

### `contenox plan`

Autonomous multi-step execution using a separate "planner" model that directs an "executor" model.

```bash
contenox plan "analyze main.go, find the bug, and write a fix to patch.diff" --enable-local-exec
```

- `--planner-model`: Override the model used for planning.
- `--executor-model`: Override the model used for executing steps.

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
