# OpenVINO S1 Embedded Controls Log

Date: 2026-06-15

This is the step-by-step record for S1 from `plan-openvino.md`.

## S1 Target

Confirm that `ContinuousBatchingPipeline` / `SchedulerConfig` controls are
reachable from the Contenox process.

Minimum proof:

- `enable_prefix_caching`
- KV precision
- cache size
- split-fuse / chunked prefill
- sparse attention controls
- perf metrics

## Outcome

S1 is proven from Go/CGo with OpenVINO GenAI `2026.2.0.0`.

The probe:

- constructs `ov::genai::ContinuousBatchingPipeline` from a Go test through CGo;
- applies `SchedulerConfig` values for prefix caching, cache size, dynamic
  split-fuse, and XAttention sparse attention;
- passes `KV_CACHE_PRECISION=f16`;
- generates one token;
- reads `PipelineMetrics`;
- verifies the report in a Go test.

Important caveat: the S1 probe keeps the GenAI pipeline alive for process
lifetime. Destroying `ContinuousBatchingPipeline` inside the CGo call corrupts
the Go test process after the report is returned. S1 does not solve production
lifecycle teardown yet; that is a follow-up before real provider wiring.

## 1. Confirmed Package Versions

The S1 run uses the existing S0 virtualenv under:

```text
/home/naro/src/github.com/contenox/ov-s0/.venv
```

Installed or confirmed:

```sh
/home/naro/src/github.com/contenox/ov-s0/.venv/bin/pip install -q openvino-genai
/home/naro/src/github.com/contenox/ov-s0/.venv/bin/python - <<'PY'
import importlib.metadata as md
for pkg in ['openvino', 'openvino-genai', 'openvino-tokenizers']:
    print(pkg, md.version(pkg))
PY
```

Result:

```text
openvino 2026.2.0
openvino-genai 2026.2.0.0
openvino-tokenizers 2026.2.0.0
```

## 2. Located Runtime Libraries

The Python packages provide the shared libraries:

```text
openvino:
  /home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino

openvino_genai:
  /home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino_genai

openvino_tokenizers:
  /home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino_tokenizers
```

The GenAI wheel does not ship the public C++ headers needed for embedding, so
the headers must come from the matching source tag.

## 3. Matched GenAI Headers To The Wheel

The local source checkout was:

```text
/home/naro/src/github.com/openvinotoolkit/openvino.genai
```

It was on `master`, which was not safe to use against the `2026.2.0.0` wheel.
Fetching tags showed the matching release exists:

```sh
git -C /home/naro/src/github.com/openvinotoolkit/openvino.genai fetch --tags --force
git -C /home/naro/src/github.com/openvinotoolkit/openvino.genai show -s --format='%H %D %ci' 2026.2.0.0
```

Result:

```text
adf73e80e66629730f976d44cad6c09cf978deca tag: 2026.2.0.0 2026-05-18 14:08:21 +0000
```

A detached worktree was created so the existing source checkout stays on
`master`:

```sh
git -C /home/naro/src/github.com/openvinotoolkit/openvino.genai \
  worktree add --detach \
  /home/naro/src/github.com/openvinotoolkit/openvino.genai-2026.2.0.0 \
  2026.2.0.0
```

## 4. Inspected The Public C++ Control Surface

The release headers expose the S1 controls under:

```text
/home/naro/src/github.com/openvinotoolkit/openvino.genai-2026.2.0.0/src/cpp/include/openvino/genai
```

Confirmed headers:

- `continuous_batching_pipeline.hpp`
- `scheduler_config.hpp`
- `sparse_attention.hpp`
- `perf_metrics.hpp`

Confirmed `SchedulerConfig` fields:

- `cache_size`
- `num_kv_blocks`
- `dynamic_split_fuse`
- `enable_prefix_caching`
- `use_sparse_attention`
- `sparse_attention_config`

Confirmed `SparseAttentionConfig` / XAttention controls:

- `SparseAttentionMode::XATTENTION`
- `num_last_dense_tokens_in_prefill`
- `xattention_threshold`
- `xattention_block_size`
- `xattention_stride`

Confirmed `ContinuousBatchingPipeline::get_metrics()` returns pipeline metrics
including cache usage and cache size.

