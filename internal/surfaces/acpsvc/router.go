package acpsvc

import (
	"context"
	"errors"
	"sync"

	"github.com/contenox/contenox/libacp"

	"github.com/contenox/contenox/internal/services/hitlservice"
	"github.com/contenox/contenox/internal/store/runtimetypes"
)

// ErrNoBoundSession reports that no live ACP transport holds the contenox session
// named in the request context.
var ErrNoBoundSession = errors.New("acpsvc: no ACP transport bound to contenox session")

// SessionRouter maps a contenox session id to every ACP Transport that holds it,
// most-recent join first. A session is shared, not owned: joining evicts no one,
// and updates and approvals reach every holder. Safe for concurrent use, and
// every method is nil-safe.
//
// The router also owns the session's [SessionJournal]: every notification it
// relays is recorded there with the session's monotonic sequence, so a holder
// that reconnects can name its last sequence and be replayed exactly the
// notifications it missed (SSE resume over the ACP channel).
type SessionRouter struct {
	mu       sync.RWMutex
	bindings map[string][]*Transport
	journals map[string]*SessionJournal
}

// NewSessionRouter returns an empty router ready to be shared across a serve
// process's ACP WebSocket transports. A session journal retains the newest
// 4096 notifications (capacity-trimmed from the oldest end).
const sessionJournalCapacity = 4096

func NewSessionRouter() *SessionRouter {
	return &SessionRouter{
		bindings: make(map[string][]*Transport),
		journals: make(map[string]*SessionJournal),
	}
}

func (r *SessionRouter) bind(contenoxSessionID string, t *Transport) {
	if r == nil || contenoxSessionID == "" || t == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	held := r.bindings[contenoxSessionID]
	for i, existing := range held {
		if existing == t {
			held = append(held[:i], held[i+1:]...)
			break
		}
	}
	r.bindings[contenoxSessionID] = append([]*Transport{t}, held...)
}

func (r *SessionRouter) unbind(contenoxSessionID string, t *Transport) {
	if r == nil || contenoxSessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	held := r.bindings[contenoxSessionID]
	for i, existing := range held {
		if existing == t {
			held = append(held[:i], held[i+1:]...)
			break
		}
	}
	if len(held) == 0 {
		delete(r.bindings, contenoxSessionID)
		return
	}
	r.bindings[contenoxSessionID] = held
}

// transportsFor returns a copy, so callers write to connections holding no lock.
func (r *SessionRouter) transportsFor(contenoxSessionID string) []*Transport {
	if r == nil || contenoxSessionID == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	held := r.bindings[contenoxSessionID]
	if len(held) == 0 {
		return nil
	}
	out := make([]*Transport, len(held))
	copy(out, held)
	return out
}

// transportFor returns the most recent holder, for the addresses that must
// resolve to exactly one connection.
func (r *SessionRouter) transportFor(contenoxSessionID string) (*Transport, bool) {
	held := r.transportsFor(contenoxSessionID)
	if len(held) == 0 {
		return nil, false
	}
	return held[0], true
}

// TransportResolver picks the connection a proxied tool call should act through
// for the session in ctx, or nil when no client is attached. Nil is an ordinary
// answer: the tools then fall back to the operating system.
type TransportResolver func(context.Context) *Transport

// TransportForContext resolves the connection a tool call should act through: the
// most recent transport holding the contenox session in ctx, or nil when nothing
// holds it. It takes ctx because which connection is correct depends on which
// session is running.
func (r *SessionRouter) TransportForContext(ctx context.Context) *Transport {
	if r == nil || ctx == nil {
		return nil
	}
	contenoxSessionID, _ := ctx.Value(runtimetypes.SessionIDContextKey).(string)
	t, ok := r.transportFor(contenoxSessionID)
	if !ok {
		return nil
	}
	return t
}

// askTargets selects which of a session's holders are asked to approve; it is the
// one place a destination policy belongs.
func (r *SessionRouter) askTargets(held []*Transport) []*Transport { return held }

