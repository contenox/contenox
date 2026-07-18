package agentregistryservice

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

func setupAgentRegistryDB(t *testing.T) (context.Context, libdb.DBManager) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "agentregistryservice.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return ctx, db
}

func newExternalACPAgent(name string) *runtimetypes.Agent {
	agent := &runtimetypes.Agent{Name: name, Enabled: true}
	if err := agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   "my-acp-agent",
	}); err != nil {
		panic(err)
	}
	return agent
}

// ─── validate() table test ─────────────────────────────────────────────────

func TestUnit_Validate(t *testing.T) {
	tests := []struct {
		name    string
		agent   *runtimetypes.Agent
		wantErr bool
	}{
		{
			name:    "valid external_acp stdio agent",
			agent:   newExternalACPAgent("valid-agent"),
			wantErr: false,
		},
		{
			name:    "empty name is rejected",
			agent:   func() *runtimetypes.Agent { a := newExternalACPAgent(""); return a }(),
			wantErr: true,
		},
		{
			name:    "empty kind is rejected",
			agent:   &runtimetypes.Agent{Name: "no-kind"},
			wantErr: true,
		},
		{
			name:    "unknown kind is rejected",
			agent:   &runtimetypes.Agent{Name: "bad-kind", Kind: "not-a-real-kind"},
			wantErr: true,
		},
		{
			name:    "reserved chain kind is rejected (not implemented yet)",
			agent:   &runtimetypes.Agent{Name: "chain-agent", Kind: runtimetypes.AgentKindChain},
			wantErr: true,
		},
		{
			name: "external_acp stdio without command is rejected",
			agent: func() *runtimetypes.Agent {
				a := &runtimetypes.Agent{Name: "no-command"}
				require.NoError(t, a.SetExternalACPConfig(runtimetypes.ExternalACPConfig{Transport: runtimetypes.ExternalACPTransportStdio}))
				return a
			}(),
			wantErr: true,
		},
		{
			name: "external_acp endpoint with url is accepted",
			agent: func() *runtimetypes.Agent {
				a := &runtimetypes.Agent{Name: "endpoint-agent"}
				require.NoError(t, a.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
					Transport: runtimetypes.ExternalACPTransportEndpoint,
					URL:       "https://agent.example.com/acp",
				}))
				return a
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.agent)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ─── Service CRUD ───────────────────────────────────────────────────────────

func TestUnit_AgentRegistryService_CreateGetUpdateDelete(t *testing.T) {
	ctx, db := setupAgentRegistryDB(t)
	svc := New(db)

	agent := newExternalACPAgent("crud-agent")
	require.NoError(t, svc.Create(ctx, agent))
	require.NotEmpty(t, agent.ID)

	got, err := svc.Get(ctx, agent.ID)
	require.NoError(t, err)
	require.Equal(t, agent.Name, got.Name)
	require.Equal(t, runtimetypes.AgentKindExternalACP, got.Kind)

	byName, err := svc.GetByName(ctx, agent.Name)
	require.NoError(t, err)
	require.Equal(t, agent.ID, byName.ID)

	got.Enabled = false
	require.NoError(t, svc.Update(ctx, got))

	updated, err := svc.Get(ctx, agent.ID)
	require.NoError(t, err)
	require.False(t, updated.Enabled)

	require.NoError(t, svc.Delete(ctx, agent.ID))
	_, err = svc.Get(ctx, agent.ID)
	require.Error(t, err)
	require.True(t, errors.Is(err, libdb.ErrNotFound))
}

func TestUnit_AgentRegistryService_List(t *testing.T) {
	ctx, db := setupAgentRegistryDB(t)
	svc := New(db)

	for _, name := range []string{"list-1", "list-2", "list-3"} {
		require.NoError(t, svc.Create(ctx, newExternalACPAgent(name)))
	}

	items, err := svc.List(ctx, nil, 100)
	require.NoError(t, err)
	require.Len(t, items, 3)
}

// ─── Conflict surfacing ─────────────────────────────────────────────────────

func TestUnit_AgentRegistryService_CreateDuplicateNameConflict(t *testing.T) {
	ctx, db := setupAgentRegistryDB(t)
	svc := New(db)

	first := newExternalACPAgent("duplicate-name")
	require.NoError(t, svc.Create(ctx, first))

	second := newExternalACPAgent("duplicate-name")
	err := svc.Create(ctx, second)
	require.Error(t, err)
	require.True(t, errors.Is(err, libdb.ErrUniqueViolation), "duplicate name must surface as a conflict, got: %v", err)
}

func TestUnit_AgentRegistryService_UpdateToExistingNameConflict(t *testing.T) {
	ctx, db := setupAgentRegistryDB(t)
	svc := New(db)

	first := newExternalACPAgent("taken-name")
	require.NoError(t, svc.Create(ctx, first))

	second := newExternalACPAgent("rename-me")
	require.NoError(t, svc.Create(ctx, second))

	second.Name = "taken-name"
	err := svc.Update(ctx, second)
	require.Error(t, err)
	require.True(t, errors.Is(err, libdb.ErrUniqueViolation))
}

func TestUnit_AgentRegistryService_UpdateKeepingOwnNameIsNotConflict(t *testing.T) {
	ctx, db := setupAgentRegistryDB(t)
	svc := New(db)

	agent := newExternalACPAgent("keep-my-name")
	require.NoError(t, svc.Create(ctx, agent))

	agent.Enabled = false
	require.NoError(t, svc.Update(ctx, agent), "updating other fields while keeping the same name must not be a conflict")
}

func TestUnit_AgentRegistryService_CreateRejectsChainKind(t *testing.T) {
	ctx, db := setupAgentRegistryDB(t)
	svc := New(db)

	err := svc.Create(ctx, &runtimetypes.Agent{Name: "future-chain-agent", Kind: runtimetypes.AgentKindChain})
	require.Error(t, err)
}

func TestUnit_AgentRegistryService_GetRequiresID(t *testing.T) {
	ctx, db := setupAgentRegistryDB(t)
	svc := New(db)

	_, err := svc.Get(ctx, "")
	require.Error(t, err)
}

func TestUnit_AgentRegistryService_CreateAssignsID(t *testing.T) {
	ctx, db := setupAgentRegistryDB(t)
	svc := New(db)

	agent := newExternalACPAgent("auto-id")
	agent.ID = ""
	require.NoError(t, svc.Create(ctx, agent))
	require.NotEmpty(t, agent.ID)
	_, err := uuid.Parse(agent.ID)
	require.NoError(t, err)
}
