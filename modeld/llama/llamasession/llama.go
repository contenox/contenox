//go:build llamanode && llamacpp_direct

// Package llamasession is the llama.cpp adapter for llama.Session.
package llamasession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/contenox/runtime/modeld/internal/sessionkit"
	"github.com/contenox/runtime/modeld/llama"
	"github.com/contenox/runtime/modeld/llama/llamacppshim"
	"github.com/contenox/runtime/modeld/residency"
	"github.com/contenox/runtime/runtime/contextasm"
)

// Available reports whether the llama.cpp backend is compiled into this build.
const Available = true

// init registers this backend so the llama provider can create sessions
// without importing this CGo package (no import cycle). NewWithAdapters is the
// registered factory; New stays a no-adapter convenience that delegates to it.
func init() { llama.SetSessionFactory(NewWithAdapters) }

type session struct {
	mu                           sync.Mutex
	model                        *llamacppshim.Model
	lctx                         *llamacppshim.Context
	batch                        *llamacppshim.Batch
	adapters                     []*llamacppshim.Adapter // applied LoRA adapters, freed before the model on close
	numCtx                       int
	plannerCtx                   int
	coldMaxTokens                int
	coldTokens                   int
	coldClock                    int64
	coldBlocks                   map[string]*coldBlock
	coldRangeKey                 map[string]string
	sparseAttention              bool
	slidingWindowAttentionTokens int
	nBatch                       int
	addBOS                       bool
	resident                     []int  // token IDs currently in the KV (seq 0), in order
	prefixLen                    int    // how many of resident are the stable prefix
	stableText                   string // raw stable text from the runtime
	prefixText                   string // the templated stable text whose tokens are resident
	stableMsgs                   []chatTemplateMessage
	tools                        string // JSON tool definitions rendered into the prompt (model-native)
	manifest                     llama.ContextManifest
	chatSyntax                   llamacppshim.ChatSyntax
	reasoning                    string
	closed                       bool

	residencyPlan residency.Plan
	residencyErr  string
}

var _ llama.Session = (*session)(nil)
var _ residency.Executor = (*session)(nil)

func (s *session) Capabilities() residency.Capabilities {
	return residency.Capabilities{
		RemoveTail:                   true,
		RemoveMiddle:                 true,
		PositionShift:                true,
		SparseAttention:              s.sparseAttention,
		SlidingWindowAttentionTokens: s.slidingWindowAttentionTokens,
		ColdStore:                    s.coldMaxTokens > 0,
	}
}

// EvictRange drops the resident token range [r.Start, r.End) from the KV cache
// and slides the surviving tail down by the removed width, so the remaining
// tokens keep contiguous RoPE positions (the StreamingLLM sliding-window move:
// remove the middle, shift the tail; llama.cpp re-applies RoPE to the shifted
// cells on the next decode). It updates the logical resident bookkeeping to
// match. The residency policy is responsible for never handing this primitive a
// protected range (attention sinks / task-pinned prefix); EvictRange trusts the
// range it is given.
func (s *session) EvictRange(ctx context.Context, r residency.Range) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return llama.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.evictRangeLocked(r.Start, r.End)
}

// evictRangeLocked removes [a,b) from the KV, slides the tail, and updates
// resident bookkeeping. The caller holds s.mu and has checked s.closed.
func (s *session) evictRangeLocked(a, b int) error {
	n := len(s.resident)
	if a < 0 || b > n || a >= b {
		return fmt.Errorf("llamasession: evict range [%d,%d) outside resident [0,%d)", a, b, n)
	}
	block, err := s.exportColdBlockLocked(a, b)
	if err != nil {
		return err
	}
	if err := s.removeKV(a, b); err != nil {
		return err
	}
	if b < n {
		s.lctx.MemorySeqAdd(0, b, -1, -(b - a))
	}
	s.resident = append(s.resident[:a], s.resident[b:]...)
	oldPrefix := s.prefixLen
	s.prefixLen = min(a, oldPrefix) + max(0, oldPrefix-b)
	if a < oldPrefix {
		s.prefixText = ""
	}
	if block != nil {
		s.storeColdBlockLocked(block)
	}
	return nil
}

func (s *session) slideForDecodeLocked() (bool, error) {
	recent := residency.DeriveEvictionBudget(s.numCtx, s.slidingWindowAttentionTokens, 1).RecentTokens
	a := s.prefixLen
	b := len(s.resident) - recent
	if b <= a {
		return false, nil
	}
	if err := s.evictRangeLocked(a, b); err != nil {
		return false, err
	}
	return true, nil
}

