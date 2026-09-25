package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/contenox/contenox/internal/store/runtimetypes"
)

// Bus subjects for gateway event distribution.
const (
	// SubjectProxyUsageTurn is published whenever a gateway finishes an LLM turn.
	SubjectProxyUsageTurn = "gateway.proxy.usage.turn"
	// SubjectProxyControlRevocation is broadcast when a key or client budget is revoked or exhausted.
	SubjectProxyControlRevocation = "gateway.proxy.control.revocation"
	// SubjectProxyControlSync is the request-reply subject for cold-start snapshot bootstrap.
	SubjectProxyControlSync = "gateway.proxy.control.sync"
)

// ProxyUsageTurnEvent is the payload published to SubjectProxyUsageTurn.
type ProxyUsageTurnEvent struct {
	Usage  runtimetypes.ProxyUsage `json:"usage"`
	Origin string                  `json:"origin,omitempty"`
}

// ProxyControlRevocationEvent is broadcast across gateways to instantly halt spent clients.
type ProxyControlRevocationEvent struct {
	ClientID  string    `json:"client_id,omitempty"`
	KeyHash   string    `json:"key_hash,omitempty"`
	Model     string    `json:"model,omitempty"`
	Reason    string    `json:"reason"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ProxyControlLock is one broadcast cutoff. Model empty means every model, which
// is what an operator revoking a key states; a cutoff raised because one model's
// week ran out names that model, so it does not silence the others.
type ProxyControlLock struct {
	ClientID  string    `json:"client_id,omitempty"`
	KeyHash   string    `json:"key_hash,omitempty"`
	Model     string    `json:"model,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ProxyControlSyncRequest struct {
	Origin string `json:"origin"`
}
type ProxyControlSyncSnapshot struct {
	Locks []ProxyControlLock `json:"locks"`
}

type revocationCache struct {
	mu          sync.RWMutex
	clientLocks map[string]time.Time
	keyLocks    map[string]time.Time
}

func newRevocationCache() *revocationCache {
	return &revocationCache{
		clientLocks: make(map[string]time.Time),
		keyLocks:    make(map[string]time.Time),
	}
}

// IsBlocked reports a broadcast cutoff for this client or key, for this model or
// for every model. A cutoff that names a model is only about that model: one
// exhausted allowance must not refuse the models that still have budget.
func (c *revocationCache) IsBlocked(clientID, keyHash, model string, now time.Time) (bool, string) {
	if c == nil {
		return false, ""
	}
	for _, lookup := range []struct {
		holder string
		id     string
		reason string
	}{
		{holder: "client", id: clientID, reason: "client budget exhausted"},
		{holder: "key", id: keyHash, reason: "proxy key revoked"},
	} {
		if lookup.id == "" {
			continue
		}
		if blocked, reason := c.blocked(lookup.holder, lookup.id, model, now, lookup.reason); blocked {
			return true, reason
		}
	}
	return false, ""
}

func (c *revocationCache) blocked(holder, id, model string, now time.Time, reason string) (bool, string) {
	locks := c.locksFor(holder)
	for _, scope := range []string{id + "\x00" + model, id} {
		if model == "" && scope != id {
			continue
		}
		c.mu.RLock()
		expires, ok := locks[scope]
		c.mu.RUnlock()
		if !ok {
			continue
		}
		if now.Before(expires) {
			if model != "" && scope != id {
				return true, reason + " for model " + model
			}
			return true, reason
		}
		c.mu.Lock()
		if expires, ok := locks[scope]; ok && !now.Before(expires) {
			delete(locks, scope)
		}
		c.mu.Unlock()
	}
	return false, ""
}

func (c *revocationCache) locksFor(holder string) map[string]time.Time {
	if holder == "client" {
		return c.clientLocks
	}
	return c.keyLocks
}

