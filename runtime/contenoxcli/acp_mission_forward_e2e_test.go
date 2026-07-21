package contenoxcli

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/contenox/runtime/libacp"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/operatorinbox"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

// TestSystem_ACPMissionForward is the acceptance for the OPT-IN forwarding path:
// a `contenox acp` editor launched with CONTENOX_SERVER_URL set fires `/mission`
// at a running `contenox serve` over its REST API instead of the in-process fleet
// (the operator's explicit choice to fire onto a bigger box; the in-process
// default is TestSystem_ACPMissionInProcess). It runs END TO END against the REAL
// binary — a hermetic serve subprocess and a hermetic `contenox acp` subprocess
// driven over stdio by a real libacp client, nothing mocked below the process
// boundary — and pins the honesty contract the forwarding rests on:
//
//	serve up   → /mission IS advertised on a new acp session; invoking it
//	             dispatches a REAL unit on the SERVE side (the mission record and
//	             its report exist there, the deterministic fixture lands); the
//	             confirmation names the OPERATOR INBOX (not this editor session),
//	             and the unit's report really lands there as parent-gone — the
//	             designed cross-process fallback, never an error.
//	serve down → /mission STAYS advertised (advertisement is unconditional now — a
//	             stable menu), and an invocation yields the teaching error naming
//	             the serve that went silent (honesty lives at the point of use).
//
// Hermetic like scripts/run_apitests.sh: an isolated HOME, `contenox init`, a
// deterministic no-model chain-agent fixture seeded before boot, a fake default
// model so any accidental model resolution fails loudly. No LLM, GPU, or network.

// fwdReporterChain is the deterministic, model-free chain the FORWARDED unit runs
// as its first and only turn: it files a RESULT report, then finishes the mission
// LANDED — both through its granted mission tools, no model touched. The report is
// what proves the cross-process supervision-edge fallback: its parent session
// lives in the acp process serve's kernel does not own, so it lands in the
// operator inbox as parent-gone. The landed status is what proves the unit ran.
const fwdReporterChain = `{
  "id": "agent-fwd-reporter",
  "description": "Forwarding e2e: file a result report, then finish the mission landed.",
  "tasks": [
    {
      "id": "report",
      "handler": "tools",
      "tools": {"name": "mission", "tool_name": "mission_report", "args": {"kind": "result", "summary": "forwarded unit reporting in"}},
      "transition": {"branches": [{"operator": "default", "goto": "finish"}]}
    },
    {
      "id": "finish",
      "handler": "tools",
      "tools": {"name": "mission", "tool_name": "mission_finish", "args": {"status": "landed", "reason": "forwarded landing"}},
      "transition": {"branches": [{"operator": "default", "goto": "done"}]}
    },
    {"id": "done", "handler": "noop", "transition": {"branches": [{"operator": "default", "goto": "end"}]}}
  ]
}`

const (
	fwdAgentName  = "agent-fwd-reporter"
	fwdIntent     = "forward this research mission from the editor"
	fwdReportText = "forwarded unit reporting in"
)