func (s *session) AdmitRange(ctx context.Context, r residency.Range) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return llama.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.admitRangeLocked(ctx, r)
}

func New(modelPath string, cfg llama.Config) (llama.Session, error) {
	return NewWithAdapters(modelPath, cfg, nil)
}

func NewWithAdapters(modelPath string, cfg llama.Config, adapters []llama.AdapterSpec) (llama.Session, error) {
	model, err := llamacppshim.LoadModel(modelPath, modelConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("llamasession: load model %q: %w", modelPath, err)
	}

	numCtx := cfg.NumCtx
	if numCtx <= 0 {
		numCtx = 8192
	}
	numCtx -= numCtx % 2
	plannerCtx := cfg.PlannerEffectiveContext
	if plannerCtx <= 0 {
		plannerCtx = numCtx
	}
	if plannerCtx < numCtx {
		plannerCtx = numCtx
	}
	coldMaxTokens := max(plannerCtx-numCtx, 0)
	slidingWindow := model.SlidingWindowAttention()
	nBatch := cfg.NumBatch
	if nBatch <= 0 {
		nBatch = 512
	}
	addBOS := model.AddBOS() && !cfg.DisableBOS
	nThreads := cfg.NumThreads
	if nThreads <= 0 {
		nThreads = runtime.NumCPU()
	}

	lctx, err := llamacppshim.NewContext(model, llamacppshim.ContextConfig{
		NumCtx:      numCtx,
		NumBatch:    nBatch,
		NumSeqMax:   1 + min(coldMaxTokens, 1),
		NumThreads:  nThreads,
		FlashAttn:   flashAttnMode(cfg.FlashAttn),
		KVCacheType: cfg.KVCacheType,
		Embeddings:  false,
		OffloadKQV:  cfg.NumGpuLayers != 0,
		KVUnified:   coldMaxTokens > 0,
	})
	if err != nil {
		model.Close()
		return nil, fmt.Errorf("llamasession: new context: %w", err)
	}
	batch, err := llamacppshim.NewBatch(nBatch, 1, 0)
	if err != nil {
		lctx.Close()
		model.Close()
		return nil, fmt.Errorf("llamasession: new batch: %w", err)
	}

	loaded, err := applyAdapters(model, lctx, adapters)
	if err != nil {
		batch.Free()
		lctx.Close()
		model.Close()
		return nil, err
	}

	return &session{
		model:                        model,
		lctx:                         lctx,
		batch:                        batch,
		adapters:                     loaded,
		numCtx:                       numCtx,
		plannerCtx:                   plannerCtx,
		coldMaxTokens:                coldMaxTokens,
		sparseAttention:              slidingWindow > 0,
		slidingWindowAttentionTokens: slidingWindow,
		nBatch:                       nBatch,
		addBOS:                       addBOS,
		reasoning:                    cfg.ReasoningFormat,
	}, nil
}

func applyAdapters(model *llamacppshim.Model, lctx *llamacppshim.Context, adapters []llama.AdapterSpec) ([]*llamacppshim.Adapter, error) {
	if len(adapters) == 0 {
		return nil, nil
	}
	loaded := make([]*llamacppshim.Adapter, 0, len(adapters))
	for _, spec := range adapters {
		ad, err := model.LoadAdapter(spec.Path)
		if err != nil {
			freeAdapters(loaded)
			return nil, llama.NewUnsupportedFeatureError(fmt.Sprintf("lora adapter %q: %v", spec.Name, err))
		}
		scale := spec.Scale
		if scale == 0 {
			scale = 1.0
		}
		if err := lctx.SetAdapter(ad, scale); err != nil {
			ad.Free()
			freeAdapters(loaded)
			return nil, fmt.Errorf("llamasession: apply lora adapter %q: %w", spec.Name, err)
		}
		loaded = append(loaded, ad)
	}
	return loaded, nil
}

func freeAdapters(adapters []*llamacppshim.Adapter) {
	for _, ad := range adapters {
		ad.Free()
	}
}

func modelConfig(cfg llama.Config) llamacppshim.ModelConfig {
	return llamacppshim.ModelConfig{
		NumGPULayers: cfg.NumGpuLayers,
		TensorSplit:  cfg.TensorSplit,
		UseMmap:      true,
	}
}

