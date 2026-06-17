package transport

import (
	"context"
	"sync"
)

// MemoryService is an in-process, in-memory Service. It does no real inference:
// it models the warm-reuse contract so the runtime wrapper can be built and
// tested against the boundary before any CGO backend exists. Reuse is keyed on
// the manifest (a changed stable segment OR a changed profile/template/runtime
// digest invalidates the resident prefix), and token counts are byte-length
// proxies. See docs/blueprints/modeld-interface-boundary.md.
//
// It is safe for concurrent use.
type MemoryService struct {
	owner string // expected fence; empty means fencing is disabled
}

// Option configures a MemoryService.
type Option func(*MemoryService)

// WithOwnerFence makes OpenSession reject a request whose Fence does not match
// ownerInstanceID with ErrStaleFence. With no fence configured (the default),
// the fence is ignored, keeping the unwired placeholder path simple.
func WithOwnerFence(ownerInstanceID string) Option {
	return func(m *MemoryService) { m.owner = ownerInstanceID }
}

// NewMemoryService returns an in-memory Service.
func NewMemoryService(opts ...Option) *MemoryService {
	m := &MemoryService{}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

var _ Service = (*MemoryService)(nil)

// OpenSession binds a session to the owner epoch (the fence) and the requested
// context window.
func (m *MemoryService) OpenSession(_ context.Context, req OpenSessionRequest) (Session, error) {
	if m.owner != "" && req.Fence.OwnerInstanceID != m.owner {
		return nil, ErrStaleFence
	}
	return &memSession{numCtx: req.Config.NumCtx}, nil
}

type memSession struct {
	mu sync.Mutex

	numCtx int
	closed bool

	// resident state, keyed on the manifest that produced it
	residentStableHash string
	residentDigest     string
	prefixTokens       int
	suffixTokens       int
}

var _ Session = (*memSession)(nil)

// tokenProxy is a deterministic stand-in for tokenization (one "token" per
// rune). The real backend tokenizes; the noop only needs stable counts.
func tokenProxy(text string) int { return len([]rune(text)) }

func (s *memSession) resident() int { return s.prefixTokens + s.suffixTokens }

func (s *memSession) available() int {
	if s.numCtx <= 0 {
		return 0
	}
	return s.numCtx - s.resident()
}

func (s *memSession) EnsurePrefix(_ context.Context, prefix PrefixInput) (PrefixStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return PrefixStatus{}, ErrSessionClosed
	}

	want := tokenProxy(prefix.Text)
	digest := prefix.Manifest.Digest()
	stableHash := prefix.Manifest.StableByteHash

	if s.numCtx > 0 && want > s.numCtx {
		return PrefixStatus{}, ErrContextOverflow
	}

	// Warm only when the resident prefix came from a manifest with the same
	// stable hash AND the same identity digest (profile/template/runtime).
	warm := s.residentStableHash == stableHash && s.residentDigest == digest && want > 0

	var reused, prefilled, dropped int
	if warm {
		reused = want
	} else {
		dropped = s.prefixTokens
		prefilled = want
	}

	// EnsurePrefix drops any prior suffix and generated tokens.
	s.suffixTokens = 0
	s.prefixTokens = want
	s.residentStableHash = stableHash
	s.residentDigest = digest

	return PrefixStatus{
		ReusedTokens:    reused,
		PrefilledTokens: prefilled,
		DroppedTokens:   dropped,
		PrefixTokens:    s.prefixTokens,
		ResidentTokens:  s.resident(),
		AvailableTokens: s.available(),
		StableByteHash:  stableHash,
		StableTokenHash: prefix.Manifest.StableTokenHash,
		ManifestDigest:  digest,
	}, nil
}

func (s *memSession) PrefillSuffix(_ context.Context, suffix SuffixInput) (SuffixStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return SuffixStatus{}, ErrSessionClosed
	}
	add := tokenProxy(suffix.Text)
	if s.numCtx > 0 && s.resident()+add > s.numCtx {
		return SuffixStatus{}, ErrContextOverflow
	}
	s.suffixTokens += add
	return SuffixStatus{
		SuffixTokens:    s.suffixTokens,
		PrefixTokens:    s.prefixTokens,
		ResidentTokens:  s.resident(),
		AvailableTokens: s.available(),
		ManifestDigest:  suffix.Manifest.Digest(),
	}, nil
}

func (s *memSession) Decode(ctx context.Context, cfg DecodeConfig) (<-chan StreamChunk, error) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, ErrSessionClosed
	}

	out := make(chan StreamChunk, cfg.MaxTokens)
	go func() {
		defer close(out)
		for i := 0; i < cfg.MaxTokens; i++ {
			select {
			case <-ctx.Done():
				out <- StreamChunk{Error: ctx.Err()}
				return
			default:
			}
			out <- StreamChunk{Text: "x"}
		}
	}()
	return out, nil
}

func (s *memSession) ExplainContext() ContextReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ContextReport{
		ResidentTokens:  s.resident(),
		PrefixTokens:    s.prefixTokens,
		NumCtx:          s.numCtx,
		AvailableTokens: s.available(),
		StableByteHash:  s.residentStableHash,
		ManifestDigest:  s.residentDigest,
		Closed:          s.closed,
	}
}

func (s *memSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
