package missionservice

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func setupMissionDB(t *testing.T) (context.Context, libdb.DBManager) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "missionservice.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return ctx, db
}

func newMission(intent string) *Mission {
	return &Mission{Intent: intent, AgentName: "runner"}
}

// ─── validate() table test ─────────────────────────────────────────────────

func TestUnit_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mission *Mission
		wantErr bool
	}{
		{name: "valid open mission", mission: &Mission{Intent: "ship the board", Status: StatusOpen}},
		{name: "valid landed mission", mission: &Mission{Intent: "ship the board", Status: StatusLanded}},
		{name: "empty intent is rejected", mission: &Mission{Intent: "", Status: StatusOpen}, wantErr: true},
		{name: "whitespace intent is rejected", mission: &Mission{Intent: "   ", Status: StatusOpen}, wantErr: true},
		{name: "multi-line intent is rejected", mission: &Mission{Intent: "line one\nline two", Status: StatusOpen}, wantErr: true},
		{name: "unknown status is rejected", mission: &Mission{Intent: "ok", Status: "bogus"}, wantErr: true},
		{name: "empty status is rejected", mission: &Mission{Intent: "ok", Status: ""}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.mission)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ─── Create / lifecycle ─────────────────────────────────────────────────────

func TestUnit_MissionService_CreateAssignsIDAndOpenStatus(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	m := newMission("ship the fleet board")
	m.Status = StatusLanded // must be forced back to open on create
	require.NoError(t, svc.Create(ctx, m))

	require.NotEmpty(t, m.ID)
	_, err := uuid.Parse(m.ID)
	require.NoError(t, err)
	require.Equal(t, StatusOpen, m.Status)
	require.False(t, m.CreatedAt.IsZero())
	require.Equal(t, m.CreatedAt, m.UpdatedAt)
	require.NotNil(t, m.SessionIDs)
	require.NotNil(t, m.InstanceIDs)
}

func TestUnit_MissionService_CreateRejectsInvalidIntent(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	require.Error(t, svc.Create(ctx, newMission("")))
	require.Error(t, svc.Create(ctx, newMission("two\nlines")))
}

func TestUnit_MissionService_CreateGetUpdateDelete(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	m := newMission("crud mission")
	require.NoError(t, svc.Create(ctx, m))

	got, err := svc.Get(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, m.Intent, got.Intent)
	require.Equal(t, "runner", got.AgentName)
	require.Equal(t, StatusOpen, got.Status)

	got.Intent = "crud mission (edited)"
	require.NoError(t, svc.Update(ctx, got))

	updated, err := svc.Get(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, "crud mission (edited)", updated.Intent)
	require.True(t, updated.CreatedAt.Equal(m.CreatedAt), "update must preserve createdAt")

	require.NoError(t, svc.Delete(ctx, m.ID))
	_, err = svc.Get(ctx, m.ID)
	require.Error(t, err)
	require.True(t, errors.Is(err, libdb.ErrNotFound))
}

func TestUnit_MissionService_List(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	for _, intent := range []string{"mission-1", "mission-2", "mission-3"} {
		require.NoError(t, svc.Create(ctx, newMission(intent)))
	}

	items, err := svc.List(ctx, nil, 100)
	require.NoError(t, err)
	require.Len(t, items, 3)
}

func TestUnit_MissionService_ListEmptyIsNonNil(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	items, err := svc.List(ctx, nil, 100)
	require.NoError(t, err)
	require.NotNil(t, items)
	require.Empty(t, items)
}

// A mission outlives the sessions it referenced: it is never deleted on session
// teardown here, so it remains listed and open.
func TestUnit_MissionService_MissionOutlivesSessionsAndStaysListed(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	m := newMission("long-running mission")
	require.NoError(t, svc.Create(ctx, m))
	_, err := svc.Bind(ctx, m.ID, "session-gone", "")
	require.NoError(t, err)

	items, err := svc.List(ctx, nil, 100)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, StatusOpen, items[0].Status)
}

// ─── Status transitions ─────────────────────────────────────────────────────

func TestUnit_MissionService_UpdateStatusTransitions(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	m := newMission("transition mission")
	require.NoError(t, svc.Create(ctx, m))

	for _, status := range []Status{StatusLanded, StatusDerailed, StatusAbandoned, StatusOpen} {
		m.Status = status
		require.NoError(t, svc.Update(ctx, m), "status %q must be accepted", status)
		got, err := svc.Get(ctx, m.ID)
		require.NoError(t, err)
		require.Equal(t, status, got.Status)
	}
}

func TestUnit_MissionService_UpdateRejectsUnknownStatus(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	m := newMission("bad-status mission")
	require.NoError(t, svc.Create(ctx, m))

	m.Status = "bogus"
	require.Error(t, svc.Update(ctx, m))
}

func TestUnit_MissionService_UpdateUnknownReturnsNotFound(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	orphan := &Mission{ID: "no-such-id", Intent: "ghost", Status: StatusOpen}
	err := svc.Update(ctx, orphan)
	require.Error(t, err)
	require.True(t, errors.Is(err, libdb.ErrNotFound))
}

func TestUnit_MissionService_GetUnknownReturnsNotFound(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	_, err := svc.Get(ctx, "no-such-id")
	require.Error(t, err)
	require.True(t, errors.Is(err, libdb.ErrNotFound))
}

// ─── Bind ───────────────────────────────────────────────────────────────────

func TestUnit_MissionService_BindAppendsAndDedups(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	m := newMission("bind mission")
	require.NoError(t, svc.Create(ctx, m))

	bound, err := svc.Bind(ctx, m.ID, "session-1", "instance-1")
	require.NoError(t, err)
	require.Equal(t, []string{"session-1"}, bound.SessionIDs)
	require.Equal(t, []string{"instance-1"}, bound.InstanceIDs)

	bound, err = svc.Bind(ctx, m.ID, "session-2", "")
	require.NoError(t, err)
	require.Equal(t, []string{"session-1", "session-2"}, bound.SessionIDs)

	// Re-binding an already-present id is a no-op.
	bound, err = svc.Bind(ctx, m.ID, "session-1", "instance-1")
	require.NoError(t, err)
	require.Equal(t, []string{"session-1", "session-2"}, bound.SessionIDs)
	require.Equal(t, []string{"instance-1"}, bound.InstanceIDs)

	persisted, err := svc.Get(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"session-1", "session-2"}, persisted.SessionIDs)
	require.Equal(t, []string{"instance-1"}, persisted.InstanceIDs)
}

func TestUnit_MissionService_BindRequiresAnID(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	m := newMission("no-op bind")
	require.NoError(t, svc.Create(ctx, m))

	_, err := svc.Bind(ctx, m.ID, "", "")
	require.Error(t, err)
}

func TestUnit_MissionService_BindUnknownReturnsNotFound(t *testing.T) {
	ctx, db := setupMissionDB(t)
	svc := New(db)

	_, err := svc.Bind(ctx, "no-such-id", "session-1", "")
	require.Error(t, err)
	require.True(t, errors.Is(err, libdb.ErrNotFound))
}
