//go:build llamanode && llamacpp_direct

// Package llamasession is the llama.cpp adapter for llama.Session. It
// implements the live warm-reuse hot path on the Contenox-owned direct
// llama.cpp shim.
package llamasession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/contenox/runtime/modeld/llama"
	"github.com/contenox/runtime/modeld/llama/llamacppshim"
	"github.com/contenox/runtime/runtime/contextasm"
)

// Available reports whether the llama.cpp backend is compiled into this build.
const Available = true

// init registers this backend so the llama provider can create sessions
// without importing this CGo package (no import cycle).
func init() { llama.SetSessionFactory(New) }

type session struct {
	mu         sync.Mutex
	model      *llamacppshim.Model
	lctx       *llamacppshim.Context
	batch      *llamacppshim.Batch
	numCtx     int
	nBatch     int
	addBOS     bool
	resident   []int  // token IDs currently in the KV (seq 0), in order
	prefixLen  int    // how many of resident are the stable prefix
	stableText string // raw stable text from the runtime
	prefixText string // the templated stable text whose tokens are resident
	stableMsgs []llamacppshim.ChatMessage
	tools      string // JSON tool definitions rendered into the prompt (model-native)
	manifest   llama.ContextManifest
	closed     bool
}

var _ llama.Session = (*session)(nil)

