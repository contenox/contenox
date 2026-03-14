# Cookbook

Practical, copy-paste recipes for automating real tasks with `contenox run`.

Each recipe works the same way: `contenox run` is a **stateless, composable execution engine** — you pipe data in, the model does the work, you get output back. No chat history, no sessions, just results.

> **Prerequisites:** `contenox init` has been run, a backend is registered (`contenox backend add`), and shell access is enabled via `--shell`. Command policy is configured in the chain's `hook_policies` — the default chain ships with a sensible allowlist.

## Categories

- [Git & DevOps](/cookbook/git-devops) — commit messages, PR reviews, log summarization
- [Automated Release Notes](/cookbook/release-notes) — generate `RELEASE_NOTES.md` from `git log` using a chain pipeline
- [Stateful Agents with MCP](/cookbook/stateful-agents-mcp) — persistent memory across tool calls using the Model Context Protocol
- [Autonomous Planning](/cookbook/autonomous-planning) — scaffold projects and execute multi-step goals with local tools
