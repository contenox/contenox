package libacp

import "context"

// FilterSessionUpdates wraps a Client so that session/update notifications for
// any session other than live are dropped before reaching inner.SessionUpdate;
// every other Client method passes straight through. It is opt-in middleware: a
// ClientSideConnection forwards every session/update regardless of session id
// (correct at the library layer — the app owns session bookkeeping), so a
// driver that reconnects or swaps sessions must filter stale updates itself, or
// a just-abandoned session's chunks leak into the new turn's UI. hash learned
// this the hard way and filters inline (acp.go:1504); this wrapper lets future
// consumers inherit the guard instead of re-learning it.
//
// Wrap the Client the ClientFactory returns; update live (by constructing a new
// wrapper) whenever the driver's active session changes.
func FilterSessionUpdates(live SessionID, inner Client) Client {
	return sessionUpdateFilter{Client: inner, live: live}
}

// sessionUpdateFilter embeds Client so all non-overridden methods (permission,
// fs, terminal) forward unchanged; only SessionUpdate gains the session-id gate.
type sessionUpdateFilter struct {
	Client
	live SessionID
}

func (f sessionUpdateFilter) SessionUpdate(ctx context.Context, n SessionNotification) error {
	if n.SessionID != f.live {
		return nil
	}
	return f.Client.SessionUpdate(ctx, n)
}
