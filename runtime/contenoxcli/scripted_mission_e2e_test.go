package contenoxcli

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

// TestSystem_ScriptedMissionSequence pins the UNIX-primitive contract `contenox
// mission fire --wait` promises, END TO END against the REAL binary and a REAL
// hermetic serve — the part the mission_cmd_test.go unit tests can only assert
// against a fake HTTP server. The unit tests prove waitForMissionOutcome maps a
// STAGED terminal condition onto the right exit code; this proves the whole loop:
// a fired unit really boots as a serve subprocess, really files its own terminal
// fact through its mission tools over the shared store, and `fire --wait` really
// observes that fact over serve's REST API and exits with the documented code a
// shell script branches on.
//
// The scenario is the scripting contract itself:
//
//	fire(lands, --wait)                    → exit 0   (mission_finish landed)
//	fire(lands) && fire(lands)             → exit 0   (&& composes on success)
//	fire(blocker) && fire(lands)           → exit 2   (a blocker breaks the chain;
//	                                                    the second fire never runs)
//	fire(progress, short --wait-timeout)   → exit 3   (only intermediate reports;
//	                                                    still running at the deadline)
//
// and the machine-readability half: with -q the mission id is the ONLY thing on
// stdout, so `mid=$(contenox mission fire -q --wait …)` captures a correlatable id
// regardless of the wait's eventual exit code — verified by feeding the captured id
// back to `mission show --json`.
//
// Hermetic like scripts/run_apitests.sh, but in Go: an isolated HOME, `contenox
// init`, three no-model chain-agent fixtures, a `contenox serve` subprocess
// health-polled to readiness, then real `contenox mission fire` invocations driven
// through `sh -c` so the `&&` semantics are the shell's, not the test's. No LLM,
// GPU, or network: each fixture is a single mission-tool `tools` task that resolves
// no model.
//
// # Why the agents are seeded into the registry in-test as external `acp --auto`
//
// The units are declared as EXTERNAL agents whose command is `contenox acp --auto`
// bound to a deterministic chain — the exact unit shape the mission-tool e2es in
// runtime/fleetservice prove files reports and finishes missions reliably. They are
// created in the registry directly (this process, before serve boots), for two
// reasons: (1) serve's own boot-time chain-agent DISCOVERY currently no-ops —
// walking .contenox/ now goes through the workspace vfs, which refuses to read the
// runtime's own control plane (runtime/vfs/controlplane.go, the control-plane
// isolation invariant), so a fixture dropped in .contenox is not declared; and (2)
// `--auto` runs the unit's gated actions unattended, keeping the mission-tool turn
// deterministic and model-free. The chain files live in a PLAIN directory (not
// .contenox), read by the unit via CONTENOX_ACP_CHAIN_PATH — a direct file read,
// never the workspace vfs — so the control-plane guard never touches them. serve's
// disableVanished only reaps DISCOVERED rows, so these directly-created agents
// survive its boot pass untouched. The registry, not discovery, is what the fleet
// resolves a dispatch against.

// The three deterministic no-model chains, each destined for one exit code. Each is
// run by an `acp --auto` unit; its mission tools are granted per-mission at
// session/new, so these run unattended with no approval and no model.
const (
	scriptedLandsChain = `{
  "id": "agent-scripted-lands",
  "description": "Scripted e2e: finish the mission landed via mission_finish.",
  "tasks": [
    {
      "id": "finish",
      "handler": "tools",
      "tools": {"name": "mission", "tool_name": "mission_finish", "args": {"status": "landed", "reason": "scripted landing"}},
      "transition": {"branches": [{"operator": "default", "goto": "done"}]}
    },
    {"id": "done", "handler": "noop", "transition": {"branches": [{"operator": "default", "goto": "end"}]}}
  ]
}`

	scriptedBlockerChain = `{
  "id": "agent-scripted-blocker",
  "description": "Scripted e2e: file a blocker report (needs attention).",
  "tasks": [
    {
      "id": "block",
      "handler": "tools",
      "tools": {"name": "mission", "tool_name": "mission_report", "args": {"kind": "blocker", "summary": "scripted blocker needs attention"}},
      "transition": {"branches": [{"operator": "default", "goto": "done"}]}
    },
    {"id": "done", "handler": "noop", "transition": {"branches": [{"operator": "default", "goto": "end"}]}}
  ]
}`

	// The progress fixture files ONE progress report and then idles. A progress
	// report is intermediate — the unit is alive and still working — so `fire
	// --wait` never treats it as an outcome and rides to its deadline: exit 3. It
	// also never files a result/blocker/terminal status, so the exit-3 verdict does
	// not depend on the fixture's boot timing — the wait can only ever end at the
	// deadline. (One report also suppresses the mute-unit nudge, so no runtime
	// blocker ever appears to flip this to exit 2.)
	scriptedProgressChain = `{
  "id": "agent-scripted-progress",
  "description": "Scripted e2e: file a single progress report, then idle (intermediate only).",
  "tasks": [
    {
      "id": "progress",
      "handler": "tools",
      "tools": {"name": "mission", "tool_name": "mission_report", "args": {"kind": "progress", "summary": "scripted progress, still working"}},
      "transition": {"branches": [{"operator": "default", "goto": "done"}]}
    },
    {"id": "done", "handler": "noop", "transition": {"branches": [{"operator": "default", "goto": "end"}]}}
  ]
}`
)