// BlockClient marks a client identity blocked until expiresAt, for one model or
// for every model when it is empty.
func (c *revocationCache) BlockClient(clientID, model string, expiresAt time.Time) {
	if c == nil || clientID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientLocks[lockScope(clientID, model)] = expiresAt
}

// BlockKey marks a proxy key hash blocked until expiresAt, for one model or for
// every model when it is empty.
func (c *revocationCache) BlockKey(keyHash, model string, expiresAt time.Time) {
	if c == nil || keyHash == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keyLocks[lockScope(keyHash, model)] = expiresAt
}

func lockScope(id, model string) string {
	if model == "" {
		return id
	}
	return id + "\x00" + model
}

// Prune removes all entries whose expiration timestamp is before or equal to now.
func (c *revocationCache) Prune(now time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, exp := range c.clientLocks {
		if !now.Before(exp) {
			delete(c.clientLocks, k)
		}
	}
	for k, exp := range c.keyLocks {
		if !now.Before(exp) {
			delete(c.keyLocks, k)
		}
	}
}

func (c *revocationCache) Snapshot(now time.Time) ProxyControlSyncSnapshot {
	if c == nil {
		return ProxyControlSyncSnapshot{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	locks := []ProxyControlLock{}
	for holder, byScope := range map[string]map[string]time.Time{"client": c.clientLocks, "key": c.keyLocks} {
		for scope, exp := range byScope {
			if !now.Before(exp) {
				continue
			}
			id, model, _ := strings.Cut(scope, "\x00")
			lock := ProxyControlLock{Model: model, ExpiresAt: exp}
			if holder == "client" {
				lock.ClientID = id
			} else {
				lock.KeyHash = id
			}
			locks = append(locks, lock)
		}
	}
	return ProxyControlSyncSnapshot{Locks: locks}
}

func (c *revocationCache) LoadSnapshot(snap ProxyControlSyncSnapshot, now time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, lock := range snap.Locks {
		if !now.Before(lock.ExpiresAt) {
			continue
		}
		if lock.ClientID != "" {
			c.clientLocks[lockScope(lock.ClientID, lock.Model)] = lock.ExpiresAt
		}
		if lock.KeyHash != "" {
			c.keyLocks[lockScope(lock.KeyHash, lock.Model)] = lock.ExpiresAt
		}
	}
}

const (
	subscribeBootstrapAttempts = 16
	subscribeBootstrapBackoff  = 250 * time.Millisecond
	subscribeBootstrapTimeout  = 1 * time.Second
	subscribeBootstrapBudget   = 10 * time.Second
)

// SubscribeControlPlane listens to broadcast control/revocation events, and
// answers a peer's bootstrap request with the cutoffs this instance holds. The
// responder belongs here rather than with the usage consumer: a peer asking for
// the snapshot is recovering its own state, and it must be answered whether or
// not this instance also runs the metering feed.
func (s *service) SubscribeControlPlane(ctx context.Context) error {
	if s.bus == nil || s.revocations == nil {
		return nil
	}

	if _, err := s.bus.Serve(ctx, SubjectProxyControlSync, func(_ context.Context, _ []byte) ([]byte, error) {
		return json.Marshal(s.revocations.Snapshot(time.Now().UTC()))
	}); err != nil {
		return fmt.Errorf("gateway: serve control sync: %w", err)
	}

	go s.bootstrapFromPeer(ctx)

	ch := make(chan []byte, 128)
	sub, err := s.bus.Stream(ctx, SubjectProxyControlRevocation, ch)
	if err != nil {
		return fmt.Errorf("gateway: subscribe control plane: %w", err)
	}

	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		defer sub.Unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.revocations.Prune(time.Now().UTC())
			case data, ok := <-ch:
				if !ok {
					return
				}
				var ev ProxyControlRevocationEvent
				if err := json.Unmarshal(data, &ev); err != nil {
					continue
				}
				s.applyRevocation(ev)
			}
		}
	}()
	return nil
}

