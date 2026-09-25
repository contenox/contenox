---
title: Native modeld
description: Run inference with the native worker through the existing Contenox harness and gateway.
---

# Native modeld

Contenox can use modeld for local inference. The worker owns model loading,
hardware allocation and resident sessions; the harness and gateway use its
session transport. The CLI does not link native inference libraries.

## Local setup

For automatic setup:

```sh
contenox auto --dry-run
contenox auto
```

`auto` detects available memory, selects a curated permissive model, installs the
native worker and weights, verifies a model-native tool call, and opens the TUI.
It prefers approximately 128K–280K resident context over larger weights. Memory
for weights, runtime overhead and the KV cache is budgeted separately; cold or
planner context does not count. The opened worker session reports the final hot
capacity. A lower capacity is reported when the target cannot be reached.

An optional origin restricts the model developer: `contenox auto us`,
`contenox auto eu`, `contenox auto china` or `contenox auto gus`. Downloads still
come from Hugging Face; origin does not select a hosting region. An origin with
no compatible model for the backend and budget fails without changing defaults.

Subsequent runs reuse the configured eligible model. Use `--refresh` to select
again or `--no-tui` to finish setup without opening the terminal interface.
`--dry-run` prints hardware facts and an estimated hot capacity without
downloading or changing configuration. A running worker keeps its backend;
otherwise detection chooses the backend, with `CONTENOX_MODELD_BACKEND` taking
precedence. Detection never adds shared RAM to dedicated VRAM. To explicitly
use CPU memory, set `CONTENOX_LLAMA_GPU_LAYERS=0` for llama or
`CONTENOX_OPENVINO_DEVICE=CPU` for OpenVINO. CPU execution does not promise GPU
speed or high-bandwidth memory.

Successful setup saves the native primary and fallback model and a context budget
bounded by the opened session. It opens a fresh TUI session with those defaults.
Existing declarations and chains retain their own explicit settings, including
smaller context limits. Shell environment overrides still apply to later runs.
See [selection and compatibility](/docs/development/auto-models/)
for the estimator and the limits of current native backends.

Downloads accept optional `HF_TOKEN` authentication and an `HF_ENDPOINT` mirror.
The token is sent only to the configured Hugging Face origin, never a different
redirect destination. Automatic selection uses ungated artifacts; manually
selected gated models require accepting the publisher's terms on Hugging Face
and granting the token access. Contenox does not accept licenses for you.

## Warm-state persistence

modeld keeps compatible inference state resident between calls. Before idle
unloading, explicit unloading, a model switch, or eviction for an embedding
request, it attempts to save the native session. Graceful daemon shutdown also
captures the resident state before releasing ownership. The next compatible
open restores it locally on the daemon, including any captured cold KV blocks.

Automatic snapshots live under
`<worker-data-root>/modeld-snapshots/<backend>/slot-v1`. They contain native state
and prompt metadata and must be treated as sensitive data. Files are private to
the daemon user. The cache retains at most 4 GiB per backend and expires entries
after 24 hours without use; cleanup runs during cache operations. Atomic writes
can temporarily require space for both an old entry and its replacement.

Set `CONTENOX_WARM_SNAPSHOT_DISABLE=1` in the daemon environment to disable
automatic snapshots, or `CONTENOX_WARM_SNAPSHOT_DIR` to relocate their root.
Existing legacy snapshot/reference files are not migrated or deleted. Automatic
snapshot files contain their native bytes rather than references to the manual
snapshot RPC's separate blob directory.

Restoration requires matching model assets, template/projector, adapters,
daemon executable, backend runtime identity and context configuration. Asset
hashes are computed on first use in a daemon process and reused while file size
and modification time are unchanged. This adds disk reads on the first open.
Missing, expired, corrupt or rejected snapshots cause a cold open. A rejected
native restore is closed and replaced with a fresh session before serving work.
Backends that cannot snapshot continue without persistence.

Automatic context sizing can choose a different physical window when available
memory changes, invalidating an otherwise compatible snapshot. For a fixed
llama window, set `CONTENOX_LLAMA_CTX` in the worker environment. Restoration
does not guarantee prefix reuse: recurrent models must recompute when the next
request requires trimming a restored tail, and some chat templates cannot
render a standalone system prefix.

Snapshots accelerate reconstruction; conversation history remains authoritative.
This is model/configuration-keyed reuse, not independent per-conversation
checkpoints, cross-node migration, or guaranteed recovery of the latest turn
after an abrupt crash. Parked cold KV does not participate in attention until
admitted to the active context. Snapshot telemetry distinguishes capture,
restoration, cold opens and failures.

## Manual setup

Install a compatible worker, choose a model from the registry, and configure it:

```sh
contenox modeld install
contenox model registry-list
contenox model pull qwen2.5-coder-7b
contenox setup
```

Choose modeld in setup. Contenox starts the local worker when needed. Use
`contenox modeld status` to inspect the worker and `contenox model list` to
inspect the models discovered through it. Registry entries are downloadable
models; they are not a claim that every entry fits your machine.

## Inspect and manage local models

```sh
contenox pull qwen2.5-coder-7b
contenox list
contenox show qwen2.5-coder-7b
contenox ps
contenox stop qwen2.5-coder-7b
```

`pull` is shorthand for `model pull`. `list` (also `ls`) reads the local model
store without starting a worker; `model list --local` gives the same inventory.
`model list` retains its live view across registered backends.

`show` asks the local worker for model metadata and effective hardware capacity,
starting it if necessary. It does not download missing weights. `ps` reports the
resident model without starting a worker. `stop` unloads the named model while
leaving the daemon running; a busy model or a changed slot is refused.
These commands address the local data root, not remote registered backends.

`show` and `ps` also work under `model`. `list`, `show` and `ps` accept `--json`.
Offline context metadata is the model ceiling when available, not a hardware
allocation; zero means unknown.

Installation requires a compatible artifact in the release index. For a source
checkout or your own release store, follow [the native build guide](/docs/development/modeld-build/).
`CONTENOX_MODELD_BIN` selects a worker executable;
`CONTENOX_DATA_ROOT` selects its model and lease directory, defaulting to
`~/.contenox`. These are separate from the workspace directory.

To configure without the wizard:

```sh
contenox backend add native --type modeld --url local
contenox config set inference.provider modeld
contenox config set inference.model qwen2.5-coder-7b
contenox beam
```

## Worker capacity and usage

The worker reports the context capacity it can serve for each model. Contenox
uses that report for routing; adding a larger context value to a stored model
record does not enlarge the worker's allocation.

Requests use the same gateway usage ledger as other providers. Usage comes from
the native backend when available. Missing token counts are not reconstructed
from text length, and a backend that does not report a separate thinking count
cannot provide that breakdown.

## Remote workers

The modeld backend also accepts a worker endpoint instead of `local`. Each worker
is registered through `contenox backend add`; use its help for the current flags.
The existing gRPC transport has no authentication or encryption. Its default
listener is loopback; remote workers require a trusted private network.