// scriptedPolicy is the envelope every fire names. It ships with `contenox init`
// (the embedded HITL policies), so it resolves in the hermetic serve. The mission
// tools these fixtures call are not gated by it — it is required only because a
// mission must name an envelope.
const scriptedPolicy = "hitl-policy-default.json"

var missionIDPattern = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)

func TestSystem_ScriptedMissionSequence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scripted-mission system e2e: builds contenox and boots a real serve")
	}

	bin := buildScriptedContenoxBin(t)

	// Hermetic layout mirroring scripts/run_apitests.sh: HOME holds the db at its
	// default location, the workspace holds the serve data dir the fixtures live in.
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	workspaceDir := filepath.Join(root, "workspace")
	dataDir := filepath.Join(workspaceDir, ".contenox")
	dbPath := filepath.Join(homeDir, ".contenox", "local.db")
	require.NoError(t, os.MkdirAll(homeDir, 0o755))
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	// Env every child process inherits: an isolated HOME and a deliberately fake
	// default model (a dispatched chain unit's engine hard-requires one at boot but
	// the no-model fixtures never resolve it, so an accidental resolution fails loudly).
	baseEnv := append(os.Environ(),
		"HOME="+homeDir,
		"CONTENOX_DEFAULT_MODEL=scripted-e2e-fake-model",
		"CONTENOX_DEFAULT_PROVIDER=ollama",
	)

	// Seed the isolated state through the real CLI (init scaffolds .contenox + the
	// embedded HITL policies), then drop the three chains into a PLAIN directory (not
	// .contenox), read by the units via CONTENOX_ACP_CHAIN_PATH.
	runScriptedCLI(t, bin, baseEnv, "--data-dir", dataDir, "--db", dbPath, "init", "--force")
	chainsDir := filepath.Join(root, "chains")
	require.NoError(t, os.MkdirAll(chainsDir, 0o755))
	landsPath := filepath.Join(chainsDir, "lands.json")
	blockerPath := filepath.Join(chainsDir, "blocker.json")
	progressPath := filepath.Join(chainsDir, "progress.json")
	require.NoError(t, os.WriteFile(landsPath, []byte(scriptedLandsChain), 0o644))
	require.NoError(t, os.WriteFile(blockerPath, []byte(scriptedBlockerChain), 0o644))
	require.NoError(t, os.WriteFile(progressPath, []byte(scriptedProgressChain), 0o644))

	// Seed the three units into the registry directly as external `acp --auto`
	// agents (see the file doc). Done and closed BEFORE serve opens the db, so the
	// two never contend on the SQLite file.
	seedScriptedAgents(t, dbPath, bin, map[string]string{
		"agent-scripted-lands":    landsPath,
		"agent-scripted-blocker":  blockerPath,
		"agent-scripted-progress": progressPath,
	})

	port := freePort(t)
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	serveEnv := append(append([]string{}, baseEnv...),
		"ADDR=127.0.0.1",
		fmt.Sprintf("PORT=%d", port),
		"TOKEN=",
	)
	serveLog := startScriptedServe(t, bin, serveEnv, dataDir, dbPath, serverURL)

	// A helper bound to this serve: run `contenox mission fire …`. Every fire names
	// --server (no token) and passes --agent and --policy so it never opens the
	// local config db for defaults.
	fireArgs := func(agent string, extra ...string) []string {
		base := []string{
			"mission", "fire", "-q", "--wait",
			"--server", serverURL,
			"--agent", agent,
			"--policy", scriptedPolicy,
			"--intent", "scripted mission for " + agent,
			"--wait-interval", "300ms",
			"--wait-timeout", "90s",
		}
		return append(base, extra...)
	}

	// ── 1. fire(lands, --wait) → exit 0, stdout is a bare, correlatable id ────────
	t.Run("landed_exits_zero_and_id_is_machine_readable", func(t *testing.T) {
		stdout, stderr, code := runScripted(t, bin, baseEnv, fireArgs("agent-scripted-lands")...)
		require.Equalf(t, 0, code, "a landed mission must exit 0.\nstdout:%s\nstderr:%s\nserve log:\n%s", stdout, stderr, serveLog())

		mid := strings.TrimSpace(stdout)
		require.Regexp(t, missionIDPattern, mid, "with -q, stdout is exactly the mission id (a bare UUID)")
		require.NotContains(t, stdout, "Mission fired", "quiet mode prints only the id to stdout; prose goes to stderr")

		// The id is correlatable: feed it back to `mission show --json`.
		showOut, _, showCode := runScripted(t, bin, baseEnv,
			"mission", "show", "--server", serverURL, "--json", mid)
		require.Equal(t, 0, showCode, "the captured id resolves via `mission show`")
		require.Contains(t, showOut, mid)
		require.Contains(t, showOut, `"status": "landed"`, "the mission the id names really landed")
	})

	// ── 2. fire(lands) && fire(lands) → exit 0 (&& composes on success) ───────────
	t.Run("success_chains_with_and", func(t *testing.T) {
		script := shJoin(bin, fireArgs("agent-scripted-lands")) + " && " + shJoin(bin, fireArgs("agent-scripted-lands"))
		stdout, stderr, code := runScripted(t, "/bin/sh", baseEnv, "-c", script)
		require.Equalf(t, 0, code, "fire --wait && fire --wait must compose on success.\nstdout:%s\nstderr:%s", stdout, stderr)

		ids := missionIDsIn(stdout)
		require.Len(t, ids, 2, "both fires ran and each printed its mission id (the second because the first exited 0)")
		require.NotEqual(t, ids[0], ids[1], "each fire is a distinct mission")
	})

	// ── 3. fire(blocker) && fire(lands) → exit 2, the chain breaks ────────────────
	t.Run("blocker_breaks_the_chain", func(t *testing.T) {
		script := shJoin(bin, fireArgs("agent-scripted-blocker")) + " && " + shJoin(bin, fireArgs("agent-scripted-lands"))
		stdout, stderr, code := runScripted(t, "/bin/sh", baseEnv, "-c", script)
		require.Equalf(t, missionWaitBlocked, code, "a blocker exits 2 and breaks the && chain.\nstdout:%s\nstderr:%s", stdout, stderr)

		ids := missionIDsIn(stdout)
		require.Len(t, ids, 1, "only the blocker fire ran; the second fire never started because the first exited non-zero")
		require.Contains(t, stderr, "BLOCKED", "the wait's stderr names why it stopped")
	})

	// ── 4. fire(progress, short timeout) → exit 3 (indeterminate) ─────────────────
	t.Run("progress_only_times_out", func(t *testing.T) {
		stdout, stderr, code := runScripted(t, bin, baseEnv,
			fireArgs("agent-scripted-progress", "--wait-timeout", "4s", "--wait-interval", "300ms")...)
		require.Equalf(t, missionWaitTimeout, code, "a unit filing only intermediate reports rides to the deadline: exit 3.\nstdout:%s\nstderr:%s", stdout, stderr)
		require.Regexp(t, missionIDPattern, strings.TrimSpace(stdout), "the id is still printed to stdout before the wait blocks")
		require.Contains(t, stderr, "timed out")
	})
}