func TestSystem_ACPMissionForward(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping acp mission-forward system e2e: builds contenox and boots a real serve + acp subprocess")
	}

	bin := fwdBuildBin(t)

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	workspaceDir := filepath.Join(root, "workspace")
	dataDir := filepath.Join(workspaceDir, ".contenox")
	dbPath := filepath.Join(homeDir, ".contenox", "local.db")
	require.NoError(t, os.MkdirAll(homeDir, 0o755))
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	baseEnv := append(os.Environ(),
		"HOME="+homeDir,
		"CONTENOX_DEFAULT_MODEL=fwd-e2e-fake-model",
		"CONTENOX_DEFAULT_PROVIDER=ollama",
		// Neutralize ambient overrides from the developer's shell so they cannot
		// redirect a child's serve target or chain.
		"CONTENOX_SERVER_URL=",
		"CONTENOX_SERVER_TOKEN=",
		"CONTENOX_ACP_CHAIN_PATH=",
	)

	// Seed the isolated state through the real CLI (init scaffolds .contenox and the
	// embedded HITL policies). The chain goes in a PLAIN directory — NOT under
	// .contenox, which control-plane isolation (landed today) refuses to discover
	// from — read by the dispatched unit via CONTENOX_ACP_CHAIN_PATH.
	fwdRunCLI(t, bin, baseEnv, "--data-dir", dataDir, "--db", dbPath, "init", "--force")
	chainsDir := filepath.Join(root, "chains")
	require.NoError(t, os.MkdirAll(chainsDir, 0o755))
	chainPath := filepath.Join(chainsDir, "fwd-reporter.json")
	require.NoError(t, os.WriteFile(chainPath, []byte(fwdReporterChain), 0o644))

	// Declare the fired agent as an external `contenox acp --auto` unit, and seed
	// the global config the forwarded /mission handler reads (default-mission-policy
	// is the envelope; update-check=false keeps startup off the network) — all in ONE
	// db handle CLOSED before serve boots, so the test and serve never contend on the
	// SQLite file (the proven scripted-e2e discipline).
	fwdSeedRegistryAndConfig(t, dbPath, bin, chainPath)

	port := fwdFreePort(t)
	serverURL := "http://127.0.0.1:" + strconv.Itoa(port)
	serveEnv := append(append([]string{}, baseEnv...),
		"ADDR=127.0.0.1",
		"PORT="+strconv.Itoa(port),
		"TOKEN=",
	)
	serveLog, stopServe := fwdStartServe(t, bin, serveEnv, dataDir, dbPath, serverURL)

	// A handle to the ONE shared store, for serve-side assertions (the mission
	// record and the routed report both live here, written by serve and its unit).
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	missions := missionservice.New(db)
	inbox := operatorinbox.New(db)

	// ── Spawn `contenox acp` as Zed would, env pointing at the serve ────────────
	acpEnv := append(append([]string{}, baseEnv...), "CONTENOX_SERVER_URL="+serverURL)
	h := fwdSpawnACP(t, bin, acpEnv)

	_, err = h.client.Initialize(ctx, libacp.InitializeRequest{
		ProtocolVersion: libacp.ProtocolVersion,
		ClientInfo:      &libacp.Implementation{Name: "zed", Version: "e2e"},
	})
	require.NoErrorf(t, err, "acp initialize failed\nacp stderr:\n%s", h.stderr())

	projectDir := filepath.Join(workspaceDir, "project")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	// ── serve UP: /mission is advertised ────────────────────────────────────────
	sid, cmds := h.newSessionCommands(t, ctx, projectDir)
	require.Containsf(t, cmds, "mission",
		"/mission must be advertised when a serve is reachable\nacp stderr:\n%s\nserve log:\n%s", h.stderr(), serveLog())

	// ── Invoking /mission dispatches a REAL unit on the SERVE side ───────────────
	confirmation := h.promptFor(t, ctx, sid, "/mission "+fwdAgentName+" "+fwdIntent)
	require.Contains(t, confirmation, "Mission fired", "the forwarded fire is confirmed")
	require.Contains(t, confirmation, fwdAgentName, "the confirmation names the fired agent")
	require.Contains(t, confirmation, "operator inbox",
		"the FORWARDED confirmation must name the inbox routing, not promise session delivery it cannot make")

	// The mission record exists on the serve side and the deterministic fixture
	// LANDS — the unit really booted as a serve subprocess and ran.
	var landed *missionservice.Mission
	require.Eventuallyf(t, func() bool {
		ms, lerr := missions.List(ctx, nil, 100)
		if lerr != nil {
			return false
		}
		for _, m := range ms {
			if m.AgentName == fwdAgentName && m.Intent == fwdIntent {
				landed = m
				return m.Status == missionservice.StatusLanded
			}
		}
		return false
	}, 90*time.Second, 250*time.Millisecond,
		"the forwarded mission must exist on serve and land\nacp stderr:\n%s\nserve log:\n%s", h.stderr(), serveLog())
	require.NotEmpty(t, landed.ParentSessionID,
		"the firing acp session id rode the dispatch as the supervision edge (recorded even though it is cross-process)")

	// The unit's report lands in the OPERATOR INBOX as parent-gone — the designed
	// fallback: its parent session lives in the acp process, which serve's kernel
	// does not own, so DeliverToSession misses and the router inboxes it. Never an
	// error; the durable report is always readable.
	require.Eventuallyf(t, func() bool {
		items, lerr := inbox.List(ctx, 100)
		if lerr != nil {
			return false
		}
		for _, it := range items {
			if it.MissionID == landed.ID && it.Report.Summary == fwdReportText {
				require.Equal(t, operatorinbox.ReasonParentGone, it.Reason,
					"a forwarded unit's report lands as parent-gone: its supervising session is not on serve's kernel")
				return true
			}
		}
		return false
	}, 60*time.Second, 200*time.Millisecond,
		"the forwarded unit's report must fall back to the operator inbox as parent-gone\nserve log:\n%s", serveLog())

	// ── serve DOWN: /mission stays advertised, invocation teaches ──────────────
	stopServe()

	// The refit makes advertisement UNCONDITIONAL (a stable menu the operator can
	// rely on): /mission is still listed on a fresh session even with the serve
	// down. Honesty now lives at INVOCATION, not in a vanishing menu entry.
	downSID, names := h.newSessionCommands(t, ctx, projectDir)
	require.Containsf(t, names, "mission",
		"/mission stays advertised even with the serve down — honesty lives at invocation now\nacp stderr:\n%s", h.stderr())

	// Typed at a dead serve it teaches, naming the serve that went silent. Polled:
	// the reachability probe is cached (~1s), so the honest verdict may take a beat
	// after the serve stops, and the first attempt may surface a raw dispatch error.
	require.Eventuallyf(t, func() bool {
		teaching := h.promptFor(t, ctx, downSID, "/mission "+fwdAgentName+" "+fwdIntent)
		return strings.Contains(teaching, "unavailable") &&
			strings.Contains(teaching, "stopped answering") &&
			strings.Contains(teaching, serverURL)
	}, 20*time.Second, 750*time.Millisecond,
		"a /mission typed at a dead serve must yield the teaching error naming the serve\nacp stderr:\n%s", h.stderr())
}

