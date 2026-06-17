//go:build llamanode

// Package llamasession is the llama.cpp adapter for llama.Session. It
// implements the live warm-reuse hot path using only the sequence/KV ops the
// ollama llama binding already exposes (Tokenize, Decode, KvCacheSeqRm) — no
// state save/restore binding is needed for live reuse; that is deferred to the
// snapshot/durability milestone.
package llamasession

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/contenox/runtime/runtime/modelrepo/llama"
	llamacpp "github.com/ollama/ollama/llama"
	"github.com/ollama/ollama/ml"
)

// Available reports whether the llama.cpp backend is compiled into this build.
const Available = true

// init registers this backend so the llama provider can create sessions
// without importing this CGo package (no import cycle).
func init() { llama.SetSessionFactory(New) }

type session struct {
	mu         sync.Mutex
	model      *llamacpp.Model
	lctx       *llamacpp.Context
	batch      *llamacpp.Batch
	numCtx     int
	nBatch     int
	addBOS     bool
	resident   []int // token IDs currently in the KV (seq 0), in order
	prefixLen  int   // how many of resident are the stable prefix
	prefixText string
	manifest   llama.ContextManifest
	closed     bool
}

var _ llama.Session = (*session)(nil)

// New loads a GGUF model and opens one persistent session — the graduated local
// node, not a fresh-context-per-call toy.
func New(modelPath string, cfg llama.Config) (llama.Session, error) {
	model, err := llamacpp.LoadModelFromFile(modelPath, llamacpp.ModelParams{
		NumGpuLayers: cfg.NumGpuLayers,
		TensorSplit:  cfg.TensorSplit,
		UseMmap:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("llamasession: load model %q: %w", modelPath, err)
	}

	numCtx := cfg.NumCtx
	if numCtx <= 0 {
		numCtx = 8192
	}
	nBatch := cfg.NumBatch
	if nBatch <= 0 {
		nBatch = 512
	}
	addBOS := !cfg.DisableBOS
	nThreads := cfg.NumThreads
	if nThreads <= 0 {
		nThreads = runtime.NumCPU()
	}
	fa := ml.FlashAttentionDisabled
	if cfg.FlashAttn {
		fa = ml.FlashAttentionEnabled
	}

	lctx, err := llamacpp.NewContextWithModel(model, llamacpp.NewContextParams(numCtx, nBatch, 1, nThreads, fa, cfg.KVCacheType))
	if err != nil {
		llamacpp.FreeModel(model)
		return nil, fmt.Errorf("llamasession: new context: %w", err)
	}
	batch, err := llamacpp.NewBatch(nBatch, 1, 0)
	if err != nil {
		// The Ollama binding exposes llama_model_free but not llama_free(ctx).
		// Do not free the model while the context still owns it; the owned
		// binding/fork milestone must close this leak completely.
		return nil, fmt.Errorf("llamasession: new batch: %w", err)
	}

	return &session{model: model, lctx: lctx, batch: batch, numCtx: numCtx, nBatch: nBatch, addBOS: addBOS}, nil
}

func (s *session) EnsurePrefix(ctx context.Context, prefix llama.PrefixInput) (llama.PrefixStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return llama.PrefixStatus{}, llama.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return llama.PrefixStatus{}, err
	}

	toks, err := s.tokenize(prefix.Text, s.addBOS)
	if err != nil {
		return llama.PrefixStatus{}, fmt.Errorf("llamasession: tokenize prefix: %w", err)
	}
	if len(toks) > s.numCtx {
		return llama.PrefixStatus{}, llama.NewContextOverflowError("prefix", 0, len(toks), s.numCtx)
	}
	manifest, err := prefix.Manifest.WithStableTokenization(prefix.Text, toks, s.tokenize, s.addBOS)
	if err != nil {
		return llama.PrefixStatus{}, err
	}

	// Longest common token prefix with what is already resident. Everything after
	// it (divergent prefix tail, old suffix, generated tokens) is dropped.
	oldResident := len(s.resident)
	reuse := 0
	if ok, _ := s.manifest.CompatibleRuntime(manifest); !ok {
		if err := s.removeKV(0, -1); err != nil {
			return llama.PrefixStatus{}, err
		}
		s.resident = nil
		s.prefixLen = 0
		s.prefixText = ""
		s.manifest = llama.ContextManifest{}
	} else {
		reuse = commonPrefixLen(s.resident, toks)
	}
	if reuse < len(s.resident) {
		if err := s.removeKV(reuse, -1); err != nil {
			return llama.PrefixStatus{}, err
		}
		s.resident = s.resident[:reuse]
		if s.prefixLen > reuse {
			s.prefixLen = reuse
			s.prefixText = ""
		}
	}
	if err := s.prefillAt(ctx, toks[reuse:], reuse, false); err != nil {
		if rollbackErr := s.removeKV(reuse, -1); rollbackErr != nil {
			return llama.PrefixStatus{}, errors.Join(prefillFailureError("prefix", err), rollbackErr)
		}
		s.resident = s.resident[:reuse]
		if s.prefixLen > reuse {
			s.prefixLen = reuse
			s.prefixText = ""
		}
		if isContextErr(err) {
			return llama.PrefixStatus{}, err
		}
		s.closeLocked()
		return llama.PrefixStatus{}, prefillFailureError("prefix", err)
	}
	s.resident = toks
	s.prefixLen = len(toks)
	s.prefixText = prefix.Text
	s.manifest = manifest

	return llama.PrefixStatus{
		ReusedTokens:    reuse,
		PrefilledTokens: len(toks) - reuse,
		DroppedTokens:   oldResident - reuse,
		PrefixTokens:    len(toks),
		ResidentTokens:  len(s.resident),
		AvailableTokens: s.numCtx - len(s.resident),
		StableByteHash:  manifest.StableByteHash,
		StableTokenHash: manifest.StableTokenHash,
		ManifestDigest:  manifest.Digest(),
	}, nil
}

