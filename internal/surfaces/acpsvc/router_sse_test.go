package acpsvc

import (
	"testing"

	"github.com/contenox/contenox/libacp"
)

func textNotif(sid string, text string) libacp.SessionNotification {
	content := libacp.NewTextContent(text)
	return libacp.SessionNotification{
		SessionID: libacp.SessionID(sid),
		Update: libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateAgentMessageChunk,
			Content:       &content,
		},
	}
}

// The classic SSE failure mode end to end, at the router seam: events stream
// to a live client, the client drops, events keep flowing, the client
// reconnects naming its last sequence, and it receives exactly the missed
// events — none lost, none duplicated — because replay is cursor-bounded.
func TestUnit_SessionRouter_ResumeAcrossDropLosesNothingAndDuplicatesNothing(t *testing.T) {
	r := NewSessionRouter()
	const sid = "s"

	// Live phase: the client is current through sequence 3.
	r.journalAppend(sid, textNotif(sid, "one"))
	r.journalAppend(sid, textNotif(sid, "two"))
	r.journalAppend(sid, textNotif(sid, "three"))

	// The client drops here. The session keeps producing.
	r.journalAppend(sid, textNotif(sid, "four"))
	r.journalAppend(sid, textNotif(sid, "five"))

	// Reconnect: the client names its last seen sequence (3) and must be
	// answered with exactly the missed notifications (4, 5), nothing earlier.
	missed, last := r.ReplaySince(sid, 3)
	if last != 5 {
		t.Fatalf("high-water after resume = %d, want 5", last)
	}
	if len(missed) != 2 {
		t.Fatalf("resume after seq 3 returned %d events, want exactly the 2 missed", len(missed))
	}
	for i, entry := range missed {
		if want := uint64(i + 4); entry.Seq != want {
			t.Fatalf("resumed event %d has seq %d, want %d (no loss, no replay of held events)", i, entry.Seq, want)
		}
	}

	// The client applies 4 and 5 and advances its cursor to 5; asking again
	// for anything newer must be empty — replaying is idempotent.
	if dup, _ := r.ReplaySince(sid, 5); len(dup) != 0 {
		t.Fatalf("replay at current cursor returned %d duplicates", len(dup))
	}

	// A fresh attach with no prior state replays the whole journal in order.
	all, _ := r.ReplaySince(sid, 0)
	if len(all) != 5 {
		t.Fatalf("fresh attach got %d events, want all 5", len(all))
	}
	for i := 1; i <= 5; i++ {
		if all[i-1].Seq != uint64(i) {
			t.Fatalf("fresh-attach event %d seq = %d, want %d", i-1, all[i-1].Seq, i)
		}
	}
}
