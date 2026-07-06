# llama.cpp Binding Decision: Contenox-Owned Shim

Date: 2026-06-16

Status: final decision record.

## Decision

The llama.cpp blocker is resolved by building a minimal **Contenox-owned
llama.cpp shim** for `llama`.

This means:

```text
yes: own the small CGo/C shim needed by Contenox llama
yes: expose only the llama.cpp primitives llama needs
yes: keep product code behind the backend-neutral llama session API

no: unsafe ABI access to Ollama's private Go struct
no: long-term fork of Ollama's wrapper
no: server sidecar
no: relying on Ollama's OpenAI-compatible/tool-call behavior for llama
```

Ollama's Go wrapper remains a temporary bring-up dependency only. It is not the
permanent substrate for the graduated local coding node.

## Why The Ollama Wrapper Blocks Us

`runtime/modelrepo/llama/llamasession` currently uses:

```go
import "github.com/ollama/ollama/llama"
```

That wrapper exposes enough for early live prefix reuse:

```text
Model.Tokenize
Context.Decode
Context.KvCacheSeqRm
Context.KvCacheSeqCp
Context.KvCacheSeqAdd
Batch
sampler helpers
```

But it does not expose the ownership primitives llama needs:

```text
llama_free(ctx)
llama_state_seq_get_size
llama_state_seq_get_data
llama_state_seq_set_data
llama_state_seq_save_file
llama_state_seq_load_file
```

It also collapses positive `llama_decode` return codes into generic failure.
Upstream llama.cpp distinguishes:

```text
0      success
1      no KV slot available
2      aborted; partially processed ubatches may remain
-1     invalid input batch
< -1   fatal error
```

The llama runtime needs those distinctions for cancellation, rollback, fatal-session
eviction, and honest context accounting. A partially processed prefill cannot be
treated as a generic KV-full error.

## Rejected Paths

### Unsafe ABI Shim

Rejected as the final path and no longer the L0 plan.

The unsafe route would mirror the beginning of Ollama's private `llama.Context`
layout and read the unexported C pointer:

```go
type Context struct {
    c          *C.struct_llama_context
    numThreads int
}
```

That is too fragile for the runtime core. It can compile while being wrong if
Ollama changes layout, and the failure mode is memory corruption, double free, or
bad snapshots. It also risks duplicate llama.cpp linkage if not fenced perfectly.

Unsafe was useful as a thought experiment because it clarified the missing ABI.
It is not the implementation path.

### Fork / Replace Ollama

Rejected as the main path.

Adding wrappers inside Ollama's `llama` package would solve visibility, but it
means carrying a fork or a heavy vendored replacement for a whole application
runtime when llama only needs a small subset of llama.cpp.

The fork has lower memory-safety risk than unsafe ABI, but higher maintenance
weight than owning a focused shim.

### Upstream PR Only

Not a blocker strategy.

An upstream PR may still be useful later, but release timing and API acceptance
are outside Contenox control. The llama runtime needs an owned path.

## Owned Shim Scope

Create a small package that owns the llama.cpp C API boundary for llama:

```text
runtime/modelrepo/llama/llamacppshim/
  model.go
  context.go
  batch.go
  sampler.go
  state.go
  lifecycle.go
  errors.go
```

The exact package name can change, but the ownership boundary should not: product
code talks to `llama.Session`; llama-specific code talks to the shim.

Expose only the required API:

```go
type Model struct{}
type Context struct{}
type Batch struct{}
type Sampler struct{}

func LoadModel(path string, cfg ModelConfig) (*Model, error)
func NewContext(model *Model, cfg ContextConfig) (*Context, error)
func (c *Context) Free() error
func (m *Model) Free() error

func (m *Model) Tokenize(text string, addSpecial, parseSpecial bool) ([]int, error)
func (m *Model) TokenToPiece(id int) (string, error)
func (m *Model) TokenIsEOG(id int) bool

func NewBatch(capacity int) (*Batch, error)
func (b *Batch) Free() error
func (b *Batch) Clear()
func (b *Batch) Add(token, pos, seqID int, logits bool) error

func (c *Context) Decode(batch *Batch) DecodeResult
func (c *Context) KVSeqRemove(seqID, p0, p1 int) error
func (c *Context) KVSeqCopy(src, dst, p0, p1 int) error
func (c *Context) KVSeqAdd(seqID, p0, p1, delta int) error

func (c *Context) StateSeqGetData(seqID int) ([]byte, error)
func (c *Context) StateSeqSetData(seqID int, data []byte) error
func (c *Context) StateSeqSaveFile(path string, seqID int, tokens []int) error
func (c *Context) StateSeqLoadFile(path string, seqID int, tokenCapacity int) ([]int, error)
```