// StartUsageConsumer starts the background ingestion loop that records usage
// events published to SubjectProxyUsageTurn into the central DB, and elects this
// deployment's one meter writer. A follower still drains the subject so its
// caller is not blocked, but only the lease holder's write is kept: a usage
// snapshot built twice from one predecessor loses a turn.
func (s *service) StartUsageConsumer(ctx context.Context) error {
	if s.bus == nil || s.db == nil {
		return nil
	}
	if _, err := s.meter.Start(ctx); err != nil {
		return fmt.Errorf("gateway: meter writer: %w", err)
	}

	ch := make(chan []byte, 256)
	sub, err := s.bus.Stream(ctx, SubjectProxyUsageTurn, ch)
	if err != nil {
		return fmt.Errorf("gateway: subscribe usage: %w", err)
	}

	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-ch:
				if !ok {
					return
				}
				var ev ProxyUsageTurnEvent
				if err := json.Unmarshal(data, &ev); err != nil {
					s.reportConsumerFailure(ctx, "usage_event_malformed", err)
					continue
				}
				if ev.Origin != "" && ev.Origin == s.instanceID {
					continue
				}
				if _, err := s.meter.Record(ctx, ev.Usage); err != nil {
					s.reportConsumerFailure(ctx, "meter_async_ingest", err)
				}
			}
		}
	}()
	return nil
}

// reportConsumerFailure records an ingestion failure once, on the span an
// operator already follows and in the durable trouble log.
func (s *service) reportConsumerFailure(ctx context.Context, op string, err error) {
	if s.trouble != nil {
		s.trouble.Record(ctx, "gateway", op)
	}
	reportErr, _, end := s.tracker.Start(ctx, "consume", op)
	reportErr(err)
	end()
}

// bootstrapFromPeer asks for a peer's cutoffs and loads them. An empty answer is
// retried rather than believed: every gateway holds something (possibly nothing,
// possibly only its own cutoffs), so an empty answer says no more than "that
// instance had none", and the gateway answering may have been this one.
//
// The detached context is deliberate. The caller stops this work when the
// process stops, and the deadline stops it when no peer ever answers, so a
// lost request cannot leave a goroutine retrying for the life of the gateway.
func (s *service) bootstrapFromPeer(ctx context.Context) {
	ask, err := json.Marshal(ProxyControlSyncRequest{Origin: s.instanceID})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), subscribeBootstrapBudget)
	defer cancel()
	for attempt := 0; attempt < subscribeBootstrapAttempts; attempt++ {
		syncCtx, cancelRequest := context.WithTimeout(ctx, subscribeBootstrapTimeout)
		resp, err := s.bus.Request(syncCtx, SubjectProxyControlSync, ask)
		cancelRequest()
		if err == nil {
			var snap ProxyControlSyncSnapshot
			if json.Unmarshal(resp, &snap) == nil && len(snap.Locks) > 0 {
				s.revocations.LoadSnapshot(snap, time.Now().UTC())
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(subscribeBootstrapBackoff):
		}
	}
}

// BroadcastRevocation cuts a client or key off on every gateway sharing the bus,
// this one included: the raiser applies the lock itself rather than waiting to
// receive its own event, which on a polling bus costs a poll interval and leaves
// the one gateway that knows why the lock exists answering peers without it.
func (s *service) BroadcastRevocation(ctx context.Context, ev ProxyControlRevocationEvent) error {
	s.applyRevocation(ev)
	if s.bus == nil {
		return nil
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return s.bus.Publish(ctx, SubjectProxyControlRevocation, raw)
}

func (s *service) applyRevocation(ev ProxyControlRevocationEvent) {
	if s.revocations == nil {
		return
	}
	if ev.ClientID != "" {
		s.revocations.BlockClient(ev.ClientID, ev.Model, ev.ExpiresAt)
	}
	if ev.KeyHash != "" {
		s.revocations.BlockKey(ev.KeyHash, ev.Model, ev.ExpiresAt)
	}
}
