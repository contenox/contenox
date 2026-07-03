# Claude Cleanup Notes

Working checklist for the follow-up after Claude session
`1463292c-2342-4bd2-83a5-48fc3bca2998`.

## Baseline From Claude Continuation

- Llama structured output now has real GBNF-backed `json_schema` and
  `llama:json_schema_tool_calls` support.
- The full tagged llama suite passes:
  `go test -tags "llamanode llamacpp_direct" -count=1 ./modeld/llama/...`
- Full `modeld` builds with all backend tags via `make build-modeld`.
- Rebuilt `bin/modeld version` reports both `llama` and `openvino`.

## Cleanup Items

1. OpenVINO runtime-client thinking propagation
   - Status: fixed.
   - Problem: `modeld/openvino` can forward `EnableThinking` and
     `ReasoningEffort`, but `runtime/modelrepo/openvino` does not derive/send
     those fields from `modelrepo.WithThink(...)`.
   - Fix: runtime OpenVINO now derives template thinking controls from
     `WithThink(...)` when the model supports thinking or has a reasoning
     parser, and sends them on `SuffixInput` for chat, prompt, and stream paths.
   - Verification: `go test -count=1 ./runtime/modelrepo/openvino`.

2. OpenVINO Describe chat-template capability truth
   - Status: fixed.
   - Problem: llama `Describe` reports `ChatTemplateFormat`,
     `ChatTemplateSupportsToolCalls`, `ChatTemplateSupportsThinking`, and
     `ChatTemplateSupportsReasoningEffort`; OpenVINO `Describe` leaves those
     fields empty.
   - Fix: modeld/OpenVINO now probes the model's actual OpenVINO GenAI tokenizer
     chat template via `ov::genai::Tokenizer`, using real rendered-output
     checks for tool definitions, `enable_thinking`, and `reasoning_effort`.
     `Describe` copies those probe results into `transport.ModelInfo`.
   - Verification:
     - `go test -count=1 ./modeld/openvino ./runtime/modelrepo/openvino`
     - Native targeted probe:
       `GOFLAGS='-run=TestSystem_OpenVINOGenAI_ProbeChatTemplate -count=1 -timeout=2m' make -f Makefile.openvino test-genai`
     - Native sequence through the prior timeout point:
       `GOFLAGS='-run=TestSystem_(GenAICacheEviction_GeneratesWithNativeEviction|OpenVINOGenAI_(LoRAAdapterGenerates|ApplyChatTemplate|ProbeChatTemplate|ApplyChatTemplateWithTools|SessionGenerateAndClose|ColdKVCapability|TokenPrefillAndGenerate|ShiftedColdKVImport|ContextCanceledBeforeGenerate)) -count=1 -timeout=10m' make -f Makefile.openvino test-genai`
   - Note: one full `make -f Makefile.openvino test-genai` run timed out later
     in `TestSystem_OpenVINOGenAI_ContextCanceledBeforeGenerate` while opening a
     session. That same test passed alone and in the sequence above, so keep it
     as an OpenVINO full-suite stability risk rather than item-2 evidence.

3. Llama fatal-discipline CI visibility
   - Status: fixed.
   - Problem: fatal-state tests are behind `llamanode && llamacpp_direct`. They
     pass in the tagged suite, but plain untagged CI will not catch drift unless
     that tagged lane is required.
   - Fix: extracted the fatal lifecycle state into cgo-free code with plain Go
     unit tests, and changed `make test-llamacpp-direct` so the tagged direct
     lane now runs both `llamacppshim` and `llamasession`.
   - Verification:
     - `go test -count=1 ./modeld/llama/llamasession ./modeld/llama`
     - `make test-llamacpp-direct`

4. TRT-LLM blueprint certainty
   - Status: fixed.
   - Problem: "no supported in-process C++ door to the PyTorch backend" was a
     well-supported inference, not an API guarantee.
   - Fix: `docs/blueprints/modeld-tensorrt-llm-backend.md` now separates
     verified TensorRT-LLM facts from the local modeld integration assessment,
     acknowledges the documented C++ Executor API, and records the PyTorch-native
     C/C++ entrypoint as an open unknown rather than a vendor-stated
     impossibility.
   - Verification: doc review against TensorRT-LLM public docs and NGC release
     metadata on 2026-07-03.

## Original Work Order

Start with item 1. It is a real product-path gap and should be small. Then item
2. Item 3 is a CI/test-policy decision. Item 4 is documentation cleanup.

## Follow-Up Audit Items

5. Structured overflow details over gRPC
   - Status: fixed.
   - Problem: runtime llama/OpenVINO clients recover the live context window by
     parsing `num_ctx=` out of error text because typed overflow fields do not
     survive the gRPC boundary.
   - Fix: added `transport.ContextOverflowError` /
     `ContextOverflowDetail`, carries those details through unary gRPC status
     metadata and decode-stream wire chunks, and switched llama/OpenVINO runtime
     clients to read typed details instead of parsing text.
   - Verification:
     - `go test -count=1 ./runtime/transport/grpc ./runtime/transport`
     - `go test -count=1 ./runtime/modelrepo/llama ./runtime/modelrepo/openvino ./modeld/llama`

