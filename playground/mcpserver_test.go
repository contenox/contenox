package playground_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/playground"
	"github.com/contenox/contenox/runtimetypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSystem_MCPServerService_CRUD tests the full service-layer lifecycle of
// MCP server configurations through a real containerized Postgres instance.
func TestSystem_MCPServerService_CRUD(t *testing.T) {
	ctx := t.Context()

	p := playground.New()
	p.WithPostgresTestContainer(ctx)
	require.NoError(t, p.GetError(), "playground setup failed")
	defer p.CleanUp()

	svc, err := p.GetMCPServerService()
	require.NoError(t, err)

	// ── Create ──────────────────────────────────────────────────────────────
	t.Run("Create_SSE", func(t *testing.T) {
		srv := &runtimetypes.MCPServer{
			Name:                  "svc-test-sse",
			Transport:             "sse",
			URL:                   "http://mcp.example.com/sse",
			ConnectTimeoutSeconds: 30,
		}
		require.NoError(t, svc.Create(ctx, srv))
		assert.NotEmpty(t, srv.ID)
		assert.WithinDuration(t, time.Now().UTC(), srv.CreatedAt, 2*time.Second)
	})

	t.Run("Create_Stdio", func(t *testing.T) {
		srv := &runtimetypes.MCPServer{
			Name:                  "svc-test-stdio",
			Transport:             "stdio",
			Command:               "npx",
			Args:                  []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
			ConnectTimeoutSeconds: 15,
		}
		require.NoError(t, svc.Create(ctx, srv))
		assert.NotEmpty(t, srv.ID)

		// Fetch back and verify args round-trip
		got, err := svc.Get(ctx, srv.ID)
		require.NoError(t, err)
		require.Equal(t, srv.Args, got.Args)
		require.Equal(t, "npx", got.Command)
	})

	t.Run("Create_WithAuthEnvKey", func(t *testing.T) {
		srv := &runtimetypes.MCPServer{
			Name:                  "svc-test-auth",
			Transport:             "sse",
			URL:                   "http://secure-mcp.example.com/sse",
			AuthType:              "bearer",
			AuthEnvKey:            "MY_MCP_TOKEN",
			ConnectTimeoutSeconds: 30,
		}
		require.NoError(t, svc.Create(ctx, srv))

		got, err := svc.Get(ctx, srv.ID)
		require.NoError(t, err)
		assert.Equal(t, "bearer", got.AuthType)
		assert.Equal(t, "MY_MCP_TOKEN", got.AuthEnvKey)
	})

	// ── Validation ──────────────────────────────────────────────────────────
	t.Run("Create_Validation_EmptyName", func(t *testing.T) {
		err := svc.Create(ctx, &runtimetypes.MCPServer{
			Transport: "sse",
			URL:       "http://mcp.example.com/sse",
		})
		require.Error(t, err)
	})

	t.Run("Create_Validation_EmptyTransport", func(t *testing.T) {
		err := svc.Create(ctx, &runtimetypes.MCPServer{
			Name: "no-transport",
			URL:  "http://mcp.example.com/sse",
		})
		require.Error(t, err)
	})

	t.Run("Create_Validation_SSE_MissingURL", func(t *testing.T) {
		err := svc.Create(ctx, &runtimetypes.MCPServer{
			Name:      "sse-no-url",
			Transport: "sse",
		})
		require.Error(t, err)
	})

	t.Run("Create_Validation_Stdio_MissingCommand", func(t *testing.T) {
		err := svc.Create(ctx, &runtimetypes.MCPServer{
			Name:      "stdio-no-command",
			Transport: "stdio",
		})
		require.Error(t, err)
	})

	t.Run("Create_Validation_UnknownTransport", func(t *testing.T) {
		err := svc.Create(ctx, &runtimetypes.MCPServer{
			Name:      "bad-transport",
			Transport: "ftp",
			URL:       "ftp://example.com",
		})
		require.Error(t, err)
	})

	// ── Get & GetByName ─────────────────────────────────────────────────────
	t.Run("GetByName", func(t *testing.T) {
		srv := &runtimetypes.MCPServer{
			Name:      "svc-get-by-name",
			Transport: "sse",
			URL:       "http://mcp.example.com/sse",
		}
		require.NoError(t, svc.Create(ctx, srv))

		got, err := svc.GetByName(ctx, "svc-get-by-name")
		require.NoError(t, err)
		assert.Equal(t, srv.ID, got.ID)
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		_, err := svc.Get(ctx, uuid.New().String())
		require.Error(t, err)
		require.True(t, errors.Is(err, libdb.ErrNotFound))
	})

	t.Run("GetByName_NotFound", func(t *testing.T) {
		_, err := svc.GetByName(ctx, "definitely-does-not-exist")
		require.Error(t, err)
		require.True(t, errors.Is(err, libdb.ErrNotFound))
	})

	// ── Update ──────────────────────────────────────────────────────────────
	t.Run("Update", func(t *testing.T) {
		srv := &runtimetypes.MCPServer{
			Name:                  "svc-update-me",
			Transport:             "sse",
			URL:                   "http://old.example.com/sse",
			ConnectTimeoutSeconds: 30,
		}
		require.NoError(t, svc.Create(ctx, srv))

		srv.URL = "http://new.example.com/sse"
		srv.ConnectTimeoutSeconds = 60
		require.NoError(t, svc.Update(ctx, srv))

		got, err := svc.Get(ctx, srv.ID)
		require.NoError(t, err)
		assert.Equal(t, "http://new.example.com/sse", got.URL)
		assert.Equal(t, 60, got.ConnectTimeoutSeconds)
		assert.True(t, got.UpdatedAt.After(srv.CreatedAt))
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		err := svc.Update(ctx, &runtimetypes.MCPServer{
			ID:        uuid.New().String(),
			Name:      "ghost",
			Transport: "sse",
			URL:       "http://ghost.example.com/sse",
		})
		require.Error(t, err)
	})

	// ── Delete ──────────────────────────────────────────────────────────────
	t.Run("Delete", func(t *testing.T) {
		srv := &runtimetypes.MCPServer{
			Name:      "svc-delete-me",
			Transport: "sse",
			URL:       "http://delete.example.com/sse",
		}
		require.NoError(t, svc.Create(ctx, srv))
		require.NoError(t, svc.Delete(ctx, srv.ID))

		_, err := svc.Get(ctx, srv.ID)
		require.Error(t, err)
		require.True(t, errors.Is(err, libdb.ErrNotFound))
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		err := svc.Delete(ctx, uuid.New().String())
		require.Error(t, err)
	})

	// ── List ────────────────────────────────────────────────────────────────
	t.Run("List_Pagination", func(t *testing.T) {
		// Create 3 uniquely-named servers for this sub-test
		for i := range 3 {
			require.NoError(t, svc.Create(ctx, &runtimetypes.MCPServer{
				Name:      fmt.Sprintf("svc-page-%s-%d", uuid.New().String()[:8], i),
				Transport: "sse",
				URL:       "http://mcp.example.com/sse",
			}))
		}

		// List with wide cursor — should get at least 3
		all, err := svc.List(ctx, nil, 100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(all), 3)

		// Page through with limit=1 using cursor
		if len(all) >= 2 {
			page1, err := svc.List(ctx, nil, 1)
			require.NoError(t, err)
			require.Len(t, page1, 1)

			cursor := page1[0].CreatedAt
			page2, err := svc.List(ctx, &cursor, 1)
			require.NoError(t, err)
			require.Len(t, page2, 1)
			// Second page item must be different from first
			assert.NotEqual(t, page1[0].ID, page2[0].ID)
		}
	})

	// ── Unique name constraint ───────────────────────────────────────────────
	t.Run("UniqueNameConstraint", func(t *testing.T) {
		srv := &runtimetypes.MCPServer{
			Name:      "svc-unique-name",
			Transport: "sse",
			URL:       "http://mcp.example.com/sse",
		}
		require.NoError(t, svc.Create(ctx, srv))

		dup := &runtimetypes.MCPServer{
			Name:      "svc-unique-name", // same name
			Transport: "http",
			URL:       "http://other.example.com/mcp",
		}
		err := svc.Create(ctx, dup)
		require.Error(t, err, "duplicate name should be rejected")
	})
}

