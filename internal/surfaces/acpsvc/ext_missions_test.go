package acpsvc

import (
	"encoding/json"
	"testing"
	"time"

	libacp "github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

func TestExtMissions_UnwiredDepsAreMethodNotFound(t *testing.T) {
	tr := &Transport{deps: Deps{}}
	cases := []struct {
		name   string
		handle func(*Transport) (json.RawMessage, *libacp.Error)
	}{
		{"missions/list", func(tr *Transport) (json.RawMessage, *libacp.Error) { return tr.handleMissionsList(t.Context(), nil) }},
		{"missions/get", func(tr *Transport) (json.RawMessage, *libacp.Error) { return tr.handleMissionsGet(t.Context(), nil) }},
		{"missions/reports", func(tr *Transport) (json.RawMessage, *libacp.Error) {
			return tr.handleMissionsReports(t.Context(), nil)
		}},
		{"inbox/list", func(tr *Transport) (json.RawMessage, *libacp.Error) { return tr.handleInboxList(t.Context(), nil) }},
	}
	for _, c := range cases {
		_, rpcErr := c.handle(tr)
		require.NotNil(t, rpcErr, c.name)
		require.Equal(t, libacp.ErrMethodNotFound, rpcErr.Code, c.name)
	}
}

func TestParseMissionCursor(t *testing.T) {
	c, err := parseMissionCursor("")
	require.NoError(t, err)
	require.Nil(t, c)

	c, err = parseMissionCursor("2026-09-08T10:00:00Z")
	require.NoError(t, err)
	require.NotNil(t, c)
	require.True(t, c.Equal(time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)))

	_, err = parseMissionCursor("not-a-time")
	require.Error(t, err)
}
