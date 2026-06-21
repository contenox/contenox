// Package llama is the graduated local coding-node runtime: a persistent,
// workspace-scoped inference session that keeps a stable prefix's KV hot and
// re-prefills only the changed suffix (the live warm-reuse hot path), distinct
// from the toy fixed-constant `local` provider.
//
// This package defines the backend-neutral session contract. The native
// adapter (the CGO llama.cpp session) now lives behind the modeld boundary and
// implements runtime/transport; no backend is registered in this build, so
// SessionAvailable reports false and the provider returns "unavailable". Product
// code talks to Session, never to llama.cpp or OpenVINO concepts. The hot coding
// loop is EnsurePrefix -> PrefillSuffix -> Decode on a live session.
package llama

import (
	"context"
	"errors"
	"fmt"

	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
)

// The session contract types below are aliases of the backend-neutral
// runtime/transport wire types modeld implements. They are aliases, not
// hand-maintained mirror structs, so new session fields (e.g. the
// effective-context hot/planner budgets) stay in sync automatically instead of
// silently breaking the struct conversions in remoteSession. This matches the
// runtime/modelrepo/openvino sibling. Types the local surface deliberately
// curates (DecodeConfig drops StructuredOutput; StreamChunk remaps per-chunk
// errors) stay distinct and are mapped explicitly below.
type (
	// Config is the explicit runtime configuration for a local session — every
	// knob is a tested setting, not a magic default.
	Config = transport.Config
	// PrefixInput is the stable prefix text plus the manifest that makes reuse
	// valid (tokenizer, template, runtime config, BOS policy, and model identity
	// are part of the cache key — byte equality alone is not enough).
	PrefixInput = transport.PrefixInput
	// SuffixInput is the volatile text appended after the stable prefix, carrying
	// the same manifest so a suffix cannot be prefilled against resident KV from a
	// different profile/template/runtime.
	SuffixInput = transport.SuffixInput
	// PrefixStatus reports what EnsurePrefix reused versus (re)computed — the
	// live-reuse signal: ReusedTokens > 0 means a warm hit.
	PrefixStatus = transport.PrefixStatus
	// SuffixStatus reports the volatile suffix added after the stable prefix.
	SuffixStatus = transport.SuffixStatus
)

// DecodeConfig controls a single decode pass.
type DecodeConfig struct {
	MaxTokens       int
	Temperature     *float64
	TopP            *float64
	TopK            int
	Seed            *int
	ParserProtocols []string
	ReasoningFormat string
}

type ToolCall = transport.ToolCall

// StreamChunk is a decoded text delta, parsed model output, or a terminal error.
type StreamChunk struct {
	Text      string
	Thinking  string
	ToolCalls []ToolCall
	Error     error
}

// ContextReport explains the session's resident context (explain-context). It
// aliases the transport wire type for the same anti-drift reason as the inputs
// above.
type ContextReport = transport.ContextReport

type SessionSnapshot = transport.SessionSnapshot

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

	// Snapshot captures backend state for durability, branching, and benchmark
	// reproducibility.
	Snapshot(ctx context.Context) (SessionSnapshot, error)

	// Restore replaces resident state from a compatible snapshot.
	Restore(ctx context.Context, snap SessionSnapshot) error

	// Close releases the session's resources.
	Close() error
}

// SessionFactory creates a backend session for a model with explicit config.
type SessionFactory func(modelPath string, cfg Config) (Session, error)

var sessionFactory SessionFactory

// SetSessionFactory registers the backend that creates sessions. The native
// CGO adapter has moved behind the modeld boundary; nothing registers a factory
// in this build, so the indirection stays but SessionAvailable reports false.
func SetSessionFactory(f SessionFactory) { sessionFactory = f }

// SessionAvailable reports whether local llama inference can be served: either a
// test factory is registered, or the modeld daemon holds a fresh lease AND is
// serving the llama backend (the cheap offline check). A daemon running in a
// different mode (e.g. openvino) advertises no llama capability. The actual open
// confirms reachability.
func SessionAvailable() bool { return sessionFactory != nil || modeldconn.Backend() == "llama" }

