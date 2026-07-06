# Plan: Cloud Provider Context Caching

> **Status:** blueprint. This tracks the strategy to map Contenox's backend-agnostic `AssembleContext` architecture to the native Prompt/Context Caching APIs of cloud providers (Anthropic, Gemini/Vertex, OpenAI).

---

## The Goal

Contenox already uses a deterministic context assembler (`AssembleContext`) to separate stable workspace state (system prompts, tool schemas, repo maps, pinned files) from volatile state (chat messages, diffs). For local nodes (`llama.cpp`, `OpenVINO`), this drives low-level KV cache reuse and snapshotting.

The goal of this track is to pipe that exact same semantic intelligence into **cloud provider APIs**. By mapping our deterministic segments to vendor-specific caching mechanisms, we achieve:
1. **Massive latency reduction (TTFT)** for large workspace contexts when using hosted models.
2. **Drastic cost savings**, as providers charge significantly less (often 50-90% off) for cached prefix tokens.

## The Workspace-Context Advantage

Cloud providers require exact byte-prefixes or specific API markers to trigger cache hits. Because Contenox already calculates strict segment hashes, tracks invalidation rules, and isolates volatile text for the local node, **we do not need to build new state-tracking logic for the cloud**. We simply need to translate the existing `AssembleContext` manifest into the shape each cloud provider expects.

## Provider Strategies

### 1. Anthropic (Breakpoint Caching)
Anthropic uses an explicit breakpoint system. You can tag specific blocks in a request with `{"cache_control": {"type": "ephemeral"}}`.
* **Mechanism:** The Contenox Anthropic provider adapter will read the `AssembleContext` manifest. It will locate the final stable segment (e.g., the end of the repo map or pinned files) right before the volatile suffix begins, and inject the `cache_control` tag at that exact boundary.
* **Requirement:** We must map our logical segments strictly to Anthropic's `content` array blocks, preserving the exact JSON structure.

### 2. Google Gemini / Vertex AI (Explicit Caching API)
Google uses an out-of-band Context Caching API. You upload a large blob of text/context (minimum 32k tokens), receive a `cacheName`/ID, and reference it in subsequent generate calls.
* **Mechanism:** The Contenox Gemini/Vertex adapter will track the hash of the combined stable segments. If the hash changes (e.g., a pinned file is edited), it registers a new cache object via the API (with a reasonable TTL, e.g., 60 minutes). During the coding loop, it only sends the volatile suffix along with the `cacheName`.
* **Requirement:** Lightweight state management in the provider adapter to track active cloud cache IDs against the local `AssembleContext` stable hash.

### 3. OpenAI (Implicit Caching)
OpenAI automatically caches prefixes longer than 1,024 tokens. If the exact byte-prefix matches a recent request, it is treated as a cache hit on their backend.
* **Mechanism:** No new API fields are required. However, the adapter must guarantee absolute determinism. Any jitter in JSON serialization, variable whitespace, or randomized tool schema ordering will break the implicit byte-prefix match.
* **Requirement:** Strict adherence to the `AssembleContext` output without any dynamic mid-stream alterations before dispatching the HTTP request.

## Architecture Boundary

The `contextasm` (or equivalent shared assembler package) remains completely unchanged. It continues to output a strict manifest:

```go
type ContextManifest struct {
    StableHash string
    Segments   []Segment
    // ...
}
```

The changes live entirely within the cloud provider adapters (e.g., `runtime/providerservice/anthropic`, `openai`, `gemini`). 
* The interface to providers must accept the structured segment manifest, rather than just a flattened `[]Message` array or single prompt string.
* Providers that support explicit caching translate the manifest; providers that don't just flatten it as usual.

## Metrics and Verification

Every cloud invocation should track and report in the CLI/telemetry:
* `cache_creation_ms` (for Gemini/Vertex out-of-band uploads)
* `cached_tokens` vs `fresh_tokens`
* `TTFT` (Time To First Token)

**Success Gate:** A Contenox chain using `claude-3-5-sonnet` or `gemini-1.5-pro` with a 100k-token codebase context achieves >90% cache hit rates across a 10-turn coding session, automatically driven by the same rules that power the local hardware node.