func (s *session) PrefillSuffix(ctx context.Context, suffix llama.SuffixInput) (llama.SuffixStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return llama.SuffixStatus{}, llama.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return llama.SuffixStatus{}, err
	}
	if ok, reason := s.manifest.CompatibleRuntime(suffix.Manifest); !ok {
		return llama.SuffixStatus{}, llama.NewManifestMismatchError(reason)
	}
	if !s.manifest.IsZero() && !suffix.Manifest.IsZero() && s.manifest.StableByteHash != suffix.Manifest.StableByteHash {
		return llama.SuffixStatus{}, llama.NewManifestMismatchError("stable prefix changed between EnsurePrefix and PrefillSuffix")
	}
	// addSpecial=false: the suffix is mid-context, no second BOS.
	stoks, err := s.tokenize(suffix.Text, false)
	if err != nil {
		return llama.SuffixStatus{}, fmt.Errorf("llamasession: tokenize suffix: %w", err)
	}
	if len(s.resident)+len(stoks) > s.numCtx {
		return llama.SuffixStatus{}, llama.NewContextOverflowError("suffix", len(s.resident), len(stoks), s.numCtx)
	}
	if err := suffix.Manifest.ValidateSplitTokenization(s.prefixText, suffix.Text, s.resident[:s.prefixLen], stoks, s.tokenize, s.addBOS); err != nil {
		return llama.SuffixStatus{}, err
	}
	manifest, err := suffix.Manifest.WithVolatileTokenization(s.manifest, s.prefixLen, suffix.Text, stoks, s.tokenize)
	if err != nil {
		return llama.SuffixStatus{}, err
	}
	// logitsOnLast=true so the final token's logits are ready for the first sample.
	beforeLen := len(s.resident)
	if err := s.prefillAt(ctx, stoks, len(s.resident), true); err != nil {
		if rollbackErr := s.removeKV(beforeLen, -1); rollbackErr != nil {
			return llama.SuffixStatus{}, errors.Join(prefillFailureError("suffix", err), rollbackErr)
		}
		if isContextErr(err) {
			return llama.SuffixStatus{}, err
		}
		s.closeLocked()
		return llama.SuffixStatus{}, prefillFailureError("suffix", err)
	}
	s.resident = append(s.resident, stoks...)
	s.manifest = manifest
	return llama.SuffixStatus{
		SuffixTokens:    len(stoks),
		PrefixTokens:    s.prefixLen,
		ResidentTokens:  len(s.resident),
		AvailableTokens: s.numCtx - len(s.resident),
		ManifestDigest:  s.manifest.Digest(),
	}, nil
}