// buildScriptedContenoxBin compiles cmd/contenox into t.TempDir(); the go build
// cache makes reruns cheap.
func buildScriptedContenoxBin(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "contenox")
	out, err := exec.Command("go", "build", "-o", binPath, "github.com/contenox/runtime/cmd/contenox").CombinedOutput()
	require.NoErrorf(t, err, "build contenox:\n%s", out)
	return binPath
}

// seedScriptedAgents declares each named unit in the registry as an external
// `contenox acp --auto` agent bound to its chain file (via CONTENOX_ACP_CHAIN_PATH).
// The db handle is fully closed before returning, so serve can open the file cleanly.
func seedScriptedAgents(t *testing.T, dbPath, bin string, agentsToChain map[string]string) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, dbPath, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	registry := agentregistryservice.New(db)
	for name, chainPath := range agentsToChain {
		agent := &runtimetypes.Agent{Name: name, Enabled: true}
		require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
			Transport: runtimetypes.ExternalACPTransportStdio,
			Command:   bin,
			Args:      []string{"acp", "--auto"},
			Env:       map[string]string{"CONTENOX_ACP_CHAIN_PATH": chainPath},
		}))
		require.NoError(t, registry.Create(ctx, agent), "seed external agent %q", name)
	}
}

// runScriptedCLI runs a contenox setup command and fails on non-zero exit.
func runScriptedCLI(t *testing.T, bin string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "contenox %v:\n%s", args, out)
}

