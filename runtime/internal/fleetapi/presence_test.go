package fleetapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/contenox/runtime/runtime/presence"
	"github.com/stretchr/testify/require"
)

type fakePresence struct {
	entries []presence.Entry
	err     error
}

func (f *fakePresence) List(context.Context) ([]presence.Entry, error) {
	return f.entries, f.err
}

var _ PresenceReader = (*fakePresence)(nil)

func setupPresenceAPI(t *testing.T, reader PresenceReader) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	AddPresenceRoutes(mux, reader)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUnit_FleetAPI_PresenceListReturnsObservedInstances(t *testing.T) {
	started := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	reader := &fakePresence{entries: []presence.Entry{
		{
			Record: presence.Record{
				InstanceID:   "acp-1",
				Kind:         presence.KindACP,
				PID:          4242,
				StartedAt:    started,
				LastSeen:     started.Add(5 * time.Second),
				Cwd:          "/home/dev/project",
				SessionCount: 2,
				ClientName:   "zed",
			},
			External: true,
			Stale:    false,
		},
		{
			Record: presence.Record{
				InstanceID: "code-1",
				Kind:       presence.KindVSCodeAgent,
				StartedAt:  started,
				LastSeen:   started,
			},
			External: true,
			Stale:    true,
		},
	}}

	srv := setupPresenceAPI(t, reader)
	resp, err := http.Get(srv.URL + "/fleet/presence")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []presence.Entry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 2)

	require.Equal(t, "acp-1", got[0].InstanceID)
	require.Equal(t, presence.KindACP, got[0].Kind)
	require.Equal(t, "zed", got[0].ClientName)
	require.Equal(t, 2, got[0].SessionCount)
	require.True(t, got[0].External, "presence entries must be marked external")
	require.False(t, got[0].Stale)

	require.Equal(t, presence.KindVSCodeAgent, got[1].Kind)
	require.True(t, got[1].Stale, "an aged editor must surface stale:true")
}

// The presence entry's wire shape is what the Beam follow-up consumes; pin the
// exact JSON keys so a rename here is caught, not discovered downstream.
func TestUnit_FleetAPI_PresenceEntryWireShape(t *testing.T) {
	started := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	reader := &fakePresence{entries: []presence.Entry{{
		Record: presence.Record{
			InstanceID: "acp-1",
			Kind:       presence.KindACP,
			PID:        4242,
			Host:       "workstation",
			StartedAt:  started,
			LastSeen:   started,
			ClientName: "zed",
		},
		External: true,
		Stale:    false,
	}}}
	srv := setupPresenceAPI(t, reader)
	resp, err := http.Get(srv.URL + "/fleet/presence")
	require.NoError(t, err)
	defer resp.Body.Close()

	var raw []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
	require.Len(t, raw, 1)
	e := raw[0]
	for _, key := range []string{"instanceId", "kind", "pid", "host", "startedAt", "lastSeen", "clientName", "sessionCount", "external", "stale"} {
		_, ok := e[key]
		require.Truef(t, ok, "presence entry wire shape must carry %q", key)
	}
	require.Equal(t, true, e["external"])
}

func TestUnit_FleetAPI_PresenceListError(t *testing.T) {
	srv := setupPresenceAPI(t, &fakePresence{err: errors.New("kv down")})
	resp, err := http.Get(srv.URL + "/fleet/presence")
	require.NoError(t, err)
	defer resp.Body.Close()
	// A reader failure is surfaced as an error status (mapped by apiframework),
	// never a misleading 200 with an empty board.
	require.GreaterOrEqual(t, resp.StatusCode, 400)
	require.NotEqual(t, http.StatusOK, resp.StatusCode)
}