func (s *session) Decode(ctx context.Context, cfg llama.DecodeConfig) (<-chan llama.StreamChunk, error) {
	ch := make(chan llama.StreamChunk, 16)
	go func() {
		defer close(ch)
		s.mu.Lock()
		defer s.mu.Unlock()
		defer func() {
			if r := recover(); r != nil {
				s.closeLocked()
				ch <- llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode panicked: %v", llama.ErrSessionFatal, r)}
			}
		}()
		if s.closed {
			ch <- llama.StreamChunk{Error: llama.ErrSessionClosed}
			return
		}
		if err := ctx.Err(); err != nil {
			ch <- llama.StreamChunk{Error: err}
			return
		}

		params := llamacpp.SamplingParams{TopK: 40, TopP: 0.9, MinP: 0.05, Temp: 0.8}
		if cfg.TopK > 0 {
			params.TopK = cfg.TopK
		}
		if cfg.TopP != nil {
			params.TopP = float32(*cfg.TopP)
		}
		if cfg.Temperature != nil {
			params.Temp = float32(*cfg.Temperature)
		}
		if cfg.Seed != nil && *cfg.Seed >= 0 {
			params.Seed = uint32(*cfg.Seed)
		}
		sampler, err := llamacpp.NewSamplingContext(s.model, params)
		if err != nil {
			ch <- llama.StreamChunk{Error: fmt.Errorf("llamasession: sampler: %w", err)}
			return
		}

		maxTokens := cfg.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 256
		}
		remaining := s.numCtx - len(s.resident)
		if remaining <= 0 {
			ch <- llama.StreamChunk{Error: llama.NewContextOverflowError("decode", len(s.resident), 1, s.numCtx)}
			return
		}
		if maxTokens > remaining {
			maxTokens = remaining
		}
		for n := 0; n < maxTokens; n++ {
			select {
			case <-ctx.Done():
				ch <- llama.StreamChunk{Error: ctx.Err()}
				return
			default:
			}
			id := sampler.Sample(s.lctx, -1)
			sampler.Accept(id, true)
			if s.model.TokenIsEog(id) {
				return
			}

			s.batch.Clear()
			s.batch.Add(id, nil, len(s.resident), true, 0)
			if err := s.lctx.Decode(s.batch); err != nil {
				s.closeLocked()
				ch <- llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode token: %v", llama.ErrSessionFatal, err)}
				return
			}
			s.resident = append(s.resident, id)
			ch <- llama.StreamChunk{Text: s.model.TokenToPiece(id)}
		}
	}()
	return ch, nil
}

func (s *session) ExplainContext() llama.ContextReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	return llama.ContextReport{
		ResidentTokens:  len(s.resident),
		PrefixTokens:    s.prefixLen,
		NumCtx:          s.numCtx,
		AvailableTokens: s.numCtx - len(s.resident),
		StableByteHash:  s.manifest.StableByteHash,
		StableTokenHash: s.manifest.StableTokenHash,
		ManifestDigest:  s.manifest.Digest(),
		Manifest:        s.manifest,
		Closed:          s.closed,
	}
}

func (s *session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closeLocked()
	return nil
}

func (s *session) closeLocked() {
	if s.closed {
		return
	}
	s.closed = true
	if s.lctx != nil {
		s.lctx.KvCacheClear()
	}
	if s.batch != nil {
		s.batch.Free()
		s.batch = nil
	}
	s.resident = nil
	s.prefixLen = 0
	s.prefixText = ""
	s.manifest = llama.ContextManifest{}
	// NOTE: this binding version exposes llama_model_free but not llama_free(ctx).
	// We cannot safely free the model while an unfreed context still references it.
}

func (s *session) tokenize(text string, addSpecial bool) ([]int, error) {
	return s.model.Tokenize(text, addSpecial, true)
}

func (s *session) removeKV(p0, p1 int) error {
	if s.lctx == nil {
		s.closeLocked()
		return fmt.Errorf("%w: llamasession context is nil during kv remove", llama.ErrSessionFatal)
	}
	if !s.lctx.KvCacheSeqRm(0, p0, p1) {
		s.closeLocked()
		return fmt.Errorf("%w: llamasession kv remove failed seq=0 p0=%d p1=%d", llama.ErrSessionFatal, p0, p1)
	}
	return nil
}

// prefillAt feeds tokens into the KV at absolute positions [startPos, startPos+len),
// chunked by nBatch. logitsOnLast requests logits for the final token (needed
// before sampling); prefix prefill sets it false.
func (s *session) prefillAt(ctx context.Context, toks []int, startPos int, logitsOnLast bool) error {
	n := len(toks)
	for i := 0; i < n; i += s.nBatch {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		end := i + s.nBatch
		if end > n {
			end = n
		}
		s.batch.Clear()
		for j := i; j < end; j++ {
			s.batch.Add(toks[j], nil, startPos+j, logitsOnLast && j == n-1, 0)
		}
		if err := s.lctx.Decode(s.batch); err != nil {
			return fmt.Errorf("llamasession: prefill decode: %w", err)
		}
	}
	return nil
}

func commonPrefixLen(a, b []int) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func prefillFailureError(stage string, err error) error {
	if isContextErr(err) {
		return err
	}
	return fmt.Errorf("%w: llamasession prefill %s failed: %v", llama.ErrSessionFatal, stage, err)
}
