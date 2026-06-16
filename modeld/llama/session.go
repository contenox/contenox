// Package llama is the graduated local coding-node runtime: a persistent,
// workspace-scoped inference session that keeps a stable prefix's KV hot and
// re-prefills only the changed suffix (the live warm-reuse hot path), distinct
// from the toy fixed-constant `local` provider.
//
// This package defines the backend-neutral session contract. Backend adapters
// implement it — llama.cpp now (./llamasession), OpenVINO later. Product code
// talks to Session, never to llama.cpp or OpenVINO concepts. Snapshot/restore
// (durability, branching, crash recovery) is a separate, later concern; the hot
// coding loop is EnsurePrefix -> PrefillSuffix -> Decode on a live session.
package llama

import (
	"context"
	"errors"
	"fmt"
)

// Config is the explicit runtime configuration for a local session. The toy
// constants (4096 ctx, 512 batch, Flash Attention off, 0 GPU layers, fresh
// context per call) die here — every knob is a tested setting, not a magic
// default.
type Config struct {
	NumCtx       int       // context window in tokens
	NumBatch     int       // prefill batch size
	NumThreads   int       // CPU threads (0 = NumCPU)
	NumGpuLayers int       // layers offloaded to GPU (0 = CPU only)
	TensorSplit  []float32 // multi-GPU split
	FlashAttn    bool
	KVCacheType  string // "", "q8_0", "q4_0"

	PromptFormat         string // profile-declared prompt format, e.g. "chatml" or "llama3"
	PromptTemplateDigest string // digest of the declared/rendered prompt template
	DisableBOS           bool   // false means tokenize the stable prefix with backend BOS handling
}

// PrefixInput is the stable prefix text plus the profile/runtime manifest that
// makes reuse valid. Byte equality alone is not enough: tokenizer, template,
// runtime config, BOS policy, and model identity are part of the cache key.
type PrefixInput struct {
	Text     string
	Manifest ContextManifest
}

// SuffixInput is the volatile text appended after the stable prefix. It carries
// the same manifest so direct Session callers cannot accidentally prefill a
// suffix against resident KV from a different profile/template/runtime.
type SuffixInput struct {
	Text     string
	Manifest ContextManifest
}

// PrefixStatus reports what EnsurePrefix reused versus had to (re)compute. This
// is the live-reuse signal: ReusedTokens > 0 means a warm hit.
type PrefixStatus struct {
	ReusedTokens    int // tokens kept warm from the resident prefix
	PrefilledTokens int // divergent tail that had to be (re)prefilled
	DroppedTokens   int // old resident tokens removed before prefill
	PrefixTokens    int // total prefix tokens now resident
	ResidentTokens  int // total resident tokens after EnsurePrefix
	AvailableTokens int // remaining context capacity
	StableByteHash  string
	StableTokenHash string
	ManifestDigest  string
}

// SuffixStatus reports the volatile suffix that was added after the stable
// prefix. It is the measurement point for suffix-growth TTFT curves.
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

// Session is a persistent, workspace-scoped inference session.
//
// The hot coding loop is: keep the stable prefix's KV hot, prefill only the
// changed suffix, decode. EnsurePrefix does token-level longest-common-prefix
// reuse, so an unchanged stable workspace context stays warm across turns and
// only the divergent tail is recomputed.
type Session interface {
	// EnsurePrefix makes the resident KV equal `prefix`, reusing the longest
	// already-resident matching token prefix and prefilling only the divergent
	// tail (this also drops any previous suffix and generated tokens).
	EnsurePrefix(ctx context.Context, prefix PrefixInput) (PrefixStatus, error)

	// PrefillSuffix prefills the volatile suffix (diff / test output / user turn)
	// after the stable prefix, leaving the stable KV untouched.
	PrefillSuffix(ctx context.Context, suffix SuffixInput) (SuffixStatus, error)

	// Decode streams generated text from the current resident state.
	Decode(ctx context.Context, cfg DecodeConfig) (<-chan StreamChunk, error)

	// ExplainContext reports the resident context for observability.
	ExplainContext() ContextReport

	// Close releases the session's resources.
	Close() error
}

