package gateway

import (
	"time"

	"github.com/contenox/contenox/libroutine"
)

const (
	breakerThreshold = 3
	breakerCooldown  = 2 * time.Minute
)

type providerCircuitBreaker struct {
	pool *libroutine.Pool
}

func newProviderCircuitBreaker(threshold int, cooldown time.Duration) *providerCircuitBreaker {
	if threshold <= 0 {
		threshold = 3
	}
	if cooldown <= 0 {
		cooldown = 2 * time.Minute
	}
	return &providerCircuitBreaker{
		pool: libroutine.NewPool(threshold, cooldown),
	}
}

// IsAvailable reports whether a backend may be asked now, consuming the single probe a half-open circuit admits.
// This is the check the model layer makes as it walks its candidates.
func (cb *providerCircuitBreaker) IsAvailable(providerID string, now time.Time) bool {
	if cb == nil {
		return true
	}
	return cb.pool.AllowAt(providerID, now)
}

// WouldServe answers the same question without consuming a probe, for a reader deciding whether an attempt is worth making at all.
func (cb *providerCircuitBreaker) WouldServe(providerID string, now time.Time) bool {
	if cb == nil {
		return true
	}
	return cb.pool.WouldAllowAt(providerID, now)
}

// RecordSuccess resets the consecutive error count for providerID.
func (cb *providerCircuitBreaker) RecordSuccess(providerID string) {
	if cb == nil {
		return
	}
	cb.pool.MarkSuccess(providerID)
}

// RecordFailure notes one refusal and reports whether the backend is now out of service. A dead credential or a rate limit takes it out on the first refusal, because the upstream has already answered the question; anything else counts toward the threshold.
func (cb *providerCircuitBreaker) RecordFailure(providerID string, refusal backendRefusal, now time.Time) bool {
	if cb == nil || providerID == "" {
		return false
	}
	r := cb.pool.Get(providerID)
	if refusal.terminal || refusal.rateLimited {
		r.ForceOpenAt(now)
		return true
	}
	r.MarkFailureAt(now)
	return r.GetState() == libroutine.Open
}