// New loads a GGUF model and opens one persistent session — the graduated local
// node, not a fresh-context-per-call toy.
func New(modelPath string, cfg llama.Config) (llama.Session, error) {
	model, err := llamacppshim.LoadModel(modelPath, modelConfig(cfg))
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
	// BOS policy is model-driven (the GGUF's add_bos_token), not a config guess; an
	// explicit DisableBOS can still force it off.
	addBOS := model.AddBOS() && !cfg.DisableBOS
	nThreads := cfg.NumThreads
	if nThreads <= 0 {
		nThreads = runtime.NumCPU()
	}

	lctx, err := llamacppshim.NewContext(model, llamacppshim.ContextConfig{
		NumCtx:      numCtx,
		NumBatch:    nBatch,
		NumSeqMax:   1,
		NumThreads:  nThreads,
		FlashAttn:   cfg.FlashAttn,
		KVCacheType: cfg.KVCacheType,
		Embeddings:  true,
		OffloadKQV:  cfg.NumGpuLayers != 0,
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

	return &session{model: model, lctx: lctx, batch: batch, numCtx: numCtx, nBatch: nBatch, addBOS: addBOS}, nil
}

func modelConfig(cfg llama.Config) llamacppshim.ModelConfig {
	return llamacppshim.ModelConfig{
		NumGPULayers: cfg.NumGpuLayers,
		TensorSplit:  cfg.TensorSplit,
		UseMmap:      true,
	}
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

	// Render the stable turns with the model's OWN chat template (read from the
	// GGUF), not a hardcoded format. The runtime sends raw content + per-segment
	// roles in the manifest; modeld owns the tokenizer and template.
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

	// Longest common token prefix with what is already resident. Everything after
	// it (divergent prefix tail, old suffix, generated tokens) is dropped.
	oldResident := len(s.resident)
	reuse := 0
	if ok, _ := s.manifest.CompatibleRuntime(prefix.Manifest); !ok {
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
	s.stableText = prefix.Text
	s.prefixText = text
	s.stableMsgs = stableMsgs
	s.tools = prefix.Tools
	enriched, err := s.enrichStableSegments(prefix.Manifest, stableMsgs, toks)
	if err != nil {
		return llama.PrefixStatus{}, err
	}
	s.manifest = enriched

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
	// Template the FULL conversation with the model's own template and take the
	// part after the resident templated stable prefix. The stable KV stays warm;
	// only this suffix is (re)prefilled.
	volatileMsgs := volatileMessages(suffix.Text, suffix.Manifest)
	suffixText := suffix.Text
	if len(s.stableMsgs)+len(volatileMsgs) > 0 {
		all := append(append([]llamacppshim.ChatMessage{}, s.stableMsgs...), volatileMsgs...)
		full, err := s.renderTemplate(all, s.tools, true)
		if err != nil {
			return llama.SuffixStatus{}, fmt.Errorf("llamasession: apply chat template: %w", err)
		}
		if !strings.HasPrefix(full, s.prefixText) {
			return llama.SuffixStatus{}, llama.NewManifestMismatchError("model template is not prefix-stable across the suffix")
		}
		suffixText = full[len(s.prefixText):]
	}

	// When the stable prefix is empty, the BOS (if the model adds one) belongs to
	// these first tokens; otherwise the suffix is mid-context with no second BOS.
	addSpecial := s.prefixLen == 0 && s.addBOS
	stoks, err := s.tokenize(suffixText, addSpecial)
	if err != nil {
		return llama.SuffixStatus{}, fmt.Errorf("llamasession: tokenize suffix: %w", err)
	}
	if len(s.resident)+len(stoks) > s.numCtx {
		return llama.SuffixStatus{}, llama.NewContextOverflowError("suffix", len(s.resident), len(stoks), s.numCtx)
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
	if err := s.enrichVolatileSegments(s.prefixLen, volatileMsgs, stoks); err != nil {
		return llama.SuffixStatus{}, err
	}
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
			ch <- llama.StreamChunk{Error: fmt.Errorf("llamasession: sampler: %w", err)}
			return
		}
		defer sampler.Free()

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
			sampler.Accept(id)
			if s.model.TokenIsEOG(id) {
				return
			}

			s.batch.Clear()
			if err := s.batch.Add(id, nil, len(s.resident), true, 0); err != nil {
				s.closeLocked()
				ch <- llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode batch: %v", llama.ErrSessionFatal, err)}
				return
			}
			if res := s.lctx.Decode(s.batch); res.Status != llamacppshim.DecodeOK {
				s.closeLocked()
				ch <- llama.StreamChunk{Error: fmt.Errorf("%w: llamasession decode token: %v", llama.ErrSessionFatal, res.Err)}
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

func (s *session) Snapshot(ctx context.Context) (llama.SessionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return llama.SessionSnapshot{}, llama.ErrSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return llama.SessionSnapshot{}, err
	}
	state, err := s.lctx.StateSeqGetData(0)
	if err != nil {
		return llama.SessionSnapshot{}, fmt.Errorf("llamasession snapshot: %w", err)
	}
	return llama.SessionSnapshot{
		State:            state,
		ResidentTokens:   len(s.resident),
		PrefixTokens:     s.prefixLen,
		NumCtx:           s.numCtx,
		ResidentTokenIDs: append([]int(nil), s.resident...),
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
		if err := s.lctx.StateSeqSetData(0, snap.State); err != nil {
			s.closeLocked()
			return fmt.Errorf("%w: llamasession restore state: %v", llama.ErrSessionFatal, err)
		}
	}
	s.resident = append([]int(nil), snap.ResidentTokenIDs...)
	s.prefixLen = snap.PrefixTokens
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
	if s.model != nil {
		s.model.Close()
		s.model = nil
	}
	s.resident = nil
	s.prefixLen = 0
	s.stableText = ""
	s.prefixText = ""
	s.manifest = llama.ContextManifest{}
}

// stableMessages reconstructs the stable role/content turns from the manifest
// segments (the runtime sends raw content keyed by role), for the model's own
// chat template. Control segments (BOS, the assistant cue) carry no role and are
// skipped — the template adds them.
func stableMessages(text string, m llama.ContextManifest) []llamacppshim.ChatMessage {
	var msgs []llamacppshim.ChatMessage
	for _, seg := range m.Segments {
		if !seg.Stable {
			continue
		}
		role := chatRole(seg.Kind)
		if role == "" || seg.ByteStart < 0 || seg.ByteEnd > len(text) || seg.ByteStart > seg.ByteEnd {
			continue
		}
		msgs = append(msgs, llamacppshim.ChatMessage{
			Role:       role,
			Content:    text[seg.ByteStart:seg.ByteEnd],
			ToolCalls:  seg.ToolCallsJSON,
			ToolCallID: seg.ToolCallID,
		})
	}
	return msgs
}

// volatileMessages reconstructs the volatile turns; their segment byte ranges are
// global (after the stable bytes), so they are offset into the suffix text.
func volatileMessages(text string, m llama.ContextManifest) []llamacppshim.ChatMessage {
	var msgs []llamacppshim.ChatMessage
	base := m.StableBytes
	for _, seg := range m.Segments {
		if seg.Stable {
			continue
		}
		role := chatRole(seg.Kind)
		if role == "" {
			continue
		}
		lo, hi := seg.ByteStart-base, seg.ByteEnd-base
		if lo < 0 || hi > len(text) || lo > hi {
			continue
		}
		msgs = append(msgs, llamacppshim.ChatMessage{
			Role:       role,
			Content:    text[lo:hi],
			ToolCalls:  seg.ToolCallsJSON,
			ToolCallID: seg.ToolCallID,
		})
	}
	return msgs
}

func chatRole(kind string) string {
	switch kind {
	case "system", "user", "assistant", "tool":
		return kind
	default:
		return ""
	}
}

// enrichStableSegments fills the stored manifest with backend-resolved stable
// token ranges/hashes. modeld owns the tokenizer/template, so a segment's token
// boundary is recovered by tokenizing the model's chat template applied to the
// leading message prefix it ends. The final role segment's boundary is the full
// stable tokenization, so the common single-segment case adds no template calls.
// Control segments (e.g. BOS) carry no message text and get a zero-width range.
func (s *session) enrichStableSegments(m llama.ContextManifest, stableMsgs []llamacppshim.ChatMessage, toks []int) (llama.ContextManifest, error) {
	m.Segments = append([]llama.ManifestSegment(nil), m.Segments...)
	m.StableTokenHash = contextasm.HashTokenIDs(toks)
	prevEnd, msgIdx := 0, 0
	for i := range m.Segments {
		seg := &m.Segments[i]
		if !seg.Stable {
			continue
		}
		if chatRole(seg.Kind) == "" {
			seg.TokenStart, seg.TokenEnd = prevEnd, prevEnd
			seg.TokenHash = contextasm.HashTokenIDs(toks[prevEnd:prevEnd])
			continue
		}
		msgIdx++
		end := len(toks)
		if msgIdx < len(stableMsgs) {
			rendered, err := s.model.ApplyChatTemplate(stableMsgs[:msgIdx], false)
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

// enrichVolatileSegments fills the stored manifest's volatile segment token
// ranges/hashes after the stable prefix, using the same incremental-template
// boundary recovery. Token positions are offset by prefixTokens. A trailing
// control segment (the assistant cue) absorbs any remaining suffix tokens.
func (s *session) enrichVolatileSegments(prefixTokens int, volatileMsgs []llamacppshim.ChatMessage, stoks []int) error {
	s.manifest.Segments = append([]llama.ManifestSegment(nil), s.manifest.Segments...)
	s.manifest.VolatileTokenHash = contextasm.HashTokenIDs(stoks)
	allMsgs := append(append([]llamacppshim.ChatMessage{}, s.stableMsgs...), volatileMsgs...)
	prevEnd, msgIdx := 0, len(s.stableMsgs)
	for i := range s.manifest.Segments {
		seg := &s.manifest.Segments[i]
		if seg.Stable {
			continue
		}
		if chatRole(seg.Kind) == "" {
			seg.TokenStart = prefixTokens + prevEnd
			seg.TokenEnd = prefixTokens + len(stoks)
			seg.TokenHash = contextasm.HashTokenIDs(stoks[prevEnd:])
			prevEnd = len(stoks)
			continue
		}
		msgIdx++
		end := len(stoks)
		if msgIdx < len(allMsgs) {
			rendered, err := s.model.ApplyChatTemplate(allMsgs[:msgIdx], false)
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

// renderTemplate applies the model's own chat template. When tool definitions are
// present it routes through minja (ApplyChatTemplateTools) so the GGUF's Jinja tool
// block is rendered model-natively; the legacy llama_chat_apply_template (used for
// the no-tools path) cannot do that.
func (s *session) renderTemplate(msgs []llamacppshim.ChatMessage, tools string, addAssistant bool) (string, error) {
	if tools != "" {
		msgsJSON, err := chatMessagesJSON(msgs)
		if err != nil {
			return "", err
		}
		return s.model.ApplyChatTemplateTools(msgsJSON, tools, addAssistant)
	}
	return s.model.ApplyChatTemplate(msgs, addAssistant)
}

// chatMessagesJSON marshals chat turns to the JSON array minja expects.
// tool_calls is embedded as a raw JSON value (not re-encoded as a string) so
// Jinja sees it as a proper list; tool_call_id is a plain string.
func chatMessagesJSON(msgs []llamacppshim.ChatMessage) (string, error) {
	type wireMsg struct {
		Role       string          `json:"role"`
		Content    string          `json:"content"`
		ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
	}
	out := make([]wireMsg, len(msgs))
	for i, m := range msgs {
		wm := wireMsg{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		if m.ToolCalls != "" {
			wm.ToolCalls = json.RawMessage(m.ToolCalls)
		}
		out[i] = wm
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *session) tokenize(text string, addSpecial bool) ([]int, error) {
	return s.model.Tokenize(text, addSpecial, true)
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