// AskApproval drives session/request_permission on every transport holding the
// contenox session in ctx and resolves with the first real answer, cancelling the
// rest. An error is not an answer. Returns ErrNoBoundSession only when nothing
// holds the session; when every holder failed, the first error is returned.
func (r *SessionRouter) AskApproval(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
	contenoxSessionID, _ := ctx.Value(runtimetypes.SessionIDContextKey).(string)
	targets := r.askTargets(r.transportsFor(contenoxSessionID))
	if len(targets) == 0 {
		return false, ErrNoBoundSession
	}
	if len(targets) == 1 {
		return targets[0].AskApproval(ctx, req)
	}

	askCtx, cancelLosers := context.WithCancel(ctx)
	defer cancelLosers()

	type verdict struct {
		approved bool
		err      error
	}
	answers := make(chan verdict, len(targets))
	for _, target := range targets {
		go func(t *Transport) {
			approved, err := t.AskApproval(askCtx, req)
			answers <- verdict{approved: approved, err: err}
		}(target)
	}

	var firstErr error
	for range targets {
		got := <-answers
		if got.err == nil {
			return got.approved, nil
		}
		if firstErr == nil {
			firstErr = got.err
		}
	}
	return false, firstErr
}

// DeliverToContenoxSession routes an out-of-band mission report to every live
// connection holding contenoxSessionID, returning ErrSessionNotLive when none
// does. One holder succeeding is enough to call it delivered. The report is
// journaled first, so a session that is unattached still records it and a
// later resume delivers it.
func (r *SessionRouter) DeliverToContenoxSession(ctx context.Context, contenoxSessionID string, n libacp.SessionNotification) error {
	if r == nil || contenoxSessionID == "" {
		return ErrNoBoundSession
	}
	r.journalAppend(contenoxSessionID, n)
	held := r.transportsFor(contenoxSessionID)
	if len(held) == 0 {
		return ErrSessionNotLive
	}
	var firstErr error
	delivered := false
	for _, t := range held {
		if err := t.DeliverToContenoxSession(ctx, contenoxSessionID, n); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		delivered = true
	}
	if delivered {
		return nil
	}
	return firstErr
}

// mirror copies notif to every holder of contenoxSessionID except origin. It
// never blocks — each copy goes onto the target's own bounded queue — and
// delivery is best-effort, since session/load replays the durable transcript.
// Journaling already happened once at the origin (sendUpdate) or at
// DeliverToContenoxSession; mirror must not journal again or every holder
// would double the record.
func (r *SessionRouter) mirror(origin *Transport, contenoxSessionID string, notif libacp.SessionNotification) {
	if r == nil || contenoxSessionID == "" {
		return
	}
	for _, t := range r.transportsFor(contenoxSessionID) {
		if t == origin {
			continue
		}
		t.mirrorUpdate(contenoxSessionID, notif)
	}
}

// ReplaySince returns the journal entries newer than since for
// contenoxSessionID, in send order, with the session's current highest
// sequence. It is the resume answer: a holder that saw sequence `since` needs
// exactly these notifications, no more and no fewer.
func (r *SessionRouter) ReplaySince(contenoxSessionID string, since uint64) ([]JournalEntry, uint64) {
	if r == nil {
		return nil, 0
	}
	r.mu.RLock()
	journal := r.journals[contenoxSessionID]
	r.mu.RUnlock()
	if journal == nil {
		return nil, 0
	}
	return journal.After(since)
}

// journalAppend records one relayed notification under the session's next
// sequence, creating the journal on first use, and returns that sequence so
// the caller can stamp the live copy identically to the stored one. Safe for
// concurrent callers.
func (r *SessionRouter) journalAppend(contenoxSessionID string, notif libacp.SessionNotification) uint64 {
	r.mu.Lock()
	journal := r.journals[contenoxSessionID]
	if journal == nil {
		journal = NewSessionJournal(sessionJournalCapacity)
		r.journals[contenoxSessionID] = journal
	}
	r.mu.Unlock()
	seq, _ := journal.Append(notif)
	return seq
}