## 5. Proved The Controls Through Python First

A Python probe in the S0 virtualenv constructed both high-level and continuous
batching GenAI pipelines with:

```python
cfg.cache_size = 1
cfg.dynamic_split_fuse = True
cfg.enable_prefix_caching = True
cfg.use_sparse_attention = True
cfg.sparse_attention_config.mode = openvino_genai.SparseAttentionMode.XATTENTION
cfg.sparse_attention_config.num_last_dense_tokens_in_prefill = 10
cfg.sparse_attention_config.xattention_threshold = 0.9
cfg.sparse_attention_config.xattention_block_size = 128
cfg.sparse_attention_config.xattention_stride = 16
```

The pipeline was constructed with:

```python
{"scheduler_config": cfg, "KV_CACHE_PRECISION": "f16"}
```

Result:

- OpenVINO logged the scheduler config with prefix caching, split-fuse, and
  XAttention enabled.
- OpenVINO logged `KV_CACHE_PRECISION: f16`.
- `ContinuousBatchingPipeline.get_metrics()` returned `PipelineMetrics`.
- `generate(... max_new_tokens=1 ...)` returned one result.

This proved the OpenVINO GenAI runtime accepts the controls, but not yet that
Contenox can reach them through Go/CGo.

## 6. Found The Tokenizer Extension Requirement

The first embedded C++ attempt failed until this environment variable was set:

```sh
OPENVINO_TOKENIZERS_PATH_GENAI=/home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino_tokenizers/lib/libopenvino_tokenizers.so
```

This is required when using the wheel-provided GenAI library from an embedded
C++/CGo process.

## 7. Proved The Same Call Shape In Standalone C++

Before debugging CGo, the same C++ call shape was compiled as a standalone
binary with release-matched headers:

```sh
OV=/home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino
GENAI=/home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino_genai
TOKENIZERS=/home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino_tokenizers/lib
GENAI_SRC=/home/naro/src/github.com/openvinotoolkit/openvino.genai-2026.2.0.0

g++ -g -std=c++17 -I$OV/include -I$GENAI_SRC/src/cpp/include /tmp/s1_genai_probe.cpp \
  -L$OV/libs -L$GENAI -L$TOKENIZERS \
  -l:libopenvino_genai.so.2620 -l:libopenvino.so.2620 -l:libopenvino_tokenizers.so -lstdc++ \
  -Wl,-rpath,$OV/libs -Wl,-rpath,$GENAI -Wl,-rpath,$TOKENIZERS \
  -o /tmp/s1_genai_probe

LD_LIBRARY_PATH="$OV/libs:$GENAI:$TOKENIZERS" \
OPENVINO_TOKENIZERS_PATH_GENAI="$TOKENIZERS/libopenvino_tokenizers.so" \
/tmp/s1_genai_probe
```

Result:

```text
phase config
phase construct
phase generate
PipelineMetrics { cache_usage: 0.03663, max_cache_usage: 0.03663, avg_cache_usage: 0.03663, cache_size_in_bytes: 1073479680 }
GenerationResultCount: 1
```

This proved the C++ API and wheel libraries work outside Go.

## 8. Added The Contenox S1 Probe

Files added under `runtime/modelrepo/openvino/ovsession/`:

- `s1_probe.h`
- `s1_probe.cpp`
- `s1_controls.go`
- `s1_controls_test.go`

Build tags:

```go
//go:build openvino && openvino_genai
```

The C++ probe sets:

```cpp
cfg.cache_size = 1;
cfg.dynamic_split_fuse = true;
cfg.enable_prefix_caching = true;
cfg.use_sparse_attention = true;
cfg.sparse_attention_config.mode = ov::genai::SparseAttentionMode::XATTENTION;
cfg.sparse_attention_config.num_last_dense_tokens_in_prefill = 10;
cfg.sparse_attention_config.xattention_threshold = 0.9f;
cfg.sparse_attention_config.xattention_block_size = 128;
cfg.sparse_attention_config.xattention_stride = 16;
```

It constructs:

```cpp
ov::genai::ContinuousBatchingPipeline(
    std::filesystem::path(model_dir),
    cfg,
    device,
    ov::AnyMap{{"KV_CACHE_PRECISION", ov::element::f16}})
```

