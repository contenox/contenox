package reportrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/libacp"
	libbus "github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/operatorinbox"
	"github.com/stretchr/testify/require"
)

type fakeDeliverer struct {
	mu       sync.Mutex
	sessions []libacp.SessionID
	notes    []libacp.SessionNotification
	err      error
}

func (f *fakeDeliverer) DeliverToSession(_ context.Context, sid libacp.SessionID, n libacp.SessionNotification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = append(f.sessions, sid)
	f.notes = append(f.notes, n)
	return f.err
}

func (f *fakeDeliverer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sessions)
}

type fakeInbox struct {
	mu    sync.Mutex
	items []*operatorinbox.Item
	err   error
}

func (f *fakeInbox) Add(_ context.Context, item *operatorinbox.Item) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.items = append(f.items, item)
	return nil
}

func (f *fakeInbox) list() []*operatorinbox.Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*operatorinbox.Item(nil), f.items...)
}

func newTestRouter(t *testing.T, del *fakeDeliverer, inbox *fakeInbox) *Router {
	t.Helper()
	r, err := New(Deps{Bus: libbus.NewInMem(), Sessions: del, Inbox: inbox})
	require.NoError(t, err)
	return r
}

func resultEvent(parentSessionID string) missionservice.ReportAddedEvent {
	return missionservice.ReportAddedEvent{
		MissionID:       "m1",
		ParentSessionID: parentSessionID,
		AgentName:       "runner",
		Intent:          "do the thing",
		Report: missionservice.Report{
			ID: "r1", MissionID: "m1", Kind: missionservice.ReportKindResult,
			Summary: "shipped the board", Detail: "all green", Refs: []string{"a.txt", "b.txt"},
		},
	}
}

// TestUnit_Route_ParentSessionDelivered: with a live parent session, the report
// is delivered into it and nothing lands in the inbox.
func TestUnit_Route_ParentSessionDelivered(t *testing.T) {
	del := &fakeDeliverer{}
	inbox := &fakeInbox{}
	r := newTestRouter(t, del, inbox)

	r.route(context.Background(), resultEvent("parent-42"))

	require.Equal(t, 1, del.count(), "the report is delivered to the parent session")
	require.Equal(t, libacp.SessionID("parent-42"), del.sessions[0])
	require.Empty(t, inbox.list(), "a delivered report does not also land in the inbox")

	// The delivered update is a transcript-legible agent_message_chunk carrying
	// the mission-report attribution in its _meta envelope.
	n := del.notes[0]
	require.Equal(t, libacp.SessionUpdateAgentMessageChunk, n.Update.SessionUpdate)
	require.NotNil(t, n.Update.Content)
	require.Contains(t, n.Update.Content.Text, "unit runner reported (result): shipped the board")
	require.Contains(t, n.Update.Content.Text, "all green")
	require.Contains(t, n.Update.Content.Text, "refs: a.txt, b.txt")

	var meta reportUpdateMeta
	require.NoError(t, json.Unmarshal(n.Update.Meta, &meta))
	require.NotNil(t, meta.Report)
	require.Equal(t, "m1", meta.Report.MissionID)
	require.Equal(t, "r1", meta.Report.ReportID)
	require.Equal(t, "result", meta.Report.Kind)
}

// TestUnit_Route_ParentGoneFallsBackToInbox: a named parent that cannot be
// reached (deliverer errors) falls back to the inbox marked parent_gone. A
// supervisor ending must never drop a report.
func TestUnit_Route_ParentGoneFallsBackToInbox(t *testing.T) {
	del := &fakeDeliverer{err: fmt.Errorf("agentinstance: session %q: not found", "parent-42")}
	inbox := &fakeInbox{}
	r := newTestRouter(t, del, inbox)

	r.route(context.Background(), resultEvent("parent-42"))

	require.Equal(t, 1, del.count(), "delivery was attempted")
	items := inbox.list()
	require.Len(t, items, 1, "an undeliverable report falls back to the inbox, never dropped")
	require.Equal(t, operatorinbox.ReasonParentGone, items[0].Reason)
	require.Equal(t, "parent-42", items[0].ParentSessionID, "the intended-but-missed supervisor is recorded")
	require.Equal(t, "shipped the board", items[0].Report.Summary)
}

// TestUnit_Route_OperatorFiredToInbox: no parent session → straight to the
// operator inbox, never a delivery attempt.
func TestUnit_Route_OperatorFiredToInbox(t *testing.T) {
	del := &fakeDeliverer{}
	inbox := &fakeInbox{}
	r := newTestRouter(t, del, inbox)

	r.route(context.Background(), resultEvent(""))

	require.Equal(t, 0, del.count(), "no supervisor session → no delivery attempt")
	items := inbox.list()
	require.Len(t, items, 1)
	require.Equal(t, operatorinbox.ReasonOperatorFired, items[0].Reason)
	require.Empty(t, items[0].ParentSessionID)
	require.Equal(t, "m1", items[0].MissionID)
}

// TestUnit_Route_InboxWriteFailureNeverPanics: an inbox write failure is
// tolerated (tracked, not crashed) — routing is best-effort.
func TestUnit_Route_InboxWriteFailureNeverPanics(t *testing.T) {
	del := &fakeDeliverer{}
	inbox := &fakeInbox{err: fmt.Errorf("store down")}
	r := newTestRouter(t, del, inbox)

	require.NotPanics(t, func() { r.route(context.Background(), resultEvent("")) })
}

// TestUnit_New_RequiresCollaborators proves the wiring guards.
func TestUnit_New_RequiresCollaborators(t *testing.T) {
	_, err := New(Deps{Sessions: &fakeDeliverer{}, Inbox: &fakeInbox{}})
	require.Error(t, err, "Bus is required")
	_, err = New(Deps{Bus: libbus.NewInMem(), Inbox: &fakeInbox{}})
	require.Error(t, err, "Sessions is required")
	_, err = New(Deps{Bus: libbus.NewInMem(), Sessions: &fakeDeliverer{}})
	require.Error(t, err, "Inbox is required")
}

// TestUnit_StartConsumesBusEvents drives the full loop: an event published on
// the bus after Start is decoded and routed. Uses the in-memory bus, so it is
// fast and needs no subprocess.
func TestUnit_StartConsumesBusEvents(t *testing.T) {
	del := &fakeDeliverer{}
	inbox := &fakeInbox{}
	bus := libbus.NewInMem()
	r, err := New(Deps{Bus: bus, Sessions: del, Inbox: inbox})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := r.Start(ctx)
	require.NoError(t, err)
	defer stop()

	data, err := json.Marshal(resultEvent("parent-42"))
	require.NoError(t, err)
	require.NoError(t, bus.Publish(ctx, missionservice.ReportAddedSubject, data))

	require.Eventually(t, func() bool { return del.count() == 1 }, 2*time.Second, 10*time.Millisecond,
		"the router consumes the published event and delivers it")
	require.Equal(t, libacp.SessionID("parent-42"), del.sessions[0])
}
