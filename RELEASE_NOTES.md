# v1.1.0

Changes since v1.0.0:

- Native local inference through modeld, with managed worker installation and startup, llama.cpp and OpenVINO backends, and curated model downloads.
- `contenox auto` selects a model for available hardware, reserves room for context, checks tool calling and opens the TUI. `contenox pull` supports manual selection.
- `contenox gateway serve` exposes Ollama-compatible endpoints and OpenAI-compatible chat, models and embeddings endpoints with caller keys, allowances, usage reporting and session-aware routing.
- Harness usage metering through `contenox usage`, with thinking tokens tracked separately and configurable thinking-token allowance discounts in the gateway.
- Descriptive configuration names, effective-value explanations, and matching TUI slash commands and ACP settings. Legacy configuration names remain accepted.
- Fixed `/compact` failing to find the installed compaction chain.

Upgrade notes:

- Session settings now stay in the session; use `contenox config set` to save defaults. Existing agent limits remain in effect; set `chain.token_limit = 0` to inherit the context budget.
- Removed the previous `contenox serve` host and relay pairing integration.
- Native workers are distributed separately from the CLI; availability depends on the platform's published worker packages.
