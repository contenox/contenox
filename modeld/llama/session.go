// Package llama defines the modeld-side llama backend contract: persistent
// inference sessions keep a stable prefix's KV hot and re-prefill only the
// changed suffix.
//
// This package defines the backend-neutral session contract. Backend adapters
// implement it; product code talks to Session, not llama.cpp internals.
// Snapshot/restore is part of the same contract for durability and branching.
// The main generation path is EnsurePrefix -> PrefillSuffix -> Decode on a live
// session.
package llama

import (
	"context"
	"errors"
	"fmt"

	"github.com/contenox/runtime/runtime/transport"
)

type Config = transport.Config
type PrefixInput = transport.PrefixInput
type SuffixInput = transport.SuffixInput
type PrefixStatus = transport.PrefixStatus
type SuffixStatus = transport.SuffixStatus
type DecodeConfig = transport.DecodeConfig
type ToolCall = transport.ToolCall
type StreamChunk = transport.StreamChunk
type ContextReport = transport.ContextReport
type SessionSnapshot = transport.SessionSnapshot
type Session = transport.Session

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
		return nil, fmt.Errorf("%w: build modeld with -tags 'llamanode llamacpp_direct'", ErrSessionUnavailable)
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
		return nil, fmt.Errorf("%w: build modeld with -tags 'llamanode llamacpp_direct'", ErrSessionUnavailable)
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
