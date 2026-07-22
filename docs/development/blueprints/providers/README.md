# Provider Blueprints

Cloud/hosted model providers plug into `runtime/modelrepo` behind the
`modelrepo.Provider` interface; request-side selection happens in
`runtime/llmrepo`. These docs cover provider-specific integration designs.

| Doc | Status | What it covers |
| --- | --- | --- |
| [aws-bedrock.md](aws-bedrock.md) | implemented | Bedrock Converse API provider: dependency assessment, auth chain, codec fit |
