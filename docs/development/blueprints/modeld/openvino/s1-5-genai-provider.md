# OpenVINO S1.5 GenAI Provider Log

Date: 2026-06-15

This log tracks the step after S1 when benchmarks are intentionally skipped:
turn the S1 GenAI proof into a minimal runtime surface and wire non-streaming
OpenVINO prompt/chat through `modelrepo.Provider`.

## Target

Build the smallest useful embedded GenAI path:

- a Go/CGo `ovsession` API for `ContinuousBatchingPipeline`;
- a real close path, not the S1 proof-only leaked pipeline;
- non-streaming `Prompt`;
- non-streaming `Chat`;
- provider capabilities that advertise prompt/chat only when the GenAI build tag
  is present;
- focused tests and make target support.

## Step Log

### 1. Read The Existing Provider Shape

Files checked:

- `runtime/modelrepo/modelprovidertypes.go`
- `runtime/modelrepo/modelproviderarguments.go`
- `runtime/modelrepo/openvino/provider.go`
- `runtime/modelrepo/openvino/catalog.go`
- `runtime/modelrepo/llama/client.go`

Findings:

- The first useful provider surface is `LLMPromptExecClient.Prompt` and
  `LLMChatClient.Chat`.
- `LLMStreamClient` and `LLMEmbedClient` can stay unsupported for this step.
- The OpenVINO provider currently returns `not wired yet` for every connection.
- The llama provider has a simple prompt builder pattern that can be mirrored
  for OpenVINO.

### 2. Implementation Plan

The S1 proof showed `ContinuousBatchingPipeline` construction, generation, and
metrics work from CGo, but destroying the pipeline inside the CGo call corrupted
the Go test process. S1 kept the pipeline alive for process lifetime because it
was only a control-surface proof.

For S1.5, the GenAI session will run create/generate/destroy work on a native
C++ worker thread. The hypothesis is that GenAI teardown is unsafe on the
Go-managed CGo caller thread but safe on a normal native thread, matching the
standalone C++ probe behavior.

### 3. Added Worker-Thread GenAI Session ABI

Files added under `runtime/modelrepo/openvino/ovsession/`:

- `genai.h`
- `genai.cpp`
- `genai.go`
- `genai_stub.go`
- `genai_test.go`

Build tags:

```go
//go:build openvino && openvino_genai
```

The native ABI exposes:

- `cx_genai_session_new`
- `cx_genai_generate`
- `cx_genai_session_free`

The Go API exposes:

- `ovsession.GenAIAvailable`
- `ovsession.NewGenAI`
- `(*GenAISession).Generate`
- `(*GenAISession).Close`

The native session owns one `ContinuousBatchingPipeline` and serializes all work
through one C++ worker thread. `Close` resets the pipeline on that worker thread
and joins the worker.

Verification:

```sh
go test -tags 'openvino openvino_genai' \
  -run TestSystem_OpenVINOGenAI_SessionGenerateAndClose \
  -v ./runtime/modelrepo/openvino/ovsession
```

Result:

```text
--- PASS: TestSystem_OpenVINOGenAI_SessionGenerateAndClose (1.89s)
PASS
```

This validates that create, generate, metrics, and close work from the Go/CGo
process without the S1 proof-only pipeline leak.

### 4. Wired Provider Prompt, Chat, And Stream

Files changed or added:

- `runtime/modelrepo/openvino/client.go`
- `runtime/modelrepo/openvino/provider.go`
- `runtime/modelrepo/openvino/catalog.go`
- `runtime/modelrepo/openvino/provider_test.go`
- `runtime/modelrepo/openvino/catalog_test.go`
- `runtime/modelrepo/openvino/provider_genai_test.go`

Provider behavior after this step:

- `CanChat()` is true only when `ovsession.GenAIAvailable` is true.
- `CanPrompt()` is true only when `ovsession.GenAIAvailable` is true.
- `CanStream()` is true only when `ovsession.GenAIAvailable` is true.
- `CanEmbed()` remains false.
- `GetChatConnection()` creates a GenAI session for `<modelDir>/<modelName>`.
- `GetPromptConnection()` creates a GenAI session for `<modelDir>/<modelName>`.
- `GetStreamConnection()` creates a GenAI session for `<modelDir>/<modelName>`.
- Embed still returns a not-wired error.

The client builds a small ChatML-style prompt, calls
`(*ovsession.GenAISession).Generate` or `Stream`, and returns assistant text.
Tool calls are explicitly rejected for now instead of being silently ignored.

Device selection:

1. `CONTENOX_OPENVINO_DEVICE`
2. profile `device`
3. `CONTENOX_OPENVINO_TEST_DEVICE`
4. `CPU`

### 5. Added Repeatable S1.5 Verification

`Makefile.openvino` now includes:

```sh
make -f Makefile.openvino test-s1-5
```

The target installs GenAI dependencies, fetches matching `2026.2.0.0` headers,
sets `OPENVINO_TOKENIZERS_PATH_GENAI`, and runs:

```sh
go test -tags 'openvino openvino_genai' -v ./runtime/modelrepo/openvino/...
```

Focused verification run:

```text
--- PASS: TestSystem_OpenVINOProvider_GenAIChatAndPrompt (3.37s)
--- PASS: TestSystem_OpenVINOGenAI_SessionGenerateAndClose (1.92s)
```

Default-build verification also passed:

```sh
go test ./runtime/modelrepo/openvino/... ./runtime/runtimestate
make test-unit
```

Result:

```text
ok  	github.com/contenox/runtime/runtime/modelrepo/openvino
ok  	github.com/contenox/runtime/runtime/runtimestate
make test-unit passed
```

Final compatibility verification:

```sh
go test -tags openvino \
  -run 'Test(System_OpenVINOSession|Unit_OpenVINO)' \
  -v ./runtime/modelrepo/openvino/...
```

Result:

```text
--- PASS: TestSystem_OpenVINOSession_SnapshotRoundTripFreshSession (4.44s)
PASS
```

Full OpenVINO + GenAI package verification:

```sh
go test -tags 'openvino openvino_genai' -v ./runtime/modelrepo/openvino/...
```

Result:

```text
--- PASS: TestSystem_OpenVINOProvider_GenAIChatAndPrompt (4.92s)
--- PASS: TestSystem_OpenVINOGenAI_SessionGenerateAndClose (2.44s)
--- PASS: TestSystem_OpenVINOGenAI_SchedulerControlsReachable (2.18s)
--- PASS: TestSystem_OpenVINOSession_SnapshotRoundTripFreshSession (2.83s)
PASS
```

## Current S1.5 Status

Done:

- close-safe GenAI session ABI;
- prompt generation through the OpenVINO provider;
- non-streaming chat generation through the OpenVINO provider;
- streaming chat generation through the OpenVINO provider, backed by GenAI's
  native streamer callback and a C++ queue read by Go;
- catalog/provider capabilities that stay honest across build tags;
- strict `contenox-openvino.json` model profile loading with profile-driven
  scheduler/session config;
- native cancellation hook using GenAI `StreamingStatus::CANCEL`, with Go
  context cancellation mapped back to `context.Canceled` / deadline errors;
- shared GenAI session pooling across prompt/chat/stream provider connections;
- profile-gated embeddings over modeld transport via OpenVINO GenAI
  `TextEmbeddingPipeline` (`can_embed` in `contenox-openvino.json`);
- repeatable make target.

Still not done:

- C++ native support for Python-only parser objects such as `VLLMParserWrapper`
  remains unresolved; native parser protocols are wired through S2.7;
- per-model shipped `contenox-openvino.json` profiles still need to declare the
  correct tool/reasoning protocols and embedding/chat capabilities.

Current broader follow-up after S2/S2.5/S2.7: move the segment assembler into
shared `contextasm`, add token/profile-stable cache manifests, implement semantic
cache admission/eviction, and run the common local-node benchmark report from
`local-coding-node-goals.md`.

### 6. Hardened Profiles, Cancellation, Streaming, And Pooling

Files added or changed:

- `runtime/modelrepo/openvino/profile.go`
- `runtime/modelrepo/openvino/profile_test.go`
- `runtime/modelrepo/openvino/session_pool.go`
- `runtime/modelrepo/openvino/client.go`
- `runtime/modelrepo/openvino/provider.go`
- `runtime/modelrepo/openvino/catalog.go`
- `runtime/modelrepo/openvino/ovsession/genai.h`
- `runtime/modelrepo/openvino/ovsession/genai.cpp`
- `runtime/modelrepo/openvino/ovsession/genai.go`
- `runtime/modelrepo/openvino/ovsession/genai_stub.go`
- `runtime/modelrepo/openvino/ovsession/genai_test.go`
- `runtime/modelrepo/openvino/provider_genai_test.go`

Profile format:

```json
{
  "context_length": 65536,
  "max_output_tokens": 512,
  "can_chat": true,
  "can_prompt": true,
  "can_stream": true,
  "can_embed": false,
  "can_think": true,
  "device": "CPU",
  "genai": {
    "kv_cache_precision": "f16",
    "cache_size": 1,
    "dynamic_split_fuse": true,
    "enable_prefix_caching": true,
    "use_sparse_attention": true,
    "num_last_dense_tokens_in_prefill": 10,
    "xattention_threshold": 0.9,
    "xattention_block_size": 128,
    "xattention_stride": 16
  }
}
```

The decoder uses `DisallowUnknownFields`; misspelled fields fail instead of
being ignored.

Cancellation:

- `GenAISession.Generate` checks `ctx.Err()` before entering native code.
- While native generation is running, Go starts a cancellation watcher that calls
  `cx_genai_session_cancel`.
- The C++ GenAI streamer checks an atomic cancel flag and returns
  `StreamingStatus::CANCEL`.
- Go maps native cancel return code `3` back to the context error when present.

Streaming:

- `cx_genai_generate_stream` passes a GenAI string streamer callback.
- The callback pushes decoded chunks into `cx_genai_stream`.
- Go drains that native queue and emits `modelrepo.StreamParcel` values.
- Stream cancellation uses the same native cancel flag.

Pooling:

- Provider prompt/chat/stream clients acquire a pooled GenAI session by
  `modelPath + device + scheduler config`.
- Client `Close()` releases the reference.
- Idle sessions remain warm in the pool for reuse.
- Tests have explicit `closeGenAISessionPoolForTest()` cleanup.

Final verification after this hardening pass:

```sh
go test ./runtime/modelrepo/openvino/... ./runtime/runtimestate
go test -tags 'openvino openvino_genai' -v ./runtime/modelrepo/openvino/...
```

Result:

```text
ok  	github.com/contenox/runtime/runtime/modelrepo/openvino
ok  	github.com/contenox/runtime/runtime/runtimestate
ok  	github.com/contenox/runtime/runtime/modelrepo/openvino	3.742s
ok  	github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession	9.667s
```
