package openvino

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
)

type genAISessionRef struct {
	key     string
	session *ovsession.GenAISession
	once    sync.Once
}

type pooledGenAISession struct {
	session  *ovsession.GenAISession
	refs     int
	lastUsed time.Time
}

var genAISessions = struct {
	sync.Mutex
	entries map[string]*pooledGenAISession
}{
	entries: map[string]*pooledGenAISession{},
}

// Eviction policy for pooled OpenVINO GenAI sessions. Idle sessions (refs==0)
// are kept resident so their warm prefix-cache KV can be reused, but they are
// reaped once they exceed genAISessionIdleTTL or the resident-session cap. Live
// KV is expensive, so the cap is deliberately low for a single-user node.
// Sessions with refs>0 are never evicted.
var (
	genAISessionIdleTTL  = 5 * time.Minute
	genAISessionMaxResid = 2
)

// reapLocked removes evictable entries (refs==0, idle past the TTL, or over the
// resident cap by LRU) from the pool and returns their sessions so the caller
// can Close them outside the pool lock. Must be called with genAISessions held.
func reapLocked(now time.Time) []*ovsession.GenAISession {
	var victims []*ovsession.GenAISession
	if genAISessionIdleTTL > 0 {
		for key, e := range genAISessions.entries {
			if e.refs == 0 && now.Sub(e.lastUsed) > genAISessionIdleTTL {
				victims = append(victims, e.session)
				delete(genAISessions.entries, key)
			}
		}
	}
	if genAISessionMaxResid > 0 {
		for len(genAISessions.entries) > genAISessionMaxResid {
			var lruKey string
			var lru *pooledGenAISession
			for key, e := range genAISessions.entries {
				if e.refs != 0 {
					continue
				}
				if lru == nil || e.lastUsed.Before(lru.lastUsed) {
					lruKey, lru = key, e
				}
			}
			if lru == nil {
				break // everything left is in use; nothing idle to evict
			}
			victims = append(victims, lru.session)
			delete(genAISessions.entries, lruKey)
		}
	}
	return victims
}

func closeGenAISessions(sessions []*ovsession.GenAISession) {
	for _, s := range sessions {
		_ = s.Close()
	}
}

// sweepIdleGenAISessions reaps evictable sessions, closing them outside the pool
// lock so a worker-thread join never blocks other pool operations.
func sweepIdleGenAISessions() {
	genAISessions.Lock()
	victims := reapLocked(time.Now())
	genAISessions.Unlock()
	closeGenAISessions(victims)
}

func acquireGenAISession(ctx context.Context, modelPath, modelDigest string, cfg ovsession.GenAIConfig) (*genAISessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg = normalizedPoolConfig(cfg)
	key := genAISessionKey(modelPath, modelDigest, cfg)
	defer sweepIdleGenAISessions()

	genAISessions.Lock()
	if entry := genAISessions.entries[key]; entry != nil {
		entry.refs++
		entry.lastUsed = time.Now()
		session := entry.session
		genAISessions.Unlock()
		return newGenAISessionRef(key, session), nil
	}
	genAISessions.Unlock()

	session, err := ovsession.NewGenAI(modelPath, cfg)
	if err != nil {
		return nil, err
	}

	genAISessions.Lock()
	if entry := genAISessions.entries[key]; entry != nil {
		entry.refs++
		entry.lastUsed = time.Now()
		existing := entry.session
		genAISessions.Unlock()
		_ = session.Close()
		return newGenAISessionRef(key, existing), nil
	}
	genAISessions.entries[key] = &pooledGenAISession{
		session:  session,
		refs:     1,
		lastUsed: time.Now(),
	}
	genAISessions.Unlock()

	return newGenAISessionRef(key, session), nil
}

func newGenAISessionRef(key string, session *ovsession.GenAISession) *genAISessionRef {
	ref := &genAISessionRef{key: key, session: session}
	runtime.SetFinalizer(ref, (*genAISessionRef).mustClose)
	return ref
}

func (r *genAISessionRef) Generate(ctx context.Context, prompt string, opts ovsession.GenerateOptions) (ovsession.GenAIResult, error) {
	if r == nil || r.session == nil {
		return ovsession.GenAIResult{}, fmt.Errorf("openvino GenAI session reference is closed")
	}
	return r.session.Generate(ctx, prompt, opts)
}

