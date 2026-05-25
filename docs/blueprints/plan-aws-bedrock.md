  Easy parts:

  - Auth: AWS SDK Go v2 credential chain (env vars, profile, IAM role, instance metadata, AssumeRole) is mature — drop-in similar to Vertex's ADC
  but without the refresh-token nightmare. Credentials renew automatically through the SDK.
  - The Converse API (bedrockruntime.Converse / ConverseStream) normalizes across Anthropic, Meta, Mistral, Cohere, AI21, Titan. One
  request/response shape for almost all models — far less translation than per-model formats.
  - Streaming events are a typed union; switch on event type, done.

  Friction parts:

  - Some legacy Bedrock models are InvokeModel-only, not Converse. Pick the Converse subset and skip the rest.
  - Tool-use protocol differs from OpenAI's: function definitions go in ToolConfig, tool calls come back as ToolUse content blocks. Mapping to
  contenox's internal tool-call format is straightforward but real code.
  - Per-region per-model availability — Bedrock model IDs differ by region. The catalog endpoint (bedrock.ListFoundationModels) tells you what's
  actually accessible per region.
  - Account-level model enablement is the canonical UX trap. AWS requires explicit enablement of each model in the Bedrock console before any API
  call works. New backends will return empty model lists until customers do that. UX needs clear handling for AccessDeniedException: model not
  enabled — otherwise it looks like the backend is broken.

  Maps onto internal/modelrepo/bedrock/ cleanly — mirror the vertex package structure (auth, catalog, chat, stream, types, tests). Probably
  2500-4000 LOC including tests. Backend types: either a single bedrock since Converse unifies them, or bedrock-anthropic / bedrock-meta / etc.
  matching the Vertex family-split pattern.