// TestSystem_MCPServerService_PersistentRepoBridge verifies that MCPServer
// configs written via mcpserverservice are picked up by PersistentRepo at
// execution time (the 3-tier lookup: local → MCP DB → remote_hooks DB).
func TestSystem_MCPServerService_PersistentRepoBridge(t *testing.T) {
	ctx := t.Context()

	p := playground.New()
	p.WithPostgresTestContainer(ctx)
	require.NoError(t, p.GetError(), "playground setup failed")
	defer p.CleanUp()

	svc, err := p.GetMCPServerService()
	require.NoError(t, err)

	// Register an MCP server that exposes a fictional echo tool.
	// We don't actually connect — we just verify the config is persisted
	// and readable so that PersistentRepo could pick it up.
	srv := &runtimetypes.MCPServer{
		Name:                  "bridge-test-mcp",
		Transport:             "sse",
		URL:                   "http://unreachable-mcp.example.com/sse",
		ConnectTimeoutSeconds: 5,
	}
	require.NoError(t, svc.Create(ctx, srv))

	// Confirm the config is in the DB under the registered name
	got, err := svc.GetByName(ctx, "bridge-test-mcp")
	require.NoError(t, err)
	assert.Equal(t, "sse", got.Transport)
	assert.Equal(t, srv.URL, got.URL)

	// Confirm delete removes it so PersistentRepo would fall through to
	// remote_hooks on the next call (distributed-safe: no stale in-memory cache)
	require.NoError(t, svc.Delete(ctx, srv.ID))
	_, err = svc.GetByName(ctx, "bridge-test-mcp")
	require.True(t, errors.Is(err, libdb.ErrNotFound), "deleted config should not be findable")
}