// ── ACP client harness over the acp subprocess's stdio ──────────────────────

// fwdACPClient captures every session/update notification in wire order, so the
// test can assert the advertised command menu and the /mission confirmation the
// way a real editor renders them.
type fwdACPClient struct {
	libacp.UnimplementedClient
	updates chan libacp.SessionNotification
}

func (c *fwdACPClient) SessionUpdate(_ context.Context, n libacp.SessionNotification) error {
	c.updates <- n
	return nil
}

type fwdACPHarness struct {
	client    *libacp.ClientSideConnection
	lc        *fwdACPClient
	stderrBuf *fwdLockedBuffer
}

func (h *fwdACPHarness) stderr() string { return h.stderrBuf.String() }

// stdioRWC adapts a subprocess's stdout (read) + stdin (write) into the single
// ReadWriteCloser libacp speaks over — exactly how an editor talks to `contenox
// acp`, but with the pipes an exec.Cmd hands back instead of the editor's tty.
type stdioRWC struct {
	r io.Reader
	w io.WriteCloser
}

func (s stdioRWC) Read(p []byte) (int, error)  { return s.r.Read(p) }
func (s stdioRWC) Write(p []byte) (int, error) { return s.w.Write(p) }
func (s stdioRWC) Close() error                { return s.w.Close() }

// fwdSpawnACP starts `contenox acp` as a subprocess and drives it with a real
// libacp client over its stdio, returning a harness. Cleanup cancels the client
// and reaps the process.
func fwdSpawnACP(t *testing.T, bin string, env []string) *fwdACPHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.Command(bin, "acp")
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderrBuf := &fwdLockedBuffer{}
	cmd.Stderr = stderrBuf
	require.NoError(t, cmd.Start())

	lc := &fwdACPClient{updates: make(chan libacp.SessionNotification, 256)}
	client := libacp.NewClientSideConnection(stdioRWC{r: stdout, w: stdin}, func(*libacp.ClientSideConnection) libacp.Client {
		return lc
	})
	go func() { _ = client.Run(ctx) }()

	t.Cleanup(func() {
		cancel()
		_ = stdin.Close()
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _, _ = cmd.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	})

	return &fwdACPHarness{client: client, lc: lc, stderrBuf: stderrBuf}
}

// newSessionCommands opens a fresh session and returns its id and the advertised
// slash-command names (from the available_commands_update the agent emits after
// the session/new result).
func (h *fwdACPHarness) newSessionCommands(t *testing.T, ctx context.Context, cwd string) (libacp.SessionID, []string) {
	t.Helper()
	resp, err := h.client.NewSession(ctx, libacp.NewSessionRequest{Cwd: cwd, McpServers: []libacp.McpServer{}})
	require.NoErrorf(t, err, "session/new failed\nacp stderr:\n%s", h.stderr())
	deadline := time.After(10 * time.Second)
	for {
		select {
		case n := <-h.lc.updates:
			if n.Update.SessionUpdate == libacp.SessionUpdateAvailableCommands {
				names := make([]string, 0, len(n.Update.AvailableCommands))
				for _, c := range n.Update.AvailableCommands {
					names = append(names, c.Name)
				}
				return resp.SessionID, names
			}
		case <-deadline:
			t.Fatalf("timed out waiting for available_commands_update\nacp stderr:\n%s", h.stderr())
		}
	}
}

