// Package transport defines the contract the modeld daemon implements and the
// runtime calls: a persistent, manifest-keyed warm-reuse inference session.
//
// The boundary is the backend-neutral session seam already validated on
// llama.cpp and OpenVINO — EnsurePrefix / PrefillSuffix / Decode. The runtime
// owns this contract; modeld implements it per backend.
//
// An earlier draft put a lower token-level Evaluate/Generate boundary here.
// Stress-checking it against the real llama.Session and OpenVINO GenAISession
// showed both backends sit at this higher, manifest-keyed altitude: OpenVINO
// GenAI holds the tokenizer and chat template internally and caches a string
// prefix, so a token-only daemon could not honor its proven prefix reuse. See
// docs/blueprints/modeld-interface-boundary.md.
package transport

import (
	"context"
	"errors"

	"github.com/contenox/runtime/runtime/contextasm"
)

// ContextManifest is the shared, backend-neutral cache key: profile, model,
// tokenizer/template digests, BOS policy, and stable/volatile hashes. Reuse is
// valid only when the manifest matches; byte equality alone is not enough.
type ContextManifest = contextasm.ContextManifest

// Fence carries the owner identity a client expects to be serving it. It is
// supplied once, at OpenSession; the returned Session is bound to that owner
// epoch, so a takeover invalidates the session rather than every method needing
// a fence. It is a freshness check, not an authentication secret.
type Fence struct {
	OwnerInstanceID string
}

// Config is the explicit hardware/runtime configuration for a session. Every
// knob is a tested setting, not a magic default.
type Config struct {
	NumCtx       int       // context window in tokens
	NumBatch     int       // prefill batch size
	NumThreads   int       // CPU threads (0 = NumCPU)
	NumGpuLayers int       // layers offloaded to the GPU (0 = CPU only)
	TensorSplit  []float32 // multi-GPU split
	FlashAttn    bool
	KVCacheType  string // "", "q8_0", "q4_0"

	PromptFormat         string // profile-declared prompt format, e.g. "chatml" or "llama3"
	PromptTemplateDigest string // digest of the declared/rendered prompt template
	DisableBOS           bool
}

// Service is the entry point modeld serves: it opens persistent sessions on the
// owned hardware. Opening is where the model is made resident and the session is
// bound to the owner epoch.
type Service interface {
	OpenSession(ctx context.Context, req OpenSessionRequest) (Session, error)
}

// OpenSessionRequest asks the owner to open a session for a model.
type OpenSessionRequest struct {
	Fence
	ModelID string
	Config  Config
}

// Session is a persistent, workspace-scoped inference session. The hot coding
// loop is EnsurePrefix -> PrefillSuffix -> Decode: keep the stable prefix's KV
// hot, re-prefill only the changed suffix, decode.
type Session interface {
	// EnsurePrefix makes the resident KV equal `prefix`, reusing the longest
	// already-resident matching prefix and prefilling only the divergent tail
	// (this also drops any previous suffix and generated tokens).
	EnsurePrefix(ctx context.Context, prefix PrefixInput) (PrefixStatus, error)

	// PrefillSuffix prefills the volatile suffix (diff / test output / user
	// turn) after the stable prefix, leaving the stable KV untouched.
	PrefillSuffix(ctx context.Context, suffix SuffixInput) (SuffixStatus, error)

	// Decode streams generated text from the current resident state.
	Decode(ctx context.Context, cfg DecodeConfig) (<-chan StreamChunk, error)

	// ExplainContext reports the resident context for observability.
	ExplainContext() ContextReport

	// Close releases the session's resources.
	Close() error
}

// PrefixInput is the stable prefix text plus the manifest that makes reuse
// valid: tokenizer, template, runtime config, BOS policy, and model identity are
// part of the cache key, not just the text.
type PrefixInput struct {
	Text     string
	Manifest ContextManifest
	// Tools is a JSON array of tool definitions to render into the prompt via the
	// model's own GGUF chat template (model-native tool calls). "" means no tools.
	// The daemon renders it; the runtime never sees the model's tool format.
	Tools string `json:",omitempty"`
}

// SuffixInput is the volatile text appended after the stable prefix. It carries
// the same manifest so a suffix cannot be prefilled against resident KV from a
// different profile/template/runtime.
type SuffixInput struct {
	Text     string
	Manifest ContextManifest
}

// PrefixStatus reports what EnsurePrefix reused versus had to (re)compute.
// ReusedTokens > 0 is a warm hit.
type PrefixStatus struct {
	ReusedTokens    int
	PrefilledTokens int
	DroppedTokens   int
	PrefixTokens    int
	ResidentTokens  int
	AvailableTokens int
	StableByteHash  string
	StableTokenHash string
	ManifestDigest  string
}

// SuffixStatus reports the volatile suffix added after the stable prefix.
type SuffixStatus struct {
	SuffixTokens    int
	PrefixTokens    int
	ResidentTokens  int
	AvailableTokens int
	ManifestDigest  string
}

// DecodeConfig controls a single decode pass.
type DecodeConfig struct {
	MaxTokens   int
	Temperature *float64
	TopP        *float64
	TopK        int
	Seed        *int
}

// StreamChunk is a decoded text delta or a terminal error.
type StreamChunk struct {
	Text  string
	Error error
}

// ContextReport explains the session's resident context (explain-context).
type ContextReport struct {
	ResidentTokens  int
	PrefixTokens    int
	NumCtx          int
	AvailableTokens int
	StableByteHash  string
	StableTokenHash string
	ManifestDigest  string
	Manifest        ContextManifest
	Closed          bool
}

// Canonical errors expected to cross the boundary.
var (
	ErrNotOwner        = errors.New("instance is not the local runtime owner")
	ErrStaleFence      = errors.New("stale owner fence token")
	ErrSessionClosed   = errors.New("session is closed")
	ErrContextOverflow = errors.New("exceeded the session context window")
)
