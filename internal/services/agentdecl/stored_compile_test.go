package agentdecl_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/agentdecl"
	"github.com/contenox/contenox/internal/services/hitlservice"
	"github.com/contenox/contenox/internal/services/vfs"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/acpsvc"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This is the claim the store-backed half of the Files seam exists for: a
// declaration that lives only in a tenant's tree in a database compiles into a
// chain and a policy in that same tree, and the chain loads back out of it for a
// session to run. No directory takes part anywhere in the path, which is what a
// deployment that keeps a tenant's tree in a database needs, and what the
// compiler could not do while it opened the filesystem itself.
func TestSystem_StoredDeclarations_CompileAndLoadWithNoFilesystem(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "stored.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	store, err := vfs.New(ctx, db, vfs.Callbacks{})
	require.NoError(t, err)

	const tenant = "acct-1"
	account, err := vfs.Stored(store, tenant).Sub(".contenox")
	require.NoError(t, err)
	require.NoError(t, account.MkdirAll(ctx, agentdecl.NativeSourceDir))
	require.NoError(t, account.WriteFile(ctx, "agents/triage.md", []byte(declTriage)))

	root := agentdecl.Root{Key: "store:" + tenant + "/.contenox", FS: account}
	dirs := agentdecl.DiscoverSourceDirs(ctx, []agentdecl.Root{root}, nil)
	require.Len(t, dirs, 1, "the stored agents directory must be discovered like one on disk")
	assert.True(t, dirs[0].Native)

	generated, err := account.Sub(agentdecl.GeneratedDirName)
	require.NoError(t, err)
	generatedRoot := agentdecl.Root{Key: root.Key + "/" + agentdecl.GeneratedDirName, FS: generated}

	cfg, err := agentdecl.Load(ctx, root)
	require.NoError(t, err)

	results, err := agentdecl.Sync(ctx, dirs, generatedRoot, cfg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, agentdecl.ActionCreated, results[0].Action, "reason: %s", results[0].Reason)
	require.Equal(t, "triage", results[0].Name)

	// The compiled pair is in the store, under the paths a chain loader and a
	// policy source look for.
	chainRaw, err := generated.ReadFile(ctx, "chain-agent-triage.json")
	require.NoError(t, err)
	var chain taskengine.TaskChainDefinition
	require.NoError(t, json.Unmarshal(chainRaw, &chain))
	require.NoError(t, taskengine.LintChain(&chain), "a chain compiled from the store must lint like any other")
	assert.Equal(t, "triage", chain.ID)

	policyRaw, err := generated.ReadFile(ctx, agentdecl.PolicyFileFor("triage"))
	require.NoError(t, err)
	assert.Contains(t, string(policyRaw), agentdecl.PolicySchemaURL)

	// A second pass over the same store writes nothing: the sync state and the
	// generated files are compared, not the source hash alone.
	again, err := agentdecl.Sync(ctx, dirs, generatedRoot, cfg)
	require.NoError(t, err)
	require.Len(t, again, 1)
	assert.Equal(t, agentdecl.ActionUnchanged, again[0].Action)

	// A loader holding only the account's handle resolves the emitted chain,
	// which is what a session needs before it can run the agent.
	chainDir, name, ok := acpsvc.ChainFileHandle(ctx, account, "chain-agent-triage.json")
	require.True(t, ok, "the emitted chain must resolve through the search path")
	registry, err := acpsvc.LoadChainRegistryAt(ctx, chainDir, name, generatedRoot.Key)
	require.NoError(t, err)
	assert.Equal(t, "triage", registry.Default().ID)

	// And the emitted policy is readable the same way, so the gate the chain
	// runs under travels with it.
	policies := hitlservice.NewFilesPolicySource(generated)
	readBack, err := policies.ReadPolicy(ctx, tenant, "triage")
	require.NoError(t, err)
	assert.Equal(t, policyRaw, readBack)
}

// A declaration removed from the store retires its chain and policy from the
// store, so a deleted agent does not keep running from a derived file nothing
// produces any more.
func TestSystem_StoredDeclarations_RetireWhenTheSourceGoes(t *testing.T) {
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "stored.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	store, err := vfs.New(ctx, db, vfs.Callbacks{})
	require.NoError(t, err)

	account, err := vfs.Stored(store, "acct-1").Sub(".contenox")
	require.NoError(t, err)
	require.NoError(t, account.MkdirAll(ctx, agentdecl.NativeSourceDir))
	require.NoError(t, account.WriteFile(ctx, "agents/triage.md", []byte(declTriage)))

	root := agentdecl.Root{Key: "store:acct-1/.contenox", FS: account}
	generated, err := account.Sub(agentdecl.GeneratedDirName)
	require.NoError(t, err)
	generatedRoot := agentdecl.Root{Key: root.Key + "/" + agentdecl.GeneratedDirName, FS: generated}
	cfg, err := agentdecl.Load(ctx, root)
	require.NoError(t, err)

	dirs := agentdecl.DiscoverSourceDirs(ctx, []agentdecl.Root{root}, nil)
	_, err = agentdecl.Sync(ctx, dirs, generatedRoot, cfg)
	require.NoError(t, err)

	require.NoError(t, account.Remove(ctx, "agents/triage.md"))
	dirs = agentdecl.DiscoverSourceDirs(ctx, []agentdecl.Root{root}, nil)
	_, err = agentdecl.Sync(ctx, dirs, generatedRoot, cfg)
	require.NoError(t, err)

	_, err = generated.ReadFile(ctx, "chain-agent-triage.json")
	require.Error(t, err, "a retired chain must not stay readable in the store")
	_, err = generated.ReadFile(ctx, agentdecl.PolicyFileFor("triage"))
	require.Error(t, err, "a retired policy must not stay readable in the store")
}
