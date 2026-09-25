package acpsvc

import (
	"encoding/json"
	"sync"

	"github.com/contenox/contenox/libacp"
)

// JournalEntry is one recorded wire notification and the session's monotonic
// sequence it was sent under.
type JournalEntry struct {
	// Seq is the session-local sequence: strictly increasing, 1-based, gap-free.
	Seq uint64 `json:"seq"`
	// Notification is the notification exactly as it crossed the wire.
	Notification libacp.SessionNotification `json:"notification"`
}

// SessionJournal is a session's record of every notification this runtime
// sent, in send order, each under the session's monotonic sequence — the SSE
// event log of a session. A reconnecting client names the highest sequence it
// holds and is answered with exactly the entries after it, so a disconnect
// cannot lose an event and a replay cannot duplicate one.
//
// Entries are stored as their marshalled wire form, so a replay is
// byte-faithful: what the journal returns is what went out, not a field-by-
// field reconstruction of it (the lossy-replay class of bug this replaces).
//
// Append is safe for concurrent callers; After/LastSeq are safe alongside it.
type SessionJournal struct {
	mu      sync.Mutex
	entries []journalWireEntry
	next    uint64
	// cap bounds retained entries to the newest cap; 0 is unbounded.
	cap int
}

type journalWireEntry struct {
	seq  uint64
	wire json.RawMessage
}

// NewSessionJournal returns an empty journal retaining up to capacity
// entries (0 = unbounded). Capacity trims from the oldest end.
func NewSessionJournal(capacity int) *SessionJournal {
	return &SessionJournal{cap: capacity}
}

// Append records notification under the next sequence and returns that
// sequence and any marshalling error (a notification that cannot cross the
// wire cannot enter the journal). The stored form carries the sequence in its
// `_meta` (WithSeq), so a replay delivers the cursor with the event.
func (j *SessionJournal) Append(notification libacp.SessionNotification) (uint64, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.next++
	stamped := notification.WithSeq(j.next)
	wire, err := json.Marshal(stamped)
	if err != nil {
		j.next--
		return 0, err
	}
	entry := journalWireEntry{seq: j.next, wire: wire}
	if j.cap > 0 && len(j.entries) == j.cap {
		// Retain the newest capacity entries: drop the oldest.
		copy(j.entries, j.entries[1:])
		j.entries[len(j.entries)-1] = entry
	} else {
		j.entries = append(j.entries, entry)
	}
	return j.next, nil
}

// LastSeq is the highest sequence recorded, or 0 when nothing has been sent.
func (j *SessionJournal) LastSeq() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.next
}

// After returns the entries whose sequence is greater than since, in send
// order, with the session's current highest sequence. An entry that was
// trimmed by capacity is gone even from here: since should name a sequence
// the client saw, which for a capacity-bounded journal must be recent enough.
func (j *SessionJournal) After(since uint64) ([]JournalEntry, uint64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []JournalEntry
	for _, entry := range j.entries {
		if entry.seq > since {
			var notif libacp.SessionNotification
			if err := json.Unmarshal(entry.wire, &notif); err != nil {
				continue
			}
			out = append(out, JournalEntry{Seq: entry.seq, Notification: notif})
		}
	}
	return out, j.next
}