6. OpenVINO runtime should consume modeld template-probe facts
   - Status: fixed.
   - Problem: modeld/OpenVINO now reports chat-template probe facts in
     `Describe`, but `runtime/modelrepo/openvino` still gates template thinking
     controls from catalog/profile metadata only.
   - Fix: runtime OpenVINO now uses
     `ModelInfo.ChatTemplateSupportsThinking`,
     `ChatTemplateSupportsReasoningEffort`, and
     `ChatTemplateReasoningFormat` to set runtime client/catalog thinking
     behavior. Tool calls still need a certified parser protocol, so template
     tool rendering alone does not imply tool-call support.
   - Verification: `go test -count=1 ./runtime/modelrepo/openvino`.

7. OpenVINO no-resident decode silently falls back to raw text
   - Status: fixed.
   - Problem: if `Decode` has no resident token tape and
     `ApplyChatTemplate(...)` fails, it silently sends `stable+suffix` as raw
     prompt text.
   - Fix: no-resident decode now returns an explicit error when chat-template
     rendering fails or returns an empty prompt; raw `stable+suffix` is only used
     when there are no chat-message segments to render.
   - Verification: `go test -count=1 ./modeld/openvino`.

8. OpenVINO parsed structured output can accept an empty parsed envelope
   - Status: fixed.
   - Problem: `chunkFromGenAIResult` rejects malformed raw structured tool-call
     JSON through the shared parser, but the GenAI `ParsedJSON` branch can
     return an empty chunk if parsed output contains neither content nor
     tool calls.
   - Fix: `chunkFromGenAIResult` now rejects unsupported structured protocols
     before parsing, and requires content or tool calls for parsed
     `openvino:json_schema_tool_calls` output.
   - Verification: `go test -count=1 ./modeld/openvino`.

9. OpenVINO full native suite timeout
   - Status: not reproduced on full native rerun.
   - Problem: one full `make -f Makefile.openvino test-genai` run timed out
     while opening `TestSystem_OpenVINOGenAI_ContextCanceledBeforeGenerate`,
     even though the same test passed alone and in a targeted sequence.
   - Evidence: a later full `make -f Makefile.openvino test-genai` run passed.
     The longest quiet section was
     `TestSystem_OpenVINOGenAI_PrefixReuseWarmsPrefill`, which completed in
     262.84s; the full `modeld/openvino/ovsession` package passed in 290.143s.
   - Proper next action if it recurs: isolate whether OpenVINO GenAI session
     construction leaks or deadlocks after earlier tests, then make the full
     suite stable instead of relying on targeted-pass evidence.

## Real CLI/Daemon E2E Verification

10. `contenox chat` against OpenVINO modeld
   - Status: passed.
   - Build: `make build-contenox build-modeld`.
   - Startup:
     `HOME=tmp/e2e-chat-20260703-183255/home CONTENOX_MODELD_BACKEND=openvino CONTENOX_OPENVINO_DEVICE=CPU bin/modeld serve --data-root tmp/e2e-chat-20260703-183255/home/.contenox --idle-ttl off`.
   - CLI wiring:
     `contenox backend add openvino --type openvino --url $PWD/.openvino/models`,
     default provider `openvino`, default model `qwen-coder-0.5b-int4`.
   - Live catalog proof:
     `contenox model list` showed `qwen-coder-0.5b-int4` with chat enabled and
     `CTX=32768`.
   - Multi-turn chat proof through separate `contenox chat` invocations:
     remembered `BLUE-17`, recalled `BLUE-17`, then answered `Yes` when asked if
     the previous answer was exactly that token.
   - Session proof: `contenox session show openvino-e2e` showed 6/6 persisted
     messages.

11. `contenox chat` against NVIDIA/llama modeld
   - Status: passed.
   - Build evidence: `make build-contenox build-modeld` reused the pinned
     llama.cpp runtime with `cuda=ON` and `libggml-cuda.so`.
   - Host accelerator: `nvidia-smi` reported `NVIDIA GeForce GTX 1660` with
     6144 MiB.
   - Startup:
     `HOME=tmp/e2e-chat-20260703-183255/home CONTENOX_MODELD_BACKEND=llama CONTENOX_LLAMA_BACKEND_DIR=$PWD/.llamacpp-runtime/local/lib bin/modeld serve --data-root tmp/e2e-chat-20260703-183255/home/.contenox --idle-ttl off`.
   - CLI wiring:
     `contenox backend add llama --type llama --url /home/naro/.contenox/models/llama`,
     default provider `llama`, default model `qwen3-4b`.
   - Live catalog proof:
     `contenox model list` showed `qwen3-4b` with chat/prompt/embed enabled,
     thinking enabled, and `CTX=19544`.
   - CUDA proof from modeld logs: llama.cpp found CUDA0
     `NVIDIA GeForce GTX 1660`, loaded `libggml-cuda.so`, used device CUDA0,
     offloaded `37/37` layers, and placed KV cache layers on CUDA0.
   - Multi-turn chat proof through separate `contenox chat` invocations:
     remembered `GREEN-42`, recalled `GREEN-42`, then answered `yes` when asked
     if the previous answer was exactly that token.
   - Session proof: `contenox session show llama-e2e` showed 6/6 persisted
     messages.

12. Interface note from E2E setup
   - Status: documented.
   - Finding: for isolated local-modeld E2E runs, set the same temporary `HOME`
     for both `modeld` and `contenox`. `--data-dir` controls the CLI's resolved
     `.contenox` workspace path, while default local modeld lease discovery
     follows the process home unless tests call `modeldconn.SetDataRoot`
     directly.
