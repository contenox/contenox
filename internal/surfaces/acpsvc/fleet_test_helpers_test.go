package acpsvc

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/enginesvc"
	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

type fleetPeer struct {
	tr     *Transport
	client *libacp.ClientSideConnection
	lc     *loopbackClient
	built  <-chan *Transport
}

func (p *fleetPeer) ensureTransport(t *testing.T) *Transport {
	t.Helper()
	if p.tr != nil {
		return p.tr
	}
	select {
	case p.tr = <-p.built:
	case <-time.After(10 * time.Second):
		t.Fatal("no transport was built for this peer")
	}
	return p.tr
}

func (p *fleetPeer) newSession(t *testing.T) (libacp.SessionID, string) {
	t.Helper()
	ctx := context.Background()
	_, err := p.client.Initialize(ctx, libacp.InitializeRequest{ProtocolVersion: libacp.ProtocolVersion})
	require.NoError(t, err)
	resp, err := p.client.NewSession(ctx, libacp.NewSessionRequest{Cwd: t.TempDir(), McpServers: []libacp.McpServer{}})
	require.NoError(t, err)
	p.lc.drain(t, 2)
	p.ensureTransport(t)
	return resp.SessionID, p.internalID(t, resp.SessionID)
}

func (p *fleetPeer) internalID(t *testing.T, sid libacp.SessionID) string {
	t.Helper()
	p.tr.sessionMu.Lock()
	defer p.tr.sessionMu.Unlock()
	entry, ok := p.tr.sessions[sid]
	require.True(t, ok, "session %s is not open on this transport", sid)
	require.NotEmpty(t, entry.InternalSessionID)
	return entry.InternalSessionID
}

type fleet struct {
	router  *SessionRouter
	desk    *fleetPeer
	built   <-chan *Transport
	factory libacp.AgentFactory
	db      libdb.DBManager
}

func (f *fleet) attach(t *testing.T, _ string) *fleetPeer {
	t.Helper()
	agentR, clientW := io.Pipe()
	clientR, agentW := io.Pipe()
	built := make(chan *Transport, 1)
	factory := func(c *libacp.AgentSideConnection) libacp.Agent {
		a := f.factory(c)
		built <- a.(*Transport)
		return a
	}
	agentConn := libacp.NewAgentSideConnection(&wirePipe{r: agentR, w: agentW}, factory)
	lc := newLoopbackClient()
	clientConn := libacp.NewClientSideConnection(&wirePipe{r: clientR, w: clientW}, func(*libacp.ClientSideConnection) libacp.Client { return lc })

	go func() { _ = agentConn.Run(context.Background()) }()
	go func() { _ = clientConn.Run(context.Background()) }()

	t.Cleanup(func() {
		_ = agentR.Close()
		_ = agentW.Close()
		_ = clientR.Close()
		_ = clientW.Close()
	})
	return &fleetPeer{client: clientConn, lc: lc, built: built}
}

func newFleet(t *testing.T, withFleetDeps ...func(*Deps, libdb.DBManager)) *fleet {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "fleet.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	bus := libbus.NewSQLiteWithOptions(db.WithoutTransaction(), libbus.SQLiteBusOptions{
		EventPoll:   5 * time.Millisecond,
		RequestPoll: 5 * time.Millisecond,
	})

	router := NewSessionRouter()
	built := make(chan *Transport, 8)
	deps := Deps{
		Engine:        &enginesvc.Engine{Bus: bus},
		DB:            db,
		ChainRegistry: &ChainRegistry{defaultChain: &taskengine.TaskChainDefinition{}},
		WorkspaceID:   "fleet-ws",
		SessionRouter: router,
	}
	for _, apply := range withFleetDeps {
		apply(&deps, db)
	}
	base := New(deps)
	factory := func(c *libacp.AgentSideConnection) libacp.Agent {
		a := base(c)
		built <- a.(*Transport)
		return a
	}

	agentR, clientW := io.Pipe()
	clientR, agentW := io.Pipe()
	agentConn := libacp.NewAgentSideConnection(&wirePipe{r: agentR, w: agentW}, factory)
	deskLC := newLoopbackClient()
	deskConn := libacp.NewClientSideConnection(&wirePipe{r: clientR, w: clientW}, func(*libacp.ClientSideConnection) libacp.Client { return deskLC })

	agentDone := make(chan error, 1)
	deskDone := make(chan error, 1)
	go func() { agentDone <- agentConn.Run(ctx) }()
	go func() { deskDone <- deskConn.Run(ctx) }()

	f := &fleet{router: router, desk: &fleetPeer{client: deskConn, lc: deskLC, built: built}, built: built, factory: factory, db: db}
	f.desk.ensureTransport(t)

	t.Cleanup(func() {
		cancel()
		<-agentDone
		<-deskDone
		require.NoError(t, bus.Close())
		require.NoError(t, db.Close())
	})
	return f
}
