//go:build llamanode && llamacpp_direct

package llamasession

import (
	"fmt"
	"sort"

	"github.com/contenox/runtime/modeld/llama"
	"github.com/contenox/runtime/modeld/residency"
	"github.com/contenox/runtime/runtime/contextasm"
)

const llamaColdScratchSeqID = 1

type coldBlock struct {
	Range      residency.Range
	Tokens     []int
	TokenHash  string
	KV         []byte
	CacheClass contextasm.CacheClass
	LastUsed   int64
}

func (s *session) coldEnabledLocked() bool {
	return s.coldMaxTokens > 0
}

func (s *session) clearColdStoreLocked() {
	s.coldTokens = 0
	s.coldClock = 0
	s.coldBlocks = nil
	s.coldRangeKey = nil
}

func (s *session) exportColdBlockLocked(a, b int) (*coldBlock, error) {
	if !s.coldEnabledLocked() {
		return nil, nil
	}
	if b <= a {
		return nil, nil
	}
	tokens := append([]int(nil), s.resident[a:b]...)
	if len(tokens) > s.coldMaxTokens {
		return nil, nil
	}
	if err := s.clearScratchSeqLocked(); err != nil {
		return nil, err
	}
	s.lctx.MemorySeqCopy(0, llamaColdScratchSeqID, a, b)
	defer s.clearScratchSeqLocked()

	kv, err := s.lctx.StateSeqGetData(llamaColdScratchSeqID)
	if err != nil {
		return nil, fmt.Errorf("%w: llamasession export cold kv [%d,%d): %v", llama.ErrSessionFatal, a, b, err)
	}
	return &coldBlock{
		Range:      residency.Range{Start: a, End: b},
		Tokens:     tokens,
		TokenHash:  contextasm.HashTokenIDs(tokens),
		KV:         kv,
		CacheClass: s.cacheClassForRangeLocked(residency.Range{Start: a, End: b}),
	}, nil
}

func (s *session) storeColdBlockLocked(block *coldBlock) {
	if block == nil || len(block.Tokens) == 0 || len(block.KV) == 0 || !s.coldEnabledLocked() {
		return
	}
	if s.coldBlocks == nil {
		s.coldBlocks = map[string]*coldBlock{}
		s.coldRangeKey = map[string]string{}
	}
	key := coldBlockKey(block.Range, block.TokenHash)
	if old := s.coldBlocks[key]; old != nil {
		s.coldTokens -= len(old.Tokens)
	}
	s.coldClock++
	block.LastUsed = s.coldClock
	s.coldBlocks[key] = block
	s.coldRangeKey[coldRangeKey(block.Range)] = key
	s.coldTokens += len(block.Tokens)
	s.trimColdStoreLocked(key)
}

func (s *session) trimColdStoreLocked(protectedKey string) {
	for s.coldTokens > s.coldMaxTokens {
		victimKey := s.coldVictimLocked(protectedKey)
		if victimKey == "" {
			return
		}
		s.deleteColdBlockLocked(victimKey)
	}
}

func (s *session) coldVictimLocked(protectedKey string) string {
	type candidate struct {
		key string
		b   *coldBlock
	}
	candidates := make([]candidate, 0, len(s.coldBlocks))
	for key, block := range s.coldBlocks {
		if key == protectedKey {
			continue
		}
		candidates = append(candidates, candidate{key: key, b: block})
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i].b, candidates[j].b
		if a.CacheClass != b.CacheClass {
			return a.CacheClass.MoreEvictableThan(b.CacheClass)
		}
		if a.LastUsed != b.LastUsed {
			return a.LastUsed < b.LastUsed
		}
		if a.Range.Start != b.Range.Start {
			return a.Range.Start < b.Range.Start
		}
		return a.Range.End < b.Range.End
	})
	return candidates[0].key
}

func (s *session) admitRangeLocked(r residency.Range) error {
	if !s.coldEnabledLocked() {
		return fmt.Errorf("%w: llamasession admit range [%d,%d) requires a cold KV store", llama.ErrUnsupportedFeature, r.Start, r.End)
	}
	key := s.coldRangeKey[coldRangeKey(r)]
	block := s.coldBlocks[key]
	if block == nil {
		return fmt.Errorf("%w: llamasession no cold KV block for range [%d,%d)", llama.ErrUnsupportedFeature, r.Start, r.End)
	}
	if len(s.resident)+len(block.Tokens) > s.numCtx {
		return llama.NewContextOverflowError("admit", len(s.resident), len(block.Tokens), s.numCtx)
	}
	if err := s.clearScratchSeqLocked(); err != nil {
		return err
	}
	if err := s.lctx.StateSeqSetData(llamaColdScratchSeqID, block.KV); err != nil {
		_ = s.clearScratchSeqLocked()
		return fmt.Errorf("%w: llamasession import cold kv [%d,%d): %v", llama.ErrSessionFatal, r.Start, r.End, err)
	}

	newStart := len(s.resident)
	newEnd := newStart + len(block.Tokens)
	if delta := newStart - block.Range.Start; delta != 0 {
		s.lctx.MemorySeqAdd(llamaColdScratchSeqID, block.Range.Start, block.Range.End, delta)
	}
	s.lctx.MemorySeqCopy(llamaColdScratchSeqID, 0, newStart, newEnd)
	if err := s.clearScratchSeqLocked(); err != nil {
		return err
	}
	s.resident = append(s.resident, block.Tokens...)
	s.deleteColdBlockLocked(key)
	return nil
}

func (s *session) clearScratchSeqLocked() error {
	if s.lctx == nil {
		s.closeLocked()
		return fmt.Errorf("%w: llamasession context is nil during scratch kv clear", llama.ErrSessionFatal)
	}
	if !s.lctx.MemorySeqRemove(llamaColdScratchSeqID, -1, -1) {
		s.closeLocked()
		return fmt.Errorf("%w: llamasession scratch kv clear failed seq=%d", llama.ErrSessionFatal, llamaColdScratchSeqID)
	}
	return nil
}

func (s *session) deleteColdBlockLocked(key string) {
	block := s.coldBlocks[key]
	if block == nil {
		return
	}
	delete(s.coldBlocks, key)
	delete(s.coldRangeKey, coldRangeKey(block.Range))
	s.coldTokens -= len(block.Tokens)
	if s.coldTokens < 0 {
		s.coldTokens = 0
	}
}

func (s *session) cacheClassForRangeLocked(r residency.Range) contextasm.CacheClass {
	class := contextasm.ClassVolatile
	for _, block := range s.residencyPlan.KeepHot {
		if block.Range.Start < r.End && r.Start < block.Range.End {
			if class.MoreEvictableThan(block.CacheClass) {
				class = block.CacheClass
			}
		}
	}
	for _, block := range s.residencyPlan.EvictCold {
		if block.Range.Start < r.End && r.Start < block.Range.End {
			if class.MoreEvictableThan(block.CacheClass) {
				class = block.CacheClass
			}
		}
	}
	return class
}

func coldBlockKey(r residency.Range, hash string) string {
	return fmt.Sprintf("%d:%d:%s", r.Start, r.End, hash)
}

func coldRangeKey(r residency.Range) string {
	return fmt.Sprintf("%d:%d", r.Start, r.End)
}
