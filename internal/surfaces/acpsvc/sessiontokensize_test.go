package acpsvc

import (
	"testing"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/clikv"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/libacp"
	"github.com/stretchr/testify/require"
)

// TestUnit_NarrowContextLimit_ModelWindowNarrowsTheChainBudget pins the half of
// the gauge's arithmetic a chain/session-only test cannot reach without a
// runtime state: a model's reported context length narrows a chain budget, the
// way taskengine narrows ctxLength, so a 131072 chain against a 128000 window
// reports 128000 rather than the chain's number.
func TestUnit_NarrowContextLimit_ModelWindowNarrowsTheChainBudget(t *testing.T) {
	t.Parallel()

	require.Equal(t, 128000, narrowContextLimit(131072, 128000),
		"a model window below the chain budget wins, or the gauge overstates the turn's budget")
	require.Equal(t, 131072, narrowContextLimit(131072, 262144),
		"a model window above the chain budget does not widen it")
	require.Equal(t, 128000, narrowContextLimit(0, 128000),
		"a chain with no declared budget defers to the model window")
	require.Zero(t, narrowContextLimit(0, 0),
		"with no budget anywhere there is no honest denominator")
}

func TestUnit_ExplicitContextAboveModelCapacityFails(t *testing.T) {
	t.Parallel()

	err := validateExplicitContextCapacity(230000, 131072, 71932, "llama", "gemma4-e4b")
	require.ErrorContains(t, err, "configured context window 230000 tokens")
	require.ErrorContains(t, err, "chain ceiling 131072")
	require.ErrorContains(t, err, "llama/gemma4-e4b reported capacity of 71932 tokens")
	require.ErrorContains(t, err, "set inference.context.window_tokens to auto")
	require.NoError(t, validateExplicitContextCapacity(64000, 131072, 71932, "llama", "gemma4-e4b"))
	require.NoError(t, validateExplicitContextCapacity(230000, 32000, 71932, "llama", "gemma4-e4b"))
}

// TestUnit_SessionTokenSize_InheritsGlobalDefaultTokenLimit pins the global
// default-token-limit setting: a session with no session-level override
// inherits it, narrowed by the chain budget.
func TestUnit_SessionTokenSize_InheritsGlobalDefaultTokenLimit(t *testing.T) {
	ctx, db := setupConfigOptionsDB(t)
	const sid = libacp.SessionID("s")

	require.NoError(t, clikv.WriteConfig(ctx, runtimetypes.New(db.WithoutTransaction()), "",
		defaultTokenLimitConfigKey, "65536"))

	tr := &Transport{
		deps:            Deps{DB: db},
		sessions:        map[libacp.SessionID]*sessionEntry{sid: {EffectiveTokenLimit: 0}},
		contenoxToACPID: make(map[string]libacp.SessionID),
	}
	tr.deps.ChainRegistry = &ChainRegistry{defaultChain: &taskengine.TaskChainDefinition{TokenLimit: 131072}}

	require.Equal(t, 65536, tr.sessionTokenSize(ctx, sid),
		"a session with no override inherits the global default, narrowed by the chain budget")

	// A malformed or zero value is no default: the budget falls back to the
	// chain's own, never to a partial parse.
	require.NoError(t, clikv.WriteConfig(ctx, runtimetypes.New(db.WithoutTransaction()), "",
		defaultTokenLimitConfigKey, "not-a-number"))
	require.Equal(t, 131072, tr.sessionTokenSize(ctx, sid),
		"an unparseable default is ignored, not half-applied")
}

// TestUnit_StoredContextMaskAboveCapacityIsFollowedNotEnforced pins the split
// between a mask and a request. A stored window is a capacity mask for providers
// that report none; the worker reports its own, so a mask left over from another
// model, or from a moment when more device memory was free, narrows to the
// reported capacity instead of failing every turn — which is what a live session
// did after `contenox auto` persisted the window it measured.
func TestUnit_StoredContextMaskAboveCapacityIsFollowedNotEnforced(t *testing.T) {
	t.Parallel()

	ctx, db := setupConfigOptionsDB(t)
	const sid = libacp.SessionID("s")

	require.NoError(t, clikv.WriteConfig(ctx, runtimetypes.New(db.WithoutTransaction()), "",
		defaultTokenLimitConfigKey, "43722"))

	tr := &Transport{
		deps:            Deps{DB: db},
		sessions:        map[libacp.SessionID]*sessionEntry{sid: {EffectiveTokenLimit: 0}},
		contenoxToACPID: make(map[string]libacp.SessionID),
	}
	tr.deps.ChainRegistry = &ChainRegistry{defaultChain: &taskengine.TaskChainDefinition{TokenLimit: 131072}}

	stored, explicit := tr.requestedContextLength(ctx, tr.sessions[sid])
	require.Equal(t, 43722, stored)
	require.True(t, explicit, "a stored window is still a window the gauge must count")
	require.False(t, tr.contextRequestIsThisTurn(tr.sessions[sid]),
		"an inherited window is not a request made this turn")

	tr.deps.DefaultContextTokens = &stored
	tr.deps.DefaultContextSource = "invocation"
	require.True(t, tr.contextRequestIsThisTurn(tr.sessions[sid]),
		"a window passed on this invocation is a request, and fails loudly when it does not fit")
}

// TestUnit_SessionOverrideIsARequestMadeThisTurn pins the other explicit source:
// /context in a live session names a number the operator chose, so it keeps the
// loud refusal even though it is not an invocation flag.
func TestUnit_SessionOverrideIsARequestMadeThisTurn(t *testing.T) {
	t.Parallel()

	tr := &Transport{deps: Deps{DefaultContextSource: "global"}}
	sess := &sessionEntry{ContextOverride: true, EffectiveTokenLimit: 65536}
	require.True(t, tr.contextRequestIsThisTurn(sess),
		"a session override is this turn's request, whatever the stored default says")

	sess = &sessionEntry{EffectiveTokenLimit: 0}
	require.False(t, tr.contextRequestIsThisTurn(sess))
}