// SessionFactory creates a backend session for a model with explicit config.
type SessionFactory func(modelPath string, cfg Config) (Session, error)

var sessionFactory SessionFactory

// SetSessionFactory registers the backend that creates sessions. The llama.cpp
// adapter (./llamasession) calls this from its init when built with the
// 'llamanode' tag, so the provider never imports the CGo package directly (no
// import cycle, default build stays CGo-free).
func SetSessionFactory(f SessionFactory) { sessionFactory = f }

// SessionAvailable reports whether a session backend is compiled into this build.
func SessionAvailable() bool { return sessionFactory != nil }

// newSession creates a session through the registered backend.
func newSession(modelPath string, cfg Config) (Session, error) {
	if sessionFactory == nil {
		return nil, fmt.Errorf("%w: build with -tags llamanode", ErrSessionUnavailable)
	}
	return sessionFactory(modelPath, cfg)
}

// EmbedFunc computes a single embedding via the native backend. The llama.cpp
// adapter registers one from its init when built with the 'llamanode' tag.
type EmbedFunc func(ctx context.Context, modelPath string, cfg Config, input string) ([]float64, error)

var embedFunc EmbedFunc

// SetEmbedFunc registers the native embedding backend.
func SetEmbedFunc(f EmbedFunc) { embedFunc = f }

// EmbedAvailable reports whether an embedding backend is compiled into this build.
func EmbedAvailable() bool { return embedFunc != nil }

// newEmbed computes an embedding through the registered backend.
func newEmbed(ctx context.Context, modelPath string, cfg Config, input string) ([]float64, error) {
	if embedFunc == nil {
		return nil, fmt.Errorf("%w: build with -tags llamanode", ErrSessionUnavailable)
	}
	return embedFunc(ctx, modelPath, cfg, input)
}

var (
	// ErrSessionUnavailable means no native llama backend was compiled into
	// this binary.
	ErrSessionUnavailable = errors.New("llama: session backend unavailable")
	// ErrSessionClosed means the caller used a closed persistent session.
	ErrSessionClosed = errors.New("llama: session closed")
	// ErrContextOverflow means a prefix, suffix, or decode would exceed NumCtx.
	ErrContextOverflow = errors.New("llama: context overflow")
	// ErrUnsupportedFeature marks explicit product-surface gaps such as tools.
	ErrUnsupportedFeature = errors.New("llama: unsupported feature")
	// ErrSessionFatal means the backend marked the session unusable and callers
	// must evict it instead of trying to reuse resident KV.
	ErrSessionFatal = errors.New("llama: session fatal")
)

// ContextOverflowError carries token counts for an overflow at a specific
// primitive boundary.
type ContextOverflowError struct {
	Stage            string
	ResidentTokens   int
	AdditionalTokens int
	NumCtx           int
}

func (e *ContextOverflowError) Error() string {
	return fmt.Sprintf("%s during %s: resident_tokens=%d additional_tokens=%d num_ctx=%d",
		ErrContextOverflow, e.Stage, e.ResidentTokens, e.AdditionalTokens, e.NumCtx)
}

func (e *ContextOverflowError) Is(target error) bool {
	return target == ErrContextOverflow
}

func NewContextOverflowError(stage string, resident, additional, numCtx int) error {
	return &ContextOverflowError{
		Stage:            stage,
		ResidentTokens:   resident,
		AdditionalTokens: additional,
		NumCtx:           numCtx,
	}
}

// UnsupportedFeatureError describes a deliberately unsupported surface.
type UnsupportedFeatureError struct {
	Feature string
}

func (e *UnsupportedFeatureError) Error() string {
	if e.Feature == "" {
		return ErrUnsupportedFeature.Error()
	}
	return fmt.Sprintf("%s: %s", ErrUnsupportedFeature, e.Feature)
}

func (e *UnsupportedFeatureError) Is(target error) bool {
	return target == ErrUnsupportedFeature
}

func NewUnsupportedFeatureError(feature string) error {
	return &UnsupportedFeatureError{Feature: feature}
}