func flashAttnMode(forceOn bool) llamacppshim.FlashAttnMode {
	if forceOn {
		return llamacppshim.FlashAttnOn
	}
	return llamacppshim.FlashAttnAuto
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

	stableMsgs := stableMessages(prefix.Text, prefix.Manifest)
	text := prefix.Text
	if len(stableMsgs) > 0 {
		templated, err := s.renderTemplate(stableMsgs, prefix.Tools, false)
		if err != nil {
			return llama.PrefixStatus{}, fmt.Errorf("llamasession: apply chat template: %w", err)
		}
		text = templated
	}

	toks, err := s.tokenize(text, s.addBOS)
	if err != nil {
		return llama.PrefixStatus{}, fmt.Errorf("llamasession: tokenize prefix: %w", err)
	}
	if len(toks) > s.numCtx {
		return llama.PrefixStatus{}, llama.NewContextOverflowError("prefix", 0, len(toks), s.numCtx)
	}

	oldResident := len(s.resident)
	reuse := 0
	if ok, _ := s.manifest.CompatibleRuntime(prefix.Manifest); !ok {
		if err := s.removeKV(0, -1); err != nil {
			return llama.PrefixStatus{}, err
		}
		s.clearColdStoreLocked()
		s.resident = nil
		s.prefixLen = 0
		s.prefixText = ""
		s.manifest = llama.ContextManifest{}
	} else {
		reuse = sessionkit.CommonPrefixLen(s.resident, toks)
	}
	if reuse < len(s.resident) {
		if err := s.removeKV(reuse, -1); err != nil {
			return llama.PrefixStatus{}, err
		}
		s.clearColdStoreLocked()
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
	s.stableText = prefix.Text
	s.prefixText = text
	s.stableMsgs = stableMsgs
	s.tools = prefix.Tools
	s.chatSyntax = llamacppshim.ChatSyntax{}
	enriched, err := s.enrichStableSegments(prefix.Manifest, stableMsgs, toks)
	if err != nil {
		return llama.PrefixStatus{}, err
	}
	s.manifest = enriched
	s.updateResidencyPlanLocked(false)

	return llama.PrefixStatus{
		ReusedTokens:    reuse,
		PrefilledTokens: len(toks) - reuse,
		DroppedTokens:   oldResident - reuse,
		PrefixTokens:    len(toks),
		ResidentTokens:  len(s.resident),
		AvailableTokens: s.numCtx - len(s.resident),
		StableByteHash:  prefix.Manifest.StableByteHash,
		StableTokenHash: contextasm.HashTokenIDs(toks),
		ManifestDigest:  prefix.Manifest.Digest(),
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
	volatileMsgs := volatileMessages(suffix.Text, suffix.Manifest)
	suffixText := suffix.Text
	if len(s.stableMsgs)+len(volatileMsgs) > 0 {
		all := append(append([]chatTemplateMessage{}, s.stableMsgs...), volatileMsgs...)
		rendered, err := s.renderTemplateForDecode(all, s.tools, suffix.EnableThinking)
		if err != nil {
			return llama.SuffixStatus{}, fmt.Errorf("llamasession: apply chat template: %w", err)
		}
		full := rendered.Prompt
		if !strings.HasPrefix(full, s.prefixText) {
			return llama.SuffixStatus{}, llama.NewManifestMismatchError("model template is not prefix-stable across the suffix")
		}
		suffixText = full[len(s.prefixText):]
		s.chatSyntax = rendered.Syntax
	}

	addSpecial := s.prefixLen == 0 && s.addBOS
	stoks, err := s.tokenize(suffixText, addSpecial)
	if err != nil {
		return llama.SuffixStatus{}, fmt.Errorf("llamasession: tokenize suffix: %w", err)
	}
	// Residency driver: when the suffix would overflow the hot window, evict
	// ranges selected by the shared policy. With cold storage this parks KV for
	// later admit; without it, StreamingLLM makes the drop intentionally lossy.
	if (s.coldEnabledLocked() || s.streamPolicyLocked().Enabled) && len(s.resident)+len(stoks) > s.numCtx {
		if err := s.driveEvictToFitLocked(len(stoks)); err != nil {
			return llama.SuffixStatus{}, err
		}
	}
	if len(s.resident)+len(stoks) > s.numCtx {
		return llama.SuffixStatus{}, llama.NewContextOverflowError("suffix", len(s.resident), len(stoks), s.numCtx)
	}
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
	if err := s.enrichVolatileSegments(s.prefixLen, volatileMsgs, stoks); err != nil {
		return llama.SuffixStatus{}, err
	}
	s.updateResidencyPlanLocked(true)
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
				sessionkit.TrySend(ch, llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode panicked: %v", llama.ErrSessionFatal, r)})
			}
		}()
		if s.closed {
			if !sessionkit.Send(ctx, ch, llama.StreamChunk{Error: llama.ErrSessionClosed}) {
				return
			}
			return
		}
		if err := ctx.Err(); err != nil {
			sessionkit.TrySend(ch, llama.StreamChunk{Error: err})
			return
		}

		params := llamacppshim.SamplingParams{TopK: 40, TopP: 0.9, MinP: 0.05, Temperature: 0.8}
		if cfg.TopK > 0 {
			params.TopK = cfg.TopK
		}
		if cfg.TopP != nil {
			params.TopP = float32(*cfg.TopP)
		}
		if cfg.Temperature != nil {
			params.Temperature = float32(*cfg.Temperature)
		}
		if cfg.Seed != nil && *cfg.Seed >= 0 {
			params.Seed = uint32(*cfg.Seed)
		}
		sampler, err := llamacppshim.NewSamplingContext(params)
		if err != nil {
			if !sessionkit.Send(ctx, ch, llama.StreamChunk{Error: fmt.Errorf("llamasession: sampler: %w", err)}) {
				return
			}
			return
		}
		defer sampler.Free()

		maxTokens := cfg.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 256
		}
		if len(s.resident) >= s.numCtx {
			slid, err := s.slideForDecodeLocked()
			if err != nil {
				s.closeLocked()
				_ = sessionkit.Send(ctx, ch, llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode slide: %v", llama.ErrSessionFatal, err)})
				return
			}
			if !slid {
				if !sessionkit.Send(ctx, ch, llama.StreamChunk{Error: llama.NewContextOverflowError("decode", len(s.resident), 1, s.numCtx)}) {
					return
				}
				return
			}
		}
		reasoningFormat := cfg.ReasoningFormat
		if reasoningFormat == "" {
			reasoningFormat = s.reasoning
		}
		parser, err := newChatOutputParser(cfg.ParserProtocols, s.chatSyntax, reasoningFormat)
		if err != nil {
			if !sessionkit.Send(ctx, ch, llama.StreamChunk{Error: err}) {
				return
			}
			return
		}
		emitParsed := func(piece string, partial bool) bool {
			if parser == nil {
				if piece != "" {
					return sessionkit.Send(ctx, ch, llama.StreamChunk{Text: piece})
				}
				return true
			}
			text, thinking, toolCalls, err := parser.Push(piece, partial)
			if err != nil {
				return sessionkit.Send(ctx, ch, llama.StreamChunk{Error: err})
			}
			if text != "" || thinking != "" || len(toolCalls) > 0 {
				return sessionkit.Send(ctx, ch, llama.StreamChunk{Text: text, Thinking: thinking, ToolCalls: toolCalls})
			}
			return true
		}
		for n := 0; n < maxTokens; n++ {
			select {
			case <-ctx.Done():
				sessionkit.TrySend(ch, llama.StreamChunk{Error: ctx.Err()})
				return
			default:
			}
			id := sampler.Sample(s.lctx, -1)
			if id < 0 {
				s.closeLocked()
				_ = sessionkit.Send(ctx, ch, llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode sample returned no token", llama.ErrSessionFatal)})
				return
			}
			sampler.Accept(id)
			if s.model.TokenIsEOG(id) {
				emitParsed("", false)
				return
			}

			if len(s.resident) >= s.numCtx {
				slid, err := s.slideForDecodeLocked()
				if err != nil {
					s.closeLocked()
					_ = sessionkit.Send(ctx, ch, llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode slide: %v", llama.ErrSessionFatal, err)})
					return
				}
				if !slid {
					emitParsed("", false)
					return
				}
			}

			s.batch.Clear()
			if err := s.batch.Add(id, nil, len(s.resident), true, 0); err != nil {
				s.closeLocked()
				_ = sessionkit.Send(ctx, ch, llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode batch: %v", llama.ErrSessionFatal, err)})
				return
			}
			if res := s.lctx.Decode(s.batch); res.Status != llamacppshim.DecodeOK {
				s.closeLocked()
				_ = sessionkit.Send(ctx, ch, llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode token: %v", llama.ErrSessionFatal, res.Err)})
				return
			}
			s.resident = append(s.resident, id)
			if !emitParsed(s.model.TokenToPiece(id), true) {
				return
			}
		}
		emitParsed("", false)
	}()
	return ch, nil
}

func (s *session) ExplainContext() llama.ContextReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	return llama.ContextReport{
		ResidentTokens:          len(s.resident),
		PrefixTokens:            s.prefixLen,
		NumCtx:                  s.numCtx,
		HotContextTokens:        s.numCtx,
		PlannerEffectiveContext: s.plannerCtx,
		AvailableTokens:         s.numCtx - len(s.resident),
		StableByteHash:          s.manifest.StableByteHash,
		StableTokenHash:         s.manifest.StableTokenHash,
		ManifestDigest:          s.manifest.Digest(),
		Manifest:                s.manifest,
		Closed:                  s.closed,
		Residency:               sessionkit.ResidencyReport(s.residencyPlan, s.residencyErr, s.Capabilities()),
	}
}

func (s *session) Snapshot(ctx context.Context) (llama.SessionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return llama.SessionSnapshot{}, llama.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return llama.SessionSnapshot{}, err
	}
	state, err := s.lctx.StateGetData()
	if err != nil {
		return llama.SessionSnapshot{}, fmt.Errorf("llamasession snapshot: %w", err)
	}
	return llama.SessionSnapshot{
		State:            state,
		ResidentTokens:   len(s.resident),
		PrefixTokens:     s.prefixLen,
		NumCtx:           s.numCtx,
		ResidentTokenIDs: append([]int(nil), s.resident...),
		ColdKVBlocks:     s.coldBlockSnapshotsLocked(),
		StableText:       s.stableText,
		PrefixText:       s.prefixText,
		Tools:            s.tools,
		Manifest:         s.manifest,
	}, nil
}

func (s *session) Restore(ctx context.Context, snap llama.SessionSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return llama.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap.NumCtx > 0 && snap.NumCtx != s.numCtx {
		return llama.NewManifestMismatchError("snapshot context window changed")
	}
	if snap.ResidentTokens < 0 || snap.PrefixTokens < 0 || snap.PrefixTokens > snap.ResidentTokens {
		return fmt.Errorf("llamasession restore invalid snapshot: prefix_tokens=%d resident_tokens=%d", snap.PrefixTokens, snap.ResidentTokens)
	}
	if snap.ResidentTokens > s.numCtx {
		return llama.NewContextOverflowError("restore", 0, snap.ResidentTokens, s.numCtx)
	}
	if snap.ResidentTokens > 0 && len(snap.ResidentTokenIDs) != snap.ResidentTokens {
		return fmt.Errorf("llamasession restore invalid snapshot: resident_token_ids=%d resident_tokens=%d", len(snap.ResidentTokenIDs), snap.ResidentTokens)
	}
	if !s.manifest.IsZero() && !snap.Manifest.IsZero() {
		if ok, reason := s.manifest.CompatibleRuntime(snap.Manifest); !ok {
			return llama.NewManifestMismatchError(reason)
		}
	}
	if s.lctx == nil {
		s.closeLocked()
		return fmt.Errorf("%w: llamasession restore context is nil", llama.ErrSessionFatal)
	}
	if snap.ResidentTokens > 0 && len(snap.State) == 0 {
		return fmt.Errorf("llamasession restore invalid snapshot: non-empty resident token set has empty state")
	}
	s.lctx.ClearMemory(true)
	if snap.ResidentTokens > 0 {
		if err := s.lctx.StateSetData(snap.State); err != nil {
			s.closeLocked()
			return fmt.Errorf("%w: llamasession restore state: %v", llama.ErrSessionFatal, err)
		}
	}
	s.resident = append([]int(nil), snap.ResidentTokenIDs...)
	s.prefixLen = snap.PrefixTokens
	if err := s.restoreColdBlocksLocked(snap.ColdKVBlocks); err != nil {
		return err
	}
	s.stableText = snap.StableText
	s.prefixText = snap.PrefixText
	s.tools = snap.Tools
	s.manifest = snap.Manifest
	s.stableMsgs = stableMessages(s.stableText, s.manifest)
	if s.prefixText == "" {
		if len(s.stableMsgs) > 0 {
			rendered, err := s.renderTemplate(s.stableMsgs, s.tools, false)
			if err != nil {
				s.closeLocked()
				return fmt.Errorf("%w: llamasession restore template: %v", llama.ErrSessionFatal, err)
			}
			s.prefixText = rendered
		} else {
			s.prefixText = s.stableText
		}
	}
	s.updateResidencyPlanLocked(true)
	return nil
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
		s.lctx.ClearMemory(true)
		s.lctx.Close()
		s.lctx = nil
	}
	if s.batch != nil {
		s.batch.Free()
		s.batch = nil
	}
	freeAdapters(s.adapters)
	s.adapters = nil
	if s.model != nil {
		s.model.Close()
		s.model = nil
	}
	s.resident = nil
	s.prefixLen = 0
	s.plannerCtx = 0
	s.coldMaxTokens = 0
	s.clearColdStoreLocked()
	s.stableText = ""
	s.prefixText = ""
	s.manifest = llama.ContextManifest{}
	s.residencyPlan = residency.Plan{}
	s.residencyErr = ""
}

func stableMessages(text string, m llama.ContextManifest) []chatTemplateMessage {
	var msgs []chatTemplateMessage
	for _, seg := range m.Segments {
		if !seg.Stable {
			continue
		}
		role := sessionkit.ChatRole(seg.Kind)
		if role == "" || seg.ByteStart < 0 || seg.ByteEnd > len(text) || seg.ByteStart > seg.ByteEnd {
			continue
		}
		msgs = append(msgs, chatTemplateMessage{
			Role:       role,
			Content:    text[seg.ByteStart:seg.ByteEnd],
			ToolCalls:  seg.ToolCallsJSON,
			ToolCallID: seg.ToolCallID,
		})
	}
	return msgs
}

func volatileMessages(text string, m llama.ContextManifest) []chatTemplateMessage {
	var msgs []chatTemplateMessage
	base := m.StableBytes
	for _, seg := range m.Segments {
		if seg.Stable {
			continue
		}
		role := sessionkit.ChatRole(seg.Kind)
		if role == "" {
			continue
		}
		lo, hi := seg.ByteStart-base, seg.ByteEnd-base
		if lo < 0 || hi > len(text) || lo > hi {
			continue
		}
		msgs = append(msgs, chatTemplateMessage{
			Role:       role,
			Content:    text[lo:hi],
			ToolCalls:  seg.ToolCallsJSON,
			ToolCallID: seg.ToolCallID,
		})
	}
	return msgs
}

func (s *session) enrichStableSegments(m llama.ContextManifest, stableMsgs []chatTemplateMessage, toks []int) (llama.ContextManifest, error) {
	m.Segments = append([]llama.ManifestSegment(nil), m.Segments...)
	m.StableTokenHash = contextasm.HashTokenIDs(toks)
	prevEnd, msgIdx := 0, 0
	for i := range m.Segments {
		seg := &m.Segments[i]
		if !seg.Stable {
			continue
		}
		if sessionkit.ChatRole(seg.Kind) == "" {
			seg.TokenStart, seg.TokenEnd = prevEnd, prevEnd
			seg.TokenHash = contextasm.HashTokenIDs(toks[prevEnd:prevEnd])
			continue
		}
		msgIdx++
		end := len(toks)
		if msgIdx < len(stableMsgs) {
			rendered, err := s.renderTemplate(stableMsgs[:msgIdx], s.tools, false)
			if err != nil {
				return llama.ContextManifest{}, fmt.Errorf("llamasession: stable segment template: %w", err)
			}
			cum, err := s.tokenize(rendered, s.addBOS)
			if err != nil {
				return llama.ContextManifest{}, fmt.Errorf("llamasession: stable segment tokenize: %w", err)
			}
			end = len(cum)
		}
		if end < prevEnd || end > len(toks) {
			return llama.ContextManifest{}, llama.NewManifestMismatchError("stable segment token boundary out of range")
		}
		seg.TokenStart, seg.TokenEnd = prevEnd, end
		seg.TokenHash = contextasm.HashTokenIDs(toks[prevEnd:end])
		prevEnd = end
	}
	return m, nil
}

func (s *session) enrichVolatileSegments(prefixTokens int, volatileMsgs []chatTemplateMessage, stoks []int) error {
	s.manifest.Segments = append([]llama.ManifestSegment(nil), s.manifest.Segments...)
	s.manifest.VolatileTokenHash = contextasm.HashTokenIDs(stoks)
	allMsgs := append(append([]chatTemplateMessage{}, s.stableMsgs...), volatileMsgs...)
	prevEnd, msgIdx := 0, len(s.stableMsgs)
	for i := range s.manifest.Segments {
		seg := &s.manifest.Segments[i]
		if seg.Stable {
			continue
		}
		if sessionkit.ChatRole(seg.Kind) == "" {
			seg.TokenStart = prefixTokens + prevEnd
			seg.TokenEnd = prefixTokens + len(stoks)
			seg.TokenHash = contextasm.HashTokenIDs(stoks[prevEnd:])
			prevEnd = len(stoks)
			continue
		}
		msgIdx++
		end := len(stoks)
		if msgIdx < len(allMsgs) {
			rendered, err := s.renderTemplate(allMsgs[:msgIdx], s.tools, false)
			if err != nil {
				return fmt.Errorf("llamasession: volatile segment template: %w", err)
			}
			cum, err := s.tokenize(rendered, s.addBOS)
			if err != nil {
				return fmt.Errorf("llamasession: volatile segment tokenize: %w", err)
			}
			end = len(cum) - prefixTokens
		}
		if end < prevEnd || end > len(stoks) {
			return llama.NewManifestMismatchError("volatile segment token boundary out of range")
		}
		seg.TokenStart = prefixTokens + prevEnd
		seg.TokenEnd = prefixTokens + end
		seg.TokenHash = contextasm.HashTokenIDs(stoks[prevEnd:end])
		prevEnd = end
	}
	return nil
}

func (s *session) renderTemplate(msgs []chatTemplateMessage, tools string, addAssistant bool) (string, error) {
	result, err := s.renderTemplateWithOptions(msgs, tools, addAssistant, nil)
	if err != nil {
		return "", err
	}
	return result.Prompt, nil
}

func (s *session) renderTemplateForDecode(msgs []chatTemplateMessage, tools string, enableThinking *bool) (llamacppshim.ChatTemplateResult, error) {
	return s.renderTemplateWithOptions(msgs, tools, true, enableThinking)
}

func (s *session) renderTemplateWithOptions(msgs []chatTemplateMessage, tools string, addAssistant bool, enableThinking *bool) (llamacppshim.ChatTemplateResult, error) {
	msgsJSON, err := chatMessagesJSON(msgs)
	if err != nil {
		return llamacppshim.ChatTemplateResult{}, err
	}
	thinking := true
	if enableThinking != nil {
		thinking = *enableThinking
	}
	return s.model.ApplyChatTemplateCommonWithOptions(msgsJSON, tools, llamacppshim.ChatTemplateOptions{
		AddAssistant:    addAssistant,
		ReasoningFormat: s.reasoning,
		EnableThinking:  thinking,
	})
}

func (s *session) tokenize(text string, addSpecial bool) ([]int, error) {
	return s.model.Tokenize(text, addSpecial, true)
}

const (
	llamaCommonChatReasoningParser = "llama:common_chat_reasoning_parser"
	llamaCommonChatToolParser      = "llama:common_chat_tool_parser"
)

type chatOutputParser struct {
	syntax          llamacppshim.ChatSyntax
	reasoningFormat string
	parseToolCalls  bool
	raw             strings.Builder
	content         string
	thinking        string
}

func newChatOutputParser(protocols []string, syntax llamacppshim.ChatSyntax, configuredReasoningFormat string) (*chatOutputParser, error) {
	var reasoningFormat string
	var parseToolCalls bool
	for _, protocol := range protocols {
		switch protocol {
		case "":
			continue
		case llamaCommonChatReasoningParser:
			if configuredReasoningFormat == "" {
				return nil, fmt.Errorf("%w: reasoning format is required for parser protocol %q", llama.ErrUnsupportedFeature, protocol)
			}
			reasoningFormat = configuredReasoningFormat
		case llamaCommonChatToolParser:
			parseToolCalls = true
		default:
			return nil, fmt.Errorf("%w: parser protocol %q", llama.ErrUnsupportedFeature, protocol)
		}
	}
	if reasoningFormat == "" && !parseToolCalls {
		return nil, nil
	}
	return &chatOutputParser{syntax: syntax, reasoningFormat: reasoningFormat, parseToolCalls: parseToolCalls}, nil
}

func (p *chatOutputParser) Push(piece string, partial bool) (textDelta, thinkingDelta string, toolCalls []llama.ToolCall, err error) {
	if p == nil {
		return piece, "", nil, nil
	}
	p.raw.WriteString(piece)
	parsed, err := llamacppshim.ParseChatResponse(p.raw.String(), partial, p.syntax, p.reasoningFormat, p.parseToolCalls)
	if err != nil {
		return "", "", nil, err
	}
	textDelta = stringDelta(p.content, parsed.Content)
	thinkingDelta = stringDelta(p.thinking, parsed.Thinking)
	p.content = parsed.Content
	p.thinking = parsed.Thinking
	if p.parseToolCalls && !partial && parsed.ToolCallsJSON != "" && parsed.ToolCallsJSON != "[]" {
		if err := json.Unmarshal([]byte(parsed.ToolCallsJSON), &toolCalls); err != nil {
			return "", "", nil, fmt.Errorf("llamasession: parse tool calls: %w", err)
		}
	}
	return textDelta, thinkingDelta, toolCalls, nil
}

func stringDelta(previous, current string) string {
	if strings.HasPrefix(current, previous) {
		return current[len(previous):]
	}
	return current
}

func (s *session) removeKV(p0, p1 int) error {
	if s.lctx == nil {
		s.closeLocked()
		return fmt.Errorf("%w: llamasession context is nil during kv remove", llama.ErrSessionFatal)
	}
	if !s.lctx.MemorySeqRemove(0, p0, p1) {
		s.closeLocked()
		return fmt.Errorf("%w: llamasession kv remove failed seq=0 p0=%d p1=%d", llama.ErrSessionFatal, p0, p1)
	}
	return nil
}

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
			if err := s.batch.Add(toks[j], nil, startPos+j, logitsOnLast && j == n-1, 0); err != nil {
				return fmt.Errorf("llamasession: prefill batch add: %w", err)
			}
		}
		if res := s.lctx.Decode(s.batch); res.Status != llamacppshim.DecodeOK {
			return fmt.Errorf("llamasession: prefill decode: %w", res.Err)
		}
	}
	return nil
}

