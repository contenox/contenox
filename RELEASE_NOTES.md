# v1.2.0

Changes since v1.1.0:

- External ACP agents: register a program that speaks ACP (`contenox agent add <name> -- <command>`), verify it with one live turn (`contenox agent check`), edit its argv/env/cwd/MCP allowlist, and drive it from Beam or the CLI. The agent runs as a subprocess inside Contenox's Linux sandbox.
- Missions accept registered agents. `/mission` dispatches to one from the TUI, reports and questions arrive while the conversation continues, and `contenox beam --agent <name>` opens a session with a registered agent as the conversation partner.
- Experimental ChatGPT subscription backend: `contenox backend add chatgpt --type openai-codex` and `contenox backend login chatgpt` for device-code login, automatic token refresh, model discovery, and chat, streaming and function tools through Contenox's own agent loop.

Upgrade notes:

- `contenox backend add` validates `--type`; account-specific and custom endpoints require `--url`, and `openai-codex` rejects `--api-key`, `--api-key-env` and `--url`.