Then it generates one token, reads `get_metrics()`, and returns a text report to
Go.

## 9. Debugged The CGo Segfault

Initial CGo runs segfaulted after the C++ probe returned.

`gdb` showed crashes in Go runtime/test logging after the report was already
available, not inside the scheduler setup itself.

Changes tried:

- matching `2026.2.0.0` headers instead of `master` headers;
- C-allocated report/error buffers instead of Go-owned byte slices;
- direct `libopenvino_tokenizers.so` linking with `--no-as-needed`.

Those did not fix teardown.

The passing change was to keep the probe `ContinuousBatchingPipeline` alive for
process lifetime instead of destroying it before returning from CGo. With that
change, Go can read the report and assert the required fields.

This is acceptable for the S1 control-surface proof, but it is not the final
production lifecycle design.

## 10. Final S1 Verification Command

```sh
OV=/home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino
GENAI=/home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino_genai
TOKENIZERS=/home/naro/src/github.com/contenox/ov-s0/.venv/lib/python3.12/site-packages/openvino_tokenizers/lib
GENAI_SRC=/home/naro/src/github.com/openvinotoolkit/openvino.genai-2026.2.0.0
MODEL=/home/naro/src/github.com/contenox/ov-s0/models/qwen-coder-0.5b-int4

CGO_ENABLED=1 \
CGO_CXXFLAGS="-std=c++17 -I$OV/include -I$GENAI_SRC/src/cpp/include" \
CGO_LDFLAGS="-L$OV/libs -L$GENAI -L$TOKENIZERS -l:libopenvino_genai.so.2620 -l:libopenvino.so.2620 -lstdc++ -Wl,-rpath,$OV/libs -Wl,-rpath,$GENAI -Wl,-rpath,$TOKENIZERS" \
LD_LIBRARY_PATH="$OV/libs:$GENAI:$TOKENIZERS" \
OPENVINO_TOKENIZERS_PATH_GENAI="$TOKENIZERS/libopenvino_tokenizers.so" \
CONTENOX_OPENVINO_TEST_MODEL="$MODEL" \
CONTENOX_OPENVINO_TEST_DEVICE=CPU \
go test -tags 'openvino openvino_genai' \
  -run TestSystem_OpenVINOGenAI_SchedulerControlsReachable \
  -v ./runtime/modelrepo/openvino/ovsession
```

Result:

```text
--- PASS: TestSystem_OpenVINOGenAI_SchedulerControlsReachable (1.68s)
PASS
ok  	github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession	1.729s
```

The report contained:

```text
cache_size: 1
dynamic_split_fuse: true
enable_prefix_caching: true
use_sparse_attention: true
sparseAttentionMode: XATTENTION
xattention_threshold: 0.9
xattention_block_size: 128
xattention_stride: 16
PipelineMetrics {
  requests: 1
  scheduled_requests: 1
  cache_usage: 0.03663
  max_cache_usage: 0.03663
  avg_cache_usage: 0.03663
  cache_size_in_bytes: 1073479680
}
GenerationResultCount: 1
```

## 11. Makefile Support

`Makefile.openvino` now has:

```sh
make -f Makefile.openvino test-s1
```

The target:

- installs `openvino-genai`;
- fetches GenAI tags;
- creates the matching `openvino.genai-2026.2.0.0` worktree if needed;
- sets `OPENVINO_TOKENIZERS_PATH_GENAI`;
- runs the `openvino openvino_genai` tagged S1 test.

## 12. What Is Still Not Done

S1 is done as a control-surface proof.

Still open for the broader OpenVINO track:

- Production lifecycle handling for `ContinuousBatchingPipeline` teardown from
  Go/CGo.
- S0 feature map completion across GenAI C API, GenAI C++ API, OVMS-only
  controls, and our required shim surface.
- S3 hardware benchmark on the target Intel device.
- Provider-level chat/stream/prompt client wiring on top of the proven
  primitives.
- The common local-node benchmark report from `local-coding-node-goals.md`,
  including warm suffix equivalence to cold full prompt, suffix-growth TTFT
  curve, snapshot save/restore timing, cancellation recovery, and cache eviction
  behavior.