func (s *session) updateResidencyPlanLocked(requireComplete bool) {
	s.residencyPlan = residency.Plan{}
	s.residencyErr = ""
	if s.numCtx <= 0 || len(s.resident) == 0 {
		return
	}
	blocks, err := residency.BlocksFromManifest(s.manifest, residency.ManifestOptions{
		ResidentTokens:  len(s.resident),
		RequireComplete: requireComplete,
	})
	if err != nil {
		s.residencyErr = err.Error()
		if len(blocks) == 0 {
			return
		}
	}
	plan, planErr := residency.PlanHotSet(residency.PlanInput{
		Blocks:       blocks,
		BudgetTokens: s.numCtx,
		StreamPolicy: s.streamPolicyLocked(),
		Capabilities: s.Capabilities(),
	})
	if planErr != nil {
		s.residencyErr = planErr.Error()
		return
	}
	if err != nil {
		plan.Diagnostics = append(plan.Diagnostics, err.Error())
	}
	s.residencyPlan = plan
}

// planForBudgetLocked computes a residency plan for the current resident tape
// under an explicit hot-token budget, without mutating s.residencyPlan. The
// overflow driver uses it to decide which ranges to park so an incoming prefill
// fits the hot window. Mirrors the OpenVINO adapter.
func (s *session) planForBudgetLocked(budget int) (residency.Plan, error) {
	if budget < 0 {
		budget = 0
	}
	blocks, err := residency.BlocksFromManifest(s.manifest, residency.ManifestOptions{
		ResidentTokens:  len(s.resident),
		RequireComplete: false,
	})
	if err != nil && len(blocks) == 0 {
		return residency.Plan{}, err
	}
	return residency.PlanHotSet(residency.PlanInput{
		Blocks:       blocks,
		BudgetTokens: budget,
		StreamPolicy: s.streamPolicyLocked(),
		Capabilities: s.Capabilities(),
	})
}

func (s *session) streamPolicyLocked() residency.StreamPolicy {
	budget := residency.DeriveEvictionBudget(s.numCtx, s.slidingWindowAttentionTokens, 1)
	return residency.StreamPolicy{
		Enabled:      budget.Valid(),
		SinkTokens:   budget.SinkTokens,
		RecentTokens: budget.RecentTokens,
	}
}

// driveEvictToFitLocked parks evictable ranges to the cold store so the current
// resident plus `incoming` tokens fits numCtx, executing the residency plan
// computed for the tightened budget. Ranges are evicted tail-first so earlier
// ranges keep stable indices. The inline residency driver: hot overflow parks
// recoverable context to host KV instead of overflowing. Caller holds s.mu.
func (s *session) driveEvictToFitLocked(incoming int) error {
	plan, err := s.planForBudgetLocked(s.numCtx - incoming)
	if err != nil {
		return err
	}
	for _, r := range residency.EvictColdRanges(plan) {
		if len(s.resident)+incoming <= s.numCtx {
			break
		}
		if err := s.evictRangeLocked(r.Start, r.End); err != nil {
			return err
		}
	}
	return nil
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
