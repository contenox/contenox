package acpsvc

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/contenox/contenox/libacp"
)

func sampleNotification(sessionID, toolCallID string, status libacp.ToolCallStatus) libacp.SessionNotification {
	return libacp.SessionNotification{
		SessionID: libacp.SessionID(sessionID),
		Update: libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateToolCallUpdate,
			ToolCallID:    toolCallID,
			Status:        status,
		},
	}
}

func TestUnit_Journal_AssignsMonotonicGapFreeSequences(t *testing.T) {
	j := NewSessionJournal(0)
	for want := uint64(1); want <= 3; want++ {
		got, err := j.Append(sampleNotification("s", "c", libacp.ToolCallStatusInProgress))
		if err != nil {
			t.Fatalf("append %d: %v", want, err)
		}
		if got != want {
			t.Fatalf("append returned seq %d, want %d", got, want)
		}
	}
	if last := j.LastSeq(); last != 3 {
		t.Fatalf("LastSeq = %d, want 3", last)
	}
}

func TestUnit_Journal_AfterReturnsExactlyTheNewerEntries(t *testing.T) {
	j := NewSessionJournal(0)
	for i := 1; i <= 5; i++ {
		_, _ = j.Append(sampleNotification("s", "c", libacp.ToolCallStatusInProgress))
	}
	entries, last := j.After(2)
	if last != 5 {
		t.Fatalf("After returned last %d, want 5", last)
	}
	if len(entries) != 3 {
		t.Fatalf("After(2) returned %d entries, want 3", len(entries))
	}
	for i, entry := range entries {
		if want := uint64(i + 3); entry.Seq != want {
			t.Fatalf("entry %d seq = %d, want %d", i, entry.Seq, want)
		}
	}
	if got, _ := j.After(5); len(got) != 0 {
		t.Fatalf("After(5) returned %d entries, want 0 (nothing newer)", len(got))
	}
	if all, _ := j.After(0); len(all) != 5 {
		t.Fatalf("After(0) returned %d entries, want all 5", len(all))
	}
}

func TestUnit_Journal_BoundedCapacityRetainsOnlyTheNewest(t *testing.T) {
	j := NewSessionJournal(2)
	for i := 1; i <= 4; i++ {
		_, _ = j.Append(sampleNotification("s", "c", libacp.ToolCallStatusInProgress))
	}
	entries, last := j.After(0)
	if last != 4 {
		t.Fatalf("After last = %d, want 4 (sequences keep counting past trimmed entries)", last)
	}
	if len(entries) != 2 {
		t.Fatalf("bounded journal retained %d entries, want newest 2", len(entries))
	}
	if entries[0].Seq != 3 || entries[1].Seq != 4 {
		t.Fatalf("retained seqs = %d,%d, want 3,4", entries[0].Seq, entries[1].Seq)
	}
}

func TestUnit_Journal_ReplayIsWireFaithful(t *testing.T) {
	j := NewSessionJournal(0)
	sent := sampleNotification("s", "c", libacp.ToolCallStatusCompleted)
	_, err := j.Append(sent)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	// Mutating the caller's copy after append must not reach the journal.
	sent.Update.Status = libacp.ToolCallStatusFailed
	entries, _ := j.After(0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	replayed := entries[0].Notification
	want := sampleNotification("s", "c", libacp.ToolCallStatusCompleted).WithSeq(1)
	if !reflect.DeepEqual(replayed, want) {
		t.Fatalf("replayed notification drifted from what was sent: %+v", replayed)
	}
	// The replayed notification carries its cursor: a client that applies it
	// can name its last seen sequence on reconnect.
	if !bytes.Contains(replayed.Meta, []byte(`"contenox.seq":1`)) {
		t.Fatalf("replayed notification lacks its sequence in _meta: %s", replayed.Meta)
	}
}

func TestUnit_Journal_EmptyJournal(t *testing.T) {
	j := NewSessionJournal(0)
	if j.LastSeq() != 0 {
		t.Fatalf("empty LastSeq = %d, want 0", j.LastSeq())
	}
	if entries, _ := j.After(0); len(entries) != 0 {
		t.Fatalf("empty After returned %d entries", len(entries))
	}
}
