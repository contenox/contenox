// The one-paragraph product description. It is the first thing a model reads in
// both /llms.txt and /llms-full.txt, so it lives here once rather than being
// kept in sync by hand in two route files.
export const PRODUCT_SUMMARY =
  'contenox is the open execution system for agentic work. The operator sets the objective, supplies the capabilities, chooses the models and infrastructure, and defines the limits. `contenox beam` provides the first-party terminal, `contenox acp` serves the same system to editors over the Agent Client Protocol, and `contenox run` carries declared work into scripts, CI and unattended execution. An agent is declared in Markdown; contenox compiles the declaration into a schema-validated chain and policy. Files and commands arrive from the client, while MCP servers and OpenAPI services attach existing systems as explicit tools. Every call crosses one policy boundary before it runs. Work that stops for a person checkpoints durably and can resume exactly once after the answer arrives. Native local inference runs through modeld; Ollama, vLLM and hosted providers are optional backends. State uses SQLite by default or operator-run PostgreSQL, NATS and Valkey for a server-backed deployment. Open source under Apache-2.0, with no Contenox account required.';

// The shapes contenox runs in, partitioned by who is accountable for the
// machine. Stated right after the summary in both files.
export const PRODUCT_SHAPES =
  'One execution system has three interfaces. `contenox beam` is the first-party terminal for a person at their own keyboard. `contenox acp` connects the same declarations, tools and policies to Zed, JetBrains, AionUi, OpenClaw and other ACP clients over stdio. `contenox run [agent] "task"` is the programmatic interface for CI, cron and other automation: the report goes to stdout and the exit status reports whether the work landed.';
