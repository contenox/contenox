# Cookbook

Practical, copy-paste recipes for automating real tasks with `contenox run`.

Each recipe works the same way: `contenox run` is a **stateless, composable execution engine** — you pipe data in, the model does the work, you get output back. No chat history, no sessions, just results.

> **Prerequisites:** `contenox init` has been run, a backend is configured, and `local_shell_allowed_commands` is set in `.contenox/config.yaml`.

## Categories

- [Git & DevOps](/cookbook/git-devops) — commit messages, PR reviews, log summarization
- [Automated Release Notes](/cookbook/release-notes) — generate `RELEASE_NOTES.md` from `git log` using a chain pipeline
