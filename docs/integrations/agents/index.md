---
title: External agents
description: Register and drive installed ACP agents through Contenox's client-side protocol host.
order: 4
---

# External agents

Contenox can sit on either side of the Agent Client Protocol. Editors drive
Contenox through `contenox acp`; Contenox drives another installed ACP agent
through `contenox agent add`, verifies it with `contenox agent check`, and
dispatches work with `contenox mission fire` or `/mission` in the TUI.

- [Host an external ACP agent](/docs/integrations/agents/external-acp/) — set up Codex or another ACP agent, run work from the TUI or terminal, and configure its sandbox and MCP servers
- [Editor integrations](/docs/integrations/editors/zed/) — use Contenox itself as the ACP agent
