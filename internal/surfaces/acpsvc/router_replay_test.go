package acpsvc

import (
	"testing"

	"github.com/contenox/contenox/libacp"
)

// mirror journals even with no other holder, so a reconnecting sole client
// finds the gap it missed. origin nil is fine for that path.
func TestUnit_SessionRouter_JournalAndReplaySince(t *testing.T) {
	r := NewSessionRouter()
	notif := func(i int) libacp.SessionNotification {
		content := libacp.NewTextContent(string(rune('a' + i)))
		return libacp.SessionNotification{
			SessionID: libacp.SessionID("s"),
			Update: libacp.SessionUpdate{
				SessionUpdate: libacp.SessionUpdateAgentMessageChunk,
				Content:       &content,
			},
		}
	}
	r.journalAppend("s", notif(0))
	r.journalAppend("s", notif(1))
	r.journalAppend("s", notif(2))

	if entries, last := r.ReplaySince("s", 1); len(entries) != 2 || last != 3 {
		t.Fatalf("ReplaySince(1) = %d entries, last %d; want 2, 3", len(entries), last)
	} else if entries[0].Seq != 2 || entries[1].Seq != 3 {
		t.Fatalf("ReplaySince(1) seqs = %d,%d; want 2,3", entries[0].Seq, entries[1].Seq)
	}
	if entries, _ := r.ReplaySince("s", 3); len(entries) != 0 {
		t.Fatalf("ReplaySince(3) returned %d entries; want none newer", len(entries))
	}
	// The replay is wire-faithful: the notification decodes to what was sent.
	if entries, _ := r.ReplaySince("s", 0); len(entries) != 3 || entries[2].Notification.Update.Content == nil {
		t.Fatalf("ReplaySince(0) not wire-faithful: %d entries", len(entries))
	}
	if entries, last := r.ReplaySince("unknown", 0); len(entries) != 0 || last != 0 {
		t.Fatalf("ReplaySince for unknown session = %d entries, last %d", len(entries), last)
	}
}