func (r *genAISessionRef) Stream(ctx context.Context, prompt string, opts ovsession.GenerateOptions) (<-chan ovsession.StreamChunk, error) {
	if r == nil || r.session == nil {
		return nil, fmt.Errorf("openvino GenAI session reference is closed")
	}
	return r.session.Stream(ctx, prompt, opts)
}

func (r *genAISessionRef) ApplyChatTemplate(messages []ovsession.ChatMessage, toolsJSON string) (string, error) {
	if r == nil || r.session == nil {
		return "", fmt.Errorf("openvino GenAI session reference is closed")
	}
	return r.session.ApplyChatTemplate(messages, toolsJSON)
}

func (r *genAISessionRef) Tokenize(ctx context.Context, prompt string, addSpecial bool) ([]int, error) {
	if r == nil || r.session == nil {
		return nil, fmt.Errorf("openvino GenAI session reference is closed")
	}
	return r.session.Tokenize(ctx, prompt, addSpecial)
}

func (r *genAISessionRef) Close() error {
	if r == nil {
		return nil
	}
	var err error
	r.once.Do(func() {
		runtime.SetFinalizer(r, nil)
		genAISessions.Lock()
		defer genAISessions.Unlock()
		entry := genAISessions.entries[r.key]
		if entry == nil {
			return
		}
		if entry.refs <= 0 {
			err = fmt.Errorf("openvino GenAI session pool refcount underflow")
			return
		}
		entry.refs--
		entry.lastUsed = time.Now()
	})
	return err
}

func (r *genAISessionRef) mustClose() {
	_ = r.Close()
}

func genAISessionKey(modelPath, modelDigest string, cfg ovsession.GenAIConfig) string {
	parts := []string{
		modelPath,
		modelDigest,
		cfg.Device,
		cfg.KVCachePrecision,
		fmt.Sprint(cfg.CacheSize),
		fmt.Sprint(boolValue(cfg.DynamicSplitFuse, true)),
		fmt.Sprint(boolValue(cfg.EnablePrefixCaching, true)),
		fmt.Sprint(boolValue(cfg.UseSparseAttention, true)),
		fmt.Sprint(cfg.NumLastDenseTokensInPrefill),
		fmt.Sprint(cfg.XAttentionThreshold),
		fmt.Sprint(cfg.XAttentionBlockSize),
		fmt.Sprint(cfg.XAttentionStride),
	}
	return strings.Join(parts, "\x00")
}

func normalizedPoolConfig(cfg ovsession.GenAIConfig) ovsession.GenAIConfig {
	if cfg.KVCachePrecision == "" {
		cfg.KVCachePrecision = "f16"
	}
	if cfg.CacheSize <= 0 {
		cfg.CacheSize = 1
	}
	if cfg.DynamicSplitFuse == nil {
		cfg.DynamicSplitFuse = boolPtr(true)
	}
	if cfg.EnablePrefixCaching == nil {
		cfg.EnablePrefixCaching = boolPtr(true)
	}
	if cfg.UseSparseAttention == nil {
		cfg.UseSparseAttention = boolPtr(true)
	}
	if cfg.NumLastDenseTokensInPrefill <= 0 {
		cfg.NumLastDenseTokensInPrefill = 10
	}
	if cfg.XAttentionThreshold <= 0 {
		cfg.XAttentionThreshold = 0.9
	}
	if cfg.XAttentionBlockSize <= 0 {
		cfg.XAttentionBlockSize = 128
	}
	if cfg.XAttentionStride <= 0 {
		cfg.XAttentionStride = 16
	}
	return cfg
}

func boolValue(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// ShutdownGenAISessions closes every pooled OpenVINO GenAI session and empties
// the pool. It is safe to call on an empty pool (including builds without the
// native backend) and is intended for deterministic teardown on runtime exit.
func ShutdownGenAISessions() error {
	genAISessions.Lock()
	entries := genAISessions.entries
	genAISessions.entries = map[string]*pooledGenAISession{}
	genAISessions.Unlock()

	var first error
	for _, entry := range entries {
		if err := entry.session.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func closeGenAISessionPoolForTest() error {
	return ShutdownGenAISessions()
}

func genAISessionPoolStatsForTest() (entries int, refs int) {
	genAISessions.Lock()
	defer genAISessions.Unlock()
	for _, entry := range genAISessions.entries {
		entries++
		refs += entry.refs
	}
	return entries, refs
}