// promptFor sends one prompt (here always a `/mission …` slash command, which
// acpsvc intercepts) and returns the text of the agent_message_chunk the command
// emits — the confirmation, or the teaching/command error rendered inline.
func (h *fwdACPHarness) promptFor(t *testing.T, ctx context.Context, sid libacp.SessionID, text string) string {
	t.Helper()
	_, err := h.client.Prompt(ctx, libacp.PromptRequest{
		SessionID: sid,
		Prompt:    []libacp.ContentBlock{libacp.NewTextContent(text)},
	})
	require.NoErrorf(t, err, "prompt failed\nacp stderr:\n%s", h.stderr())
	deadline := time.After(15 * time.Second)
	for {
		select {
		case n := <-h.lc.updates:
			if n.Update.SessionUpdate == libacp.SessionUpdateAgentMessageChunk {
				if c := n.Update.Content; c != nil && strings.TrimSpace(c.Text) != "" {
					return c.Text
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for the command's agent message\nacp stderr:\n%s", h.stderr())
		}
	}
}

// ── serve subprocess (with an explicit mid-test stop) ───────────────────────

// fwdStartServe boots `contenox serve` in its own process group, health-polls it
// to readiness, and returns (logAccessor, stop). stop terminates the whole group
// (reaping any dispatched unit subprocesses) and is idempotent; it is also
// registered as cleanup so a test that never calls it still tears serve down.
func fwdStartServe(t *testing.T, bin string, env []string, dataDir, dbPath, serverURL string) (func() string, func()) {
	t.Helper()
	log := &fwdLockedBuffer{}
	cmd := exec.Command(bin, "--data-dir", dataDir, "--db", dbPath, "serve")
	cmd.Env = env
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())

	pgid := cmd.Process.Pid
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
			done := make(chan struct{})
			go func() { _, _ = cmd.Process.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
			}
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		})
	}
	t.Cleanup(stop)

	deadline := time.Now().Add(45 * time.Second)
	healthURL := serverURL + "/health"
	for time.Now().Before(deadline) {
		// Fail fast if serve died at boot rather than polling a corpse for 45s.
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			t.Fatalf("contenox serve exited before becoming ready:\n%s", log.String())
		}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, healthURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return log.String, stop
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("contenox serve did not become ready at %s:\n%s", healthURL, log.String())
	return log.String, stop
}

// fwdLockedBuffer is a concurrency-safe sink for a subprocess's stderr/log.
type fwdLockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *fwdLockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *fwdLockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// ── self-contained build/CLI/port helpers (no dependency on sibling test files) ──

// fwdBuildBin compiles cmd/contenox into t.TempDir(); the go build cache makes
// reruns cheap.
func fwdBuildBin(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "contenox")
	out, err := exec.Command("go", "build", "-o", binPath, "github.com/contenox/runtime/cmd/contenox").CombinedOutput()
	require.NoErrorf(t, err, "build contenox:\n%s", out)
	return binPath
}

// fwdRunCLI runs a contenox setup command and fails on non-zero exit.
func fwdRunCLI(t *testing.T, bin string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "contenox %v:\n%s", args, out)
}

// fwdFreePort returns a currently-free loopback TCP port. A small race window
// exists between close and serve's bind, acceptable for a test.
func fwdFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// fwdSeedRegistryAndConfig declares the fired agent as an external `contenox acp
// --auto` unit bound to its chain file, and seeds the global config the forwarded
// /mission handler reads. It does both in ONE db handle that is CLOSED before it
// returns, so serve (and the ACP process) open the SQLite file without contending
// with this seeding write — the discipline the scripted e2e proved.
func fwdSeedRegistryAndConfig(t *testing.T, dbPath, bin, chainPath string) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	agent := &runtimetypes.Agent{Name: fwdAgentName, Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   bin,
		Args:      []string{"acp", "--auto"},
		Env:       map[string]string{"CONTENOX_ACP_CHAIN_PATH": chainPath},
	}))
	require.NoError(t, agentregistryservice.New(db).Create(ctx, agent))

	// default-mission-policy and update-check are GLOBAL config keys (not
	// workspace-scoped), so an empty workspace id writes them where clikv.Read finds
	// them regardless of the reading process's workspace.
	store := runtimetypes.New(db.WithoutTransaction())
	require.NoError(t, clikv.WriteConfig(ctx, store, "", "default-mission-policy", "hitl-policy-default.json"))
	require.NoError(t, clikv.WriteConfig(ctx, store, "", "update-check", "false"))
}