// runScripted runs name with args, returning (stdout, stderr, exit code). A
// non-ExitError failure (the binary could not be launched) fails the test.
func runScripted(t *testing.T, name string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !asExitError(err, &ee) {
			t.Fatalf("run %s %v: %v\nstderr:%s", name, args, err, stderr.String())
		}
		code = ee.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

// asExitError is errors.As specialized to *exec.ExitError, kept local so the one
// call site reads clearly.
func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

// shJoin renders a command line for `sh -c`, quoting each argument so intents with
// spaces survive the shell. bin is the program; args are its arguments.
func shJoin(bin string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shQuote(bin))
	for _, a := range args {
		parts = append(parts, shQuote(a))
	}
	return strings.Join(parts, " ")
}

// shQuote single-quotes a shell argument (wrapping embedded single quotes).
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// missionIDsIn returns every line of stdout that is a bare mission id — the -q
// output. Non-id lines (there should be none on stdout) are ignored.
func missionIDsIn(stdout string) []string {
	var ids []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if missionIDPattern.MatchString(line) {
			ids = append(ids, line)
		}
	}
	return ids
}

// freePort returns a currently-free TCP port on loopback. A small race window
// exists between close and serve's bind, acceptable for a test.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// serveBootOutcome is one boot attempt's result: ready means health returned 200;
// diedEarly means the process exited before readiness (a retryable transient).
type serveBootOutcome struct {
	log       func() string
	ready     bool
	diedEarly bool
}

// startScriptedServe launches `contenox serve` and health-polls it to readiness,
// retrying a boot that dies early (a transient SQLITE_BUSY on the freshly-created
// db has been seen on first open). On success it registers cleanup that terminates
// the whole process group (reaping the dispatched acp unit grandchildren) and
// returns a closure yielding the serve log.
func startScriptedServe(t *testing.T, bin string, env []string, dataDir, dbPath, serverURL string) func() string {
	t.Helper()
	const maxAttempts = 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		out := bootScriptedServeOnce(t, bin, env, dataDir, dbPath, serverURL)
		if out.ready {
			return out.log
		}
		if out.diedEarly && attempt < maxAttempts {
			t.Logf("serve boot attempt %d exited early (retrying):\n%s", attempt, out.log())
			continue
		}
		t.Fatalf("contenox serve never became ready (attempt %d):\n%s", attempt, out.log())
	}
	return func() string { return "" }
}

// bootScriptedServeOnce runs one serve boot attempt: start it in its own process
// group, poll /health, and register group-kill cleanup only if it comes up.
func bootScriptedServeOnce(t *testing.T, bin string, env []string, dataDir, dbPath, serverURL string) serveBootOutcome {
	t.Helper()
	var log bytes.Buffer
	cmd := exec.Command(bin, "--data-dir", dataDir, "--db", dbPath, "serve")
	cmd.Env = env
	cmd.Stdout = &log
	cmd.Stderr = &log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())

	pid := cmd.Process.Pid
	exited := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(exited) }()
	logFn := func() string { return log.String() }
	kill := func() {
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
		}
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}

	healthURL := serverURL + "/health"
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			kill()
			return serveBootOutcome{log: logFn, diedEarly: true}
		default:
		}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, healthURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			code := resp.StatusCode
			_ = resp.Body.Close()
			if code == http.StatusOK {
				t.Cleanup(kill)
				return serveBootOutcome{log: logFn, ready: true}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	// Poll timed out with the process still alive: not a transient.
	kill()
	return serveBootOutcome{log: logFn}
}
