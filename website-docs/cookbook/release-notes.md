# Automated Release Notes

Use a contenox chain as a deterministic CI pipeline: run `git log`, send commits to an LLM, write `RELEASE_NOTES.md` — all orchestrated in JSON, no bash glue required.

---

## The chain

Save this as `.contenox/chain-release-notes.json`:

```json
{
    "id": "chain-release-notes",
    "description": "Deterministic pipeline: git log → LLM → RELEASE_NOTES.md",
    "tasks": [
        {
            "id": "get_git_log",
            "description": "Run git log from the tag passed as input to HEAD.",
            "handler": "hook",
            "hook": {
                "name": "local_shell",
                "args": {
                    "command": "git log --oneline \"$(cat)\"..HEAD",
                    "shell": "true"
                }
            },
            "output_template": "{{.Stdout}}",
            "transition": {
                "branches": [{ "operator": "default", "goto": "generate_notes" }]
            }
        },
        {
            "id": "generate_notes",
            "handler": "prompt_to_string",
            "input_var": "get_git_log",
            "system_instruction": "You are a release notes writer. Use ONLY the commits provided. Group under ## Features, ## Bug Fixes, ## Improvements, ## Documentation. Omit empty sections. No preamble.",
            "prompt_template": "Write release notes from ONLY these commits:\n\n{{.get_git_log}}\n\nGroup into markdown sections.",
            "execute_config": {
                "model": "{{var:model}}",
                "provider": "{{var:provider}}"
            },
            "transition": {
                "branches": [{ "operator": "default", "goto": "write_file" }]
            }
        },
        {
            "id": "write_file",
            "handler": "hook",
            "input_var": "generate_notes",
            "hook": {
                "name": "local_shell",
                "args": {
                    "command": "cat > RELEASE_NOTES.md",
                    "shell": "true"
                }
            },
            "transition": {
                "branches": [{ "operator": "default", "goto": "end" }]
            }
        }
    ],
    "token_limit": 32768
}
```

## Run it

Pass the previous release tag as `--input`. The chain reads from it, not from any state.

```bash
contenox run --shell \
  --chain .contenox/chain-release-notes.json \
  --input "v0.2.2"
```

Output in `RELEASE_NOTES.md`:

```markdown
## Improvements
- Improve session implementation
- Refactor input handling: Combine positional args and stdin; resolve default provider
```

---

## How it works

This is a **deterministic pipeline** — three tasks wired in sequence, no agentic loops:

| Step | Handler | What it does |
|---|---|---|
| `get_git_log` | `hook` → `local_shell` | Runs `git log --oneline "<input>"..HEAD`; `$(cat)` reads the tag from stdin |
| `generate_notes` | `prompt_to_string` | LLM formats the commit log into grouped markdown; `{{.get_git_log}}` injects the log |
| `write_file` | `hook` → `local_shell` | Pipes the release notes string into `cat > RELEASE_NOTES.md` |

The key template variable is `{{.get_git_log}}` — not `{{.input}}`. In a chain, `{{.input}}` is always the **original chain input** (the tag string). Each task's output is available under its task ID.

---

## Tips

- **Use a stronger model** for better grouping:
  ```bash
  contenox run --shell \
    --chain .contenox/chain-release-notes.json \
    --input "v0.2.2" \
    --model gemini-3.1-flash-lite-preview --provider gemini
  ```
- **Point at a specific tag**: `--input "v0.1.0"` covers everything since that tag
- **Upload to GitHub**: pipe the output into `gh release create v0.2.3 --notes-file RELEASE_NOTES.md`
