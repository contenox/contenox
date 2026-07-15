package acpsvc

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

type recOp struct {
	op      string
	errs    int
	changes int
	ended   int
}

type recTracker struct {
	mu  sync.Mutex
	ops []*recOp
}

func (rt *recTracker) Start(_ context.Context, op, _ string, _ ...any) (func(error), func(string, any), func()) {
	rt.mu.Lock()
	o := &recOp{op: op}
	rt.ops = append(rt.ops, o)
	rt.mu.Unlock()
	return func(error) { rt.mu.Lock(); o.errs++; rt.mu.Unlock() },
		func(string, any) { rt.mu.Lock(); o.changes++; rt.mu.Unlock() },
		func() { rt.mu.Lock(); o.ended++; rt.mu.Unlock() }
}

func (rt *recTracker) find(op string) *recOp {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, o := range rt.ops {
		if o.op == op {
			return o
		}
	}
	return nil
}

type fakeAgent struct {
	mu      sync.Mutex
	resp    *agentservice.PromptResponse
	err     error
	lastReq agentservice.PromptRequest
}

func (f *fakeAgent) Capabilities(context.Context) (*agentservice.AgentCapabilities, error) {
	return nil, nil
}
func (f *fakeAgent) SessionNew(context.Context, string) (string, error) { return "", nil }
func (f *fakeAgent) SessionList(context.Context) ([]*agentservice.SessionInfo, error) {
	return nil, nil
}
func (f *fakeAgent) SessionLoad(context.Context, string) (string, []taskengine.Message, error) {
	return "", nil, nil
}
func (f *fakeAgent) SessionResume(context.Context, string) (string, error) { return "", nil }
func (f *fakeAgent) SessionDelete(context.Context, string) error           { return nil }
func (f *fakeAgent) SessionEnsureDefault(context.Context) (string, error)  { return "", nil }
func (f *fakeAgent) Prompt(_ context.Context, req agentservice.PromptRequest) (*agentservice.PromptResponse, error) {
	f.mu.Lock()
	f.lastReq = req
	f.mu.Unlock()
	return f.resp, f.err
}

func (f *fakeAgent) lastPromptRequest() agentservice.PromptRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}

func transportWithFakeAgent(a agentservice.Agent) (*Transport, libacp.SessionID, *recTracker) {
	sid := libacp.SessionID("sess-cancel")
	rt := &recTracker{}
	tr := &Transport{
		sessions:        make(map[libacp.SessionID]*sessionEntry),
		contenoxToACPID: make(map[string]libacp.SessionID),
	}
	tr.deps.Engine = &enginesvc.Engine{Tracker: rt}
	tr.deps.ChainRegistry = &ChainRegistry{defaultChain: &taskengine.TaskChainDefinition{}}
	tr.sessions[sid] = &sessionEntry{InternalSessionID: "internal-1", Agent: a}
	return tr, sid, rt
}

func promptReq(sid libacp.SessionID) libacp.PromptRequest {
	return libacp.PromptRequest{
		SessionID: sid,
		Prompt:    []libacp.ContentBlock{{Type: string(libacp.ContentKindText), Text: "hi"}},
	}
}

func requireSpan(t *testing.T, rt *recTracker, wantErrs, wantChanges int) {
	t.Helper()
	s := rt.find("prompt")
	require.NotNil(t, s, "the prompt activity span must always be opened")
	require.Equal(t, wantErrs, s.errs)
	require.Equal(t, wantChanges, s.changes)
	require.Equal(t, 1, s.ended, "the tracker span must be ended exactly once")
}

func TestUnit_Prompt_CancelledStopReasonReturnsNilError(t *testing.T) {
	tr, sid, rt := transportWithFakeAgent(&fakeAgent{
		resp: &agentservice.PromptResponse{StopReason: agentservice.StopCancelled},
		err:  context.Canceled,
	})

	resp, err := tr.Prompt(context.Background(), promptReq(sid))

	require.NoError(t, err, "ACP spec: cancellation MUST NOT surface as a JSON-RPC error")
	require.Equal(t, libacp.StopReasonCancelled, resp.StopReason)
	requireSpan(t, rt, 0, 1)
}

func TestUnit_Prompt_CancelledParentContextFallback(t *testing.T) {
	tr, sid, rt := transportWithFakeAgent(&fakeAgent{
		resp: nil,
		err:  context.Canceled,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := tr.Prompt(ctx, promptReq(sid))

	require.NoError(t, err, "a cancelled parent context must still yield StopReason cancelled, not an error")
	require.Equal(t, libacp.StopReasonCancelled, resp.StopReason)
	requireSpan(t, rt, 0, 1)
}

func TestUnit_Prompt_GenuineFailureStaysAnError(t *testing.T) {
	tr, sid, rt := transportWithFakeAgent(&fakeAgent{
		resp: &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn},
		err:  errors.New("boom"),
	})

	resp, err := tr.Prompt(context.Background(), promptReq(sid))

	require.Error(t, err, "a non-cancellation failure must not masquerade as cancelled")
	require.Equal(t, libacp.StopReasonEndTurn, resp.StopReason)
	var e *libacp.Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, libacp.ErrInternalError, e.Code)
	requireSpan(t, rt, 1, 0)
}

func TestUnit_Prompt_HappyPath(t *testing.T) {
	tr, sid, rt := transportWithFakeAgent(&fakeAgent{
		resp: &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn},
		err:  nil,
	})

	resp, err := tr.Prompt(context.Background(), promptReq(sid))

	require.NoError(t, err)
	require.Equal(t, libacp.StopReasonEndTurn, resp.StopReason)
	requireSpan(t, rt, 0, 1)
}

func TestUnit_Prompt_IncludesSessionThinkTemplateVar(t *testing.T) {
	agent := &fakeAgent{resp: &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn}}
	tr, sid, rt := transportWithFakeAgent(agent)
	tr.sessions[sid].Think = "low"

	resp, err := tr.Prompt(context.Background(), promptReq(sid))
	require.NoError(t, err)
	require.Equal(t, libacp.StopReasonEndTurn, resp.StopReason)
	req := agent.lastPromptRequest()
	require.Equal(t, "low", req.TemplateVars["think"])
	requireSpan(t, rt, 0, 1)
}

func TestUnit_Prompt_IncludesAltAndMaxTokenTemplateVars(t *testing.T) {
	agent := &fakeAgent{resp: &agentservice.PromptResponse{StopReason: agentservice.StopEndTurn}}
	tr, sid, rt := transportWithFakeAgent(agent)
	tr.defaultAltModel = "small-model"
	tr.defaultAltProvider = "local"
	tr.defaultMaxTokens = "8192"

	resp, err := tr.Prompt(context.Background(), promptReq(sid))
	require.NoError(t, err)
	require.Equal(t, libacp.StopReasonEndTurn, resp.StopReason)
	req := agent.lastPromptRequest()
	require.Equal(t, "small-model", req.TemplateVars["alt_model"])
	require.Equal(t, "local", req.TemplateVars["alt_provider"])
	require.Equal(t, "8192", req.TemplateVars["max_tokens"])
	requireSpan(t, rt, 0, 1)
}
