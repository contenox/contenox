# OpenVINO S2 Prefix-Reuse Proof Log

Date: 2026-06-15

Records S2 from `plan-openvino.md`: prove that OpenVINO
`ContinuousBatchingPipeline` prefix caching actually delivers warm reuse of a
repeated stable prefix — the load-bearing assumption under the Contenox
workspace-context reuse layer.

## S2 Target

Confirm that a large, stable prefix sent twice through one pooled GenAI session
is materially cheaper on the second (warm) call because the cached KV of the
shared prefix is reused — not re-prefilled.

## What Was Added

- `runtime/modelrepo/openvino/ovsession/s2_prefix_reuse_test.go`
  (`//go:build openvino && openvino_genai`)
- `Makefile.openvino` target: `make -f Makefile.openvino test-s2`

The test:

- creates one `GenAISession` (`enable_prefix_caching` defaults true);
- builds a ~166 KB stable "repo context" prefix (2000 synthetic source lines,
  ~47k tokens);
- warms the runtime with a short unrelated prompt so one-time init is excluded;
- cold call: `prefix + suffix A`, `max_new_tokens=1`;
- warm call: `prefix + suffix B`, `max_new_tokens=1`;
- asserts `warm < 0.8 * cold` and logs both timings + cache metrics.

## Result (CPU, Qwen2.5-Coder-0.5B-int4)

```text
prefix bytes = 166000
cold (full prefill + 1 tok) = 2m14.46s   cache_usage=37.51%  cache_size_in_bytes=1073479680
warm (cached prefix + 1 tok) = 664.17ms  cache_usage=37.51%
warm/cold = 0.005   (speedup 99.5%)
--- PASS: TestSystem_OpenVINOGenAI_PrefixReuseWarmsPrefill (136.70s)
```

Run with:

```sh
make -f Makefile.openvino test-s2
```

## Finding (two-faced)

1. **The reuse substrate works — spectacularly.** A repeated stable prefix is
   reused almost entirely: 2m14s cold prefill collapses to 664ms warm. The
   warm-workspace thesis holds on this stack with OpenVINO's automatic CB prefix
   caching; no explicit S0 snapshot/restore is required for *same-session* prefix
   reuse.

2. **Cold prefill on CPU is the real bottleneck.** ~47k tokens took 134s ≈ ~350
   prompt tokens/sec. That blows past the "first useful response < 1–2 min"
   target for large cold context. Consequences:
   - warm reuse is **mandatory**, not an optimization — a cold big-context
     prefill is unusable on CPU;
   - first load of a workspace, any cache miss, eviction, or change to a stable
     segment pays this penalty;
   - the budget Intel node target (Arc / Arc Pro dGPU) is needed to make cold and
     near-cold paths tolerable; CPU alone is ~3× too slow for the raw target.
     This is exactly what S3 (hardware benchmark) must quantify.

## Implications For The Workspace-Context Design

- The Contenox session layer must maximize warm-prefix hit rate: stable segments
  (system / tools / repo rules / repo map / pinned files) must be byte-identical
  and ordered first so the CB prefix cache reuses them every turn.
- Segment changes must be cheap: re-prefill only the changed suffix, never the
  whole context. Incremental, segment-aware assembly is the product value layered
  on top of the substrate.
- Cache sizing/eviction matters: `cache_usage` hit 37.5% of 1 GB for a single
  ~47k-token prefix; multiple workspaces/sessions need a real cache budget and
  eviction policy.
- Live prefix reuse is the hot path. S0 snapshot/restore remains valuable for
  suspend/resume, branch, crash recovery, and reproducible benchmark fixtures,
  but it should not be treated as the normal latency path until S3 proves
  snapshot save/restore is faster than keeping the prefix hot.
- A stable-prefix hash is necessary but not sufficient. Product cache hits also
  need model digest, tokenizer digest, chat template digest, backend/runtime
  version, context/RoPE settings, KV precision, token hash, token position, and
  cache-block alignment compatibility.

## Cache Policy Direction

The session layer should apply a semantic admission/eviction policy instead of
plain recency:

```text
highest: system/developer prompt, tool schemas, repo instructions
high:    repo map, pinned files, active task summary
medium:  current diff, recent failing test output
low:     stale terminal logs, old user turns, exploratory snippets
```

This is product knowledge the OpenVINO scheduler cannot infer from anonymous
tokens.

## S2.5: The Assembler Drives The Cache

First brick of the workspace-context layer on top of the proven substrate.

Added:

- `runtime/modelrepo/openvino/segments.go` — `AssembleContext(segs)` orders
  segments canonically by `SegmentKind` (stable kinds first, volatile last),
  renders a self-delimiting prompt, and returns a sha256 of the **stable prefix
  only**. Pure Go, no OpenVINO imports, in the default build (CI-tested).
- `runtime/modelrepo/openvino/segments_test.go` — unit tests: input-order
  independence, stable-before-volatile, volatile change keeps the stable-prefix
  hash, stable edit changes it. Run in the default `go test` (no tags).
- `runtime/modelrepo/openvino/segments_integration_test.go` +
  `make -f Makefile.openvino test-s2-5` — end-to-end proof through a real GenAI
  session.

End-to-end result (CPU, ~9k-token stable repo-map prefix):

```text
turn1 cold (new stable prefix)       = 7.61s   hash=360be0ef2574
turn2 warm (same stable prefix)      = 73ms    hash=360be0ef2574   (104x faster)
turn3 stable edited (prefix changed) = 7.51s   hash=311f9a59bf86   (re-prefill, back to cold)
PASS (16.7s)
```

**The stable-prefix hash is a reliable predictor of a cache hit.** Equal hash
across turns => the prefix cache reuses the KV (73ms). A changed hash (a stable
segment was edited) => the tail re-prefills (7.5s, ~cold). This is the exact
signal a context planner needs to decide reuse-vs-rebuild.

## Still Open

- S3 hardware benchmark on the target Intel dGPU (cold + warm prefill tok/s).
- Provider-level exposure of cache metrics so context policy can act on real
  hit-rate signal.
- Wire `AssembleContext` into the provider chat path (today the chain layer must
  supply structured segments; the provider still receives flat `[]Message`).
- Cache budget + eviction policy across multiple workspaces/sessions.
- Move `AssembleContext` into shared `contextasm`, add manifest generation,
  token hashes, token ranges, profile compatibility checks, and
  `explain-context`.
- Add correctness gates: warm suffix output must equal cold full prompt output
  under deterministic decoding; edited stable segments and profile/tokenizer/
  template drift must force misses.