// newSession opens a session. A registered factory (tests) wins; otherwise the
// session is opened on the modeld daemon over runtime/transport and adapted to
// the package-local Session contract. The CGO llama.cpp backend lives in modeld.
func newSession(ref modeldconn.ModelRef, cfg Config) (Session, error) {
	if sessionFactory != nil {
		return sessionFactory(ref.Path, cfg)
	}
	s, err := modeldconn.OpenSession(context.Background(), ref, transport.Config(cfg))
	if err != nil {
		// Preserve the ErrSessionUnavailable contract callers branch on, while
		// keeping the actionable modeld detail (not installed / unreachable / ...).
		return nil, fmt.Errorf("%w: %v", ErrSessionUnavailable, err)
	}
	return remoteSession{s: s}, nil
}

// remoteSession adapts a runtime/transport.Session (resident in modeld) to the
// package-local Session interface. The session-contract types are aliases of the
// transport types, so inputs and outputs pass through directly; this adapter
// exists to remap sentinel errors (so the session cache evicts a closed/stale/
// fatal session and reopens against the current leader) and to curate the
// DecodeConfig/StreamChunk surface.
type remoteSession struct{ s transport.Session }

func (r remoteSession) EnsurePrefix(ctx context.Context, p PrefixInput) (PrefixStatus, error) {
	st, err := r.s.EnsurePrefix(ctx, p)
	return st, mapSessionErr(err)
}

func (r remoteSession) PrefillSuffix(ctx context.Context, s SuffixInput) (SuffixStatus, error) {
	st, err := r.s.PrefillSuffix(ctx, s)
	return st, mapSessionErr(err)
}

func (r remoteSession) Decode(ctx context.Context, cfg DecodeConfig) (<-chan StreamChunk, error) {
	src, err := r.s.Decode(ctx, transportDecodeConfig(cfg))
	if err != nil {
		return nil, mapSessionErr(err)
	}
	out := make(chan StreamChunk, 16)
	go func() {
		defer close(out)
		for c := range src {
			out <- StreamChunk{Text: c.Text, Thinking: c.Thinking, ToolCalls: c.ToolCalls, Error: mapSessionErr(c.Error)}
		}
	}()
	return out, nil
}

func transportDecodeConfig(cfg DecodeConfig) transport.DecodeConfig {
	return transport.DecodeConfig{
		MaxTokens:       cfg.MaxTokens,
		Temperature:     cfg.Temperature,
		TopP:            cfg.TopP,
		TopK:            cfg.TopK,
		Seed:            cfg.Seed,
		ParserProtocols: append([]string(nil), cfg.ParserProtocols...),
		ReasoningFormat: cfg.ReasoningFormat,
	}
}

func (r remoteSession) ExplainContext() ContextReport { return r.s.ExplainContext() }

func (r remoteSession) Snapshot(ctx context.Context) (SessionSnapshot, error) {
	snap, err := r.s.Snapshot(ctx)
	return snap, mapSessionErr(err)
}

func (r remoteSession) Restore(ctx context.Context, snap SessionSnapshot) error {
	return mapSessionErr(r.s.Restore(ctx, snap))
}

func (r remoteSession) Close() error { return mapSessionErr(r.s.Close()) }

// mapSessionErr translates transport sentinels to this package's, so the session
// cache's fatal-eviction keeps working over the wire. A stale fence (the owner
// changed under us) is fatal: drop the cached session and reopen on the new
// leader.
func mapSessionErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, transport.ErrSessionClosed):
		return ErrSessionClosed
	case errors.Is(err, transport.ErrSessionFatal), errors.Is(err, transport.ErrStaleFence), errors.Is(err, transport.ErrSlotGenerationStale), errors.Is(err, transport.ErrModelNotActive):
		return fmt.Errorf("%w: %v", ErrSessionFatal, err)
	case errors.Is(err, transport.ErrContextOverflow):
		return fmt.Errorf("%w: %v", ErrContextOverflow, err)
	default:
		return err
	}
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
		return nil, fmt.Errorf("%w: embeddings require a native llama embedding backend", ErrSessionUnavailable)
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