`DecodeResult` must preserve llama.cpp's real status:

```go
type DecodeStatus string

const (
    DecodeOK          DecodeStatus = "ok"
    DecodeNoKVSlot    DecodeStatus = "no_kv_slot"
    DecodeAborted     DecodeStatus = "aborted_partial"
    DecodeInvalid     DecodeStatus = "invalid_batch"
    DecodeFatal       DecodeStatus = "fatal"
)

type DecodeResult struct {
    Status DecodeStatus
    Err    error
}
```

## Build Requirements

The shim must own its build/link story explicitly:

```text
default build:
  no native llama.cpp requirement

llamanode build tag:
  builds the Contenox shim
  links one llama.cpp copy
  exposes the llama adapter

tests:
  tiny GGUF only by default
  larger models opt-in through env vars
```

No duplicate llama.cpp copies may be linked into the same binary. The L0 test
must report linked mode and runtime version through `ExplainContext` or benchmark
metadata.

## Integration Plan

### L0 - Owned Shim Round-Trip

Build the shim first. Then prove:

```text
load tiny GGUF
create model/context/batch/sampler
tokenize prompt
prefill prompt
save seq state
create fresh context
load seq state
continue greedy decode
restored continuation equals original
close context/model without leak or crash
```

Also record:

```text
state bytes
save ms
restore ms
decode status
llama.cpp version/build metadata
```

### L1 - Port `llamasession`

Replace direct use of `github.com/ollama/ollama/llama` inside the graduated
llama adapter. The implementation target is the single `runtime/modelrepo/llama`
package; the old `runtime/modelrepo/local` and `runtime/modelrepo/localnode`
packages are retired. Compatibility is a backend keyword shim only:
`modelrepo.CanonicalBackendType("local") == "llama"`.

### L2 - Warm Prefix / Suffix Equivalence

Use the same owned shim to prove:

```text
warm stable prefix + suffix == cold full prompt
edited stable segment causes precise miss/reuse behavior
decode cancellation yields valid or explicitly fatal session
failed suffix prefill rolls back or marks session fatal
```

### L3 - Snapshot Policy

Only after L0/L2 numbers exist, decide whether snapshots are hot-path useful or
only durability/branching/reproduction tools. The hot path remains live prefix
reuse unless measurements prove restore is better.

## Required Llama Contracts

The shim does not leak into product code. It feeds the existing llama
primitives:

```go
EnsurePrefix(ctx, PrefixInput{Text, Manifest}) -> PrefixStatus
PrefillSuffix(ctx, SuffixInput{Text, Manifest}) -> SuffixStatus
Decode(ctx, DecodeConfig) -> StreamChunk
ExplainContext() -> ContextReport
Close() error
```

Compatibility rules before restore or prefix reuse:

```text
model digest must match
backend version must match
prompt template digest must match
runtime digest must match
BOS policy must match
stable token hash must match or restore is refused
```

## Kill Gates

Do not graduate the shim until all pass:

```text
tiny GGUF <=512 MiB
state get returns non-empty bytes
restore into fresh context succeeds
one-token greedy continuation equals original
multi-token greedy continuation equals original
restore with wrong manifest is rejected
double close does not crash
cancel during prefill/decode leaves session valid or fatal, never ambiguous
no duplicate llama.cpp link/copy
decode status preserves aborted/no-slot/invalid/fatal distinctions
```

Minimum report:

```json
{
  "model": "tiny",
  "backend": "llamacpp",
  "mode": "contenox_owned_shim",
  "prefix_tokens": 123,
  "state_bytes": 456789,
  "save_ms": 12,
  "restore_ms": 18,
  "continuation_equal": true,
  "decode_status_mapping": "preserved",
  "close_mode": "llama_free",
  "duplicate_llama_link": false,
  "notes": []
}
```

## Current Action

Build the Contenox-owned shim. Do not spend more implementation time on unsafe
ABI access or an Ollama fork unless the owned shim hits a concrete blocker.
