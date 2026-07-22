package contenoxcli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"os/signal"

	"github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/apiframework/middleware"
	"github.com/contenox/runtime/libacp"
	libbus "github.com/contenox/runtime/libbus"
	"github.com/contenox/runtime/libkvstore"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/acpsvc"
	"github.com/contenox/runtime/runtime/agentinstance"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/chainagents"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/fleetservice"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/internal/compatapi"
	"github.com/contenox/runtime/runtime/internal/fleetapi"
	internaltools "github.com/contenox/runtime/runtime/internal/tools"
	internalweb "github.com/contenox/runtime/runtime/internal/web"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/missionchanges"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/operatorinbox"
	"github.com/contenox/runtime/runtime/presence"
	"github.com/contenox/runtime/runtime/reportrouter"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/serverapi"
	"github.com/contenox/runtime/runtime/shellsession"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskchainservice"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/contenox/runtime/runtime/terminalservice"
	"github.com/contenox/runtime/runtime/toolsproviderservice"
	"github.com/contenox/runtime/runtime/version"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/contenox/runtime/runtime/workspacegrants"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	defaultServeAddr = "127.0.0.1"
	defaultServePort = "32123"
	// remoteBindAddr is the bind host --remote selects: all interfaces, so other
	// machines on the LAN can reach the API and Beam UI.
	remoteBindAddr = "0.0.0.0"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Contenox HTTP server and Beam UI.",
	Long: `Start the Contenox HTTP API server.

Foundation routes:
  GET /health
  GET /version

The product API is served under /api (state, models, model-registry, backends,
setup-status, providers, tools, mcp-servers, task-chains, hitl-policies, task
execution, task events, terminal sessions). Chat, its HITL approvals, and
execution-state/event replay run over the /acp WebSocket instead (see
'contenox acp'). The Beam web UI is served at /.

OpenAI- and Ollama-compatible endpoints are served for external clients:
point an OpenAI client's base URL at the server root (POST /v1/chat/completions,
GET /v1/models; also under /api/openai) or an Ollama client at the same host
(GET /api/tags, POST /api/chat, POST /api/generate). Requests execute through
the configured default chain and runtime defaults; a requested model is honored
when the default provider serves it, otherwise the default model is used.

Terminal routes are enabled by default on local serve under /api/terminal/sessions.
Set TERMINAL_ENABLED=false to disable them. TERMINAL_ALLOWED_ROOT defaults to the
workspace root. TERMINAL_MAX_SESSIONS defaults to 8 (0 = unlimited).

The server binds to 127.0.0.1:32123 by default (local-only). Pass --remote to
bind all interfaces (0.0.0.0) so other machines on your LAN can reach it, or set
ADDR/PORT to bind a specific address. TOKEN gates every API request and the Beam
login, and is MANDATORY for any non-loopback bind (--remote or a LAN ADDR). The
token is resolved as: TOKEN env, else ~/.contenox/serve-token.txt, else (with
--remote) a freshly generated token saved there (0600); programmatic clients
(contenox approvals/mission/fleet) discover the same file automatically. Serving
is plain HTTP — put a TLS-terminating reverse proxy in front for untrusted
networks. Set BEAM_DEV_PROXY_URL to proxy Beam UI requests to a Vite dev server
while keeping /api on this server. A configured model is required — run
` + "`contenox setup`" + ` first if you have not configured one.`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringArray("workspace-root", nil,
		"Directory a browser client may choose as a session workspace (repeatable). The serve directory is always the default; these extend the allowlist. Also settable via WORKSPACE_ROOTS (OS path-list separated) or as `contenox serve [dir]...` positional arguments.")
	serveCmd.Flags().Bool("remote", false,
		"Serve on all network interfaces (0.0.0.0) so other machines on your LAN can reach the API and Beam UI. Auto-provisions a TOKEN (env, else ~/.contenox/serve-token.txt, else generated and saved there) since a token is mandatory off loopback. A shorthand for ADDR=0.0.0.0; an explicit ADDR still wins. Plain HTTP — front with a TLS reverse proxy for untrusted networks.")
}

// resolveServeAddr picks the bind host for `contenox serve`. An explicitly
// configured ADDR (env) always wins; otherwise --remote binds all interfaces
// (0.0.0.0) so the server is reachable on the LAN, and the default is
// loopback-only (127.0.0.1). Binding a non-loopback address requires a TOKEN
// (enforced by ValidateLocalServeSecurity), so --remote without a TOKEN is
// refused before this ever binds.
func resolveServeAddr(remote bool, envAddr string) string {
	if a := strings.TrimSpace(envAddr); a != "" {
		return a
	}
	if remote {
		return remoteBindAddr
	}
	return defaultServeAddr
}

// lanURLs returns browsable http URLs for each non-loopback IPv4 the host has —
// the addresses other machines actually use to reach a --remote serve, since the
// bind banner shows the unhelpful "0.0.0.0". Best-effort: an interface-enumeration
// error yields nil and the banner just omits the list.
func lanURLs(port string) []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var urls []string
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			urls = append(urls, "http://"+net.JoinHostPort(ip4.String(), port))
		}
	}
	return urls
}

// buildWorkspaceBaseRoots assembles the LAUNCH-time workspace-root set — the
// part of the allowlist fixed at serve start. defaultRoot is the effective
// workspace root (the served project directory when one is given positionally,
// else home) and is always FIRST, making it the Factory default. It already IS
// the first positional serve arg (resolved) when one was given, so only
// positional args BEYOND the first extend the set here — home is never injected
// as an extra root when a project is served. --workspace-root flags and the
// WORKSPACE_ROOTS env (OS path-list separated) also extend it.
//
// This is the BASE the hot-reload path (workspaceRootReloader) always prepends;
// the durable, runtime-mutable grants (workspacegrants) are appended to it, so
// defaultRoot stays roots[0] across every reload and a grant never displaces the
// default. Duplicates are collapsed by the Factory.
func buildWorkspaceBaseRoots(cmd *cobra.Command, args []string, config *serverapi.Config, defaultRoot string) []string {
	roots := []string{defaultRoot}
	if len(args) > 1 {
		roots = append(roots, args[1:]...)
	}
	if flags, _ := cmd.Flags().GetStringArray("workspace-root"); len(flags) > 0 {
		roots = append(roots, flags...)
	}
	if config != nil {
		for _, r := range filepath.SplitList(config.WorkspaceRoots) {
			if strings.TrimSpace(r) != "" {
				roots = append(roots, r)
			}
		}
	}
	return roots
}

func runServe(cmd *cobra.Command, args []string) error {
	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, stop := signal.NotifyContext(baseCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Drain registered model-backend shutdown hooks on exit (no-op when none).
	defer func() { _ = modelrepo.Shutdown() }()

	config := &serverapi.Config{}
	if err := serverapi.LoadConfig(config); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	remote, _ := cmd.Flags().GetBool("remote")
	config.Addr = resolveServeAddr(remote, config.Addr)
	if config.Port == "" {
		config.Port = defaultServePort
	}
	config.Token = strings.TrimSpace(config.Token)
	// Token resolution, the serve-token.txt convention: an explicit TOKEN env wins;
	// else the token persisted at ~/.contenox/serve-token.txt (shared with
	// programmatic clients, and control-plane-denied so no agent can read it); else
	// --remote provisions one — it mandates a token — by generating and persisting
	// it. Loopback with nothing configured stays zero-friction (no token, no login).
	if config.Token == "" {
		if fileTok := readServeTokenFile(); fileTok != "" {
			config.Token = fileTok
			fmt.Fprintf(cmd.OutOrStdout(), "serve token: loaded from %s\n", serveTokenPathHint())
		} else if remote {
			gen, err := generateServeToken()
			if err != nil {
				return fmt.Errorf("generate serve token: %w", err)
			}
			if err := writeServeTokenFile(gen); err != nil {
				return fmt.Errorf("persist serve token to %s: %w", serveTokenPathHint(), err)
			}
			config.Token = gen
			fmt.Fprintf(cmd.OutOrStdout(),
				"serve token: generated and saved to %s (0600) — clients auto-discover it there; `cat` it to log in, or set TOKEN to override\n",
				serveTokenPathHint())
		}
	}
	// Defensive backstop: --remote (any non-loopback bind) requires a token. After
	// the resolution above this only fires if generation/persistence was skipped.
	if remote && config.Token == "" {
		return fmt.Errorf("--remote binds all network interfaces (%s) and requires a TOKEN; set one, or ensure %s is writable so serve can provision it",
			remoteBindAddr, serveTokenPathHint())
	}
	if err := serverapi.ValidateLocalServeSecurity(config.Addr, config.Token); err != nil {
		return err
	}

	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return err
	}
	dbCtx := libtracker.WithNewRequestID(ctx)
	db, err := OpenDBAt(dbCtx, dbPath)
	if err != nil {
		return fmt.Errorf("open database %q: %w", dbPath, err)
	}
	defer db.Close()

	contenoxDir, err := ResolveContenoxDir(cmd)
	if err != nil {
		return fmt.Errorf("resolve .contenox dir: %w", err)
	}
	workspaceID := ResolveWorkspaceID(contenoxDir)
	// The effective workspace root is the cwd for every chat session (native and
	// external), the local-exec allowed dir, the terminal allowed root, the
	// /files ProjectRoot, and the Factory default. When a project directory is
	// served positionally it BECOMES that root, bounding all of the above to the
	// project. Only the argument-less `contenox serve` falls back to the parent
	// of ~/.contenox (home), preserving today's behavior.
	workspaceRoot := filepath.Dir(contenoxDir)
	if len(args) > 0 {
		servedDir, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolve serve directory %q: %w", args[0], err)
		}
		workspaceRoot = servedDir
	}
	nodeID := uuid.NewString()[:8]

	store := runtimetypes.New(db.WithoutTransaction())
	closeLogs, err := setupTelemetryLogging(ctx, store, contenoxDir)
	if err != nil {
		return fmt.Errorf("setup telemetry logging: %w", err)
	}
	defer closeLogs()

	opts, err := buildRunOpts(cmd, db, contenoxDir)
	if err != nil {
		return err
	}
	// serve offers the local_shell tool by default (the Beam chat workspace
	// expects it); it is gated by the HITL policy (EnableHITL below). Default
	// the exec allowed-dir to the workspace root so commands cannot escape it.
	opts.EffectiveEnableLocalExec = true
	if opts.EffectiveLocalExecAllowedDir == "" {
		opts.EffectiveLocalExecAllowedDir = workspaceRoot
	}

	// The workspace-root allowlist bounds what a browser client may choose as a
	// session's workspace. The serve directory is the default root, so behavior
	// is unchanged when nothing else is configured. The launch-time BASE (serve
	// dir + args + flags + env) is combined with the DURABLE grants an operator
	// added through `contenox workspace` / POST /workspace/roots, so a grant made
	// while serve was down is honored at boot. The reloader (wired to the bus
	// doorbell below) re-applies base ∪ grants whenever a grant changes, without a
	// restart.
	workspaceBaseRoots := buildWorkspaceBaseRoots(cmd, args, config, workspaceRoot)
	effectiveWorkspaceRoots := append(append([]string{}, workspaceBaseRoots...),
		workspacegrants.ReadGrants(dbCtx, store)...)
	workspaceFactory, err := vfs.NewFactory(effectiveWorkspaceRoots...)
	if err != nil {
		return fmt.Errorf("resolve workspace roots: %w", err)
	}
	// Control-plane isolation (vfs-invariant slice, 2026-07-21). Register the
	// runtime's OWN state dirs (~/.contenox: config, database, HITL policies,
	// declared agents, models) so NO session, browse root, or agent fs tool can
	// reach them — even though the default workspace root is the PARENT of the
	// .contenox dir, which containment (landed today) would otherwise let a client
	// browse into. Set ONCE here, before serving; it is process-global and NOT part
	// of the mutable root set, so it survives every SetRoots hot-reload the
	// reloader performs. See runtime/vfs/controlplane.go.
	if err := vfs.SetControlPlaneDenied(controlPlaneDirs(contenoxDir)...); err != nil {
		return fmt.Errorf("register control-plane isolation: %w", err)
	}
	workspaceReloader := newWorkspaceRootReloader(workspaceFactory, workspaceBaseRoots, store)

	var tracker libtracker.ActivityTracker = libtracker.NoopTracker{}
	if opts.EffectiveTracing {
		tracker = libtracker.NewLogActivityTracker(slog.Default())
	}

	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer bus.Close()
	kvMgr := libkvstore.NewSQLiteManager(db)

	// Fleet presence: the WHOLE fleet on the board, not only kernel-dispatched
	// units. Editor-spawned contenox processes (`contenox acp`, `vscode-agent`)
	// self-register into this shared-SQLite store; serve is both a WRITER (it
	// registers its own row, kind serve, for symmetry) and the READER the board
	// join below surfaces the observed section from. Best-effort throughout — a
	// presence write never blocks serve.
	presenceStore := presence.NewStore(kvMgr)
	presenceReporter := presence.StartReporter(ctx, presenceStore, presence.Record{
		Kind: presence.KindServe,
		Cwd:  workspaceRoot,
		// The reachable address a sibling process discovers this serve at (a
		// standalone `contenox acp` forwarding `/mission` reads it from this shared
		// store when CONTENOX_SERVER_URL is unset). config.Addr/Port are already
		// defaulted to the loopback bind above, so this is the concrete host:port
		// serve listens on. The token is NOT advertised — presence is world-readable.
		Address: net.JoinHostPort(config.Addr, config.Port),
	})
	defer presenceReporter.Stop()

	// The workspace-root reload doorbell. `contenox workspace add/remove` (a
	// SECOND process) and POST/DELETE /workspace/roots both write the durable
	// grant config and publish workspacegrants.RootsChangedSubject on this shared
	// SQLite bus; this subscriber re-reads the config and swaps the live Factory's
	// root set, so a grant applies without restarting serve. Best-effort and
	// off-band: a failed reload logs and keeps the current roots.
	stopWorkspaceReload, err := startWorkspaceRootReloader(ctx, bus, workspaceReloader)
	if err != nil {
		return fmt.Errorf("start workspace-root reloader: %w", err)
	}
	defer stopWorkspaceReload()

	localTools := map[string]taskengine.ToolsRepo{
		"echo":     localtools.NewEchoTools(),
		"print":    localtools.NewPrint(tracker),
		"webtools": localtools.NewWebCaller(tracker),
		// local_fs roots at the session's chosen workspace (its cwd), falling back
		// to the default workspace root for sessions without one. The fixed root is
		// intentionally empty so the cwd resolver, not a static dir, drives
		// containment — mirroring the stdio ACP path, but resolving the cwd from
		// the DB because serve shares one tool across many per-connection
		// transports (there is no single Transport to close over).
		"local_fs": localtools.NewLocalFSToolsWith(
			"", db, nil, "local_fs",
			acpsvc.NewServeCwdResolver(db, workspaceFactory),
		),
	}
	if opts.EffectiveEnableLocalExec {
		execOpts := []localtools.LocalExecOption{}
		if opts.EffectiveLocalExecAllowedDir != "" {
			execOpts = append(execOpts, localtools.WithLocalExecAllowedDir(opts.EffectiveLocalExecAllowedDir))
		}
		localTools["local_shell"] = localtools.NewLocalExecTools(execOpts...)
	}

	// The shell-session surface (persistent per-chat PTY behind the terminal
	// panel, the `!` passthrough, and the shell_session_run/read agent tools)
	// shares the same enablement gate as local_shell. Each shell is rooted at its
	// session's workspace via the same cwd resolver local_fs uses. When disabled,
	// the tools are absent and the terminal extension methods report
	// method-not-found — the feature is absent, not broken.
	var shellSessions shellsession.Manager
	if opts.EffectiveEnableLocalExec {
		shellSessions = shellsession.NewManager(shellsession.Config{
			CwdResolver: acpsvc.NewServeCwdResolver(db, workspaceFactory),
			Workspace:   workspaceFactory,
		})
		defer shellSessions.Shutdown()
		localTools[shellsession.ToolsProviderName] = shellsession.NewTools(shellSessions)
	}

	toolsRepo := internaltools.NewPersistentRepo(localTools, db, http.DefaultClient, bus, tracker)
	toolsProviderSvc := toolsproviderservice.New(db, toolsRepo, tracker)

	// Journal events durably per request (console scrollback evidence: diffs,
	// approvals, tool calls) in addition to the live bus stream served by SSE.
	taskEventSink := taskengine.NewKVJournalTaskEventSink(
		taskengine.NewBusTaskEventSink(bus), kvMgr, tracker)
	hitlSource := hitlPolicySource(contenoxDir)
	hitlSvc := hitlservice.NewWithDefaultPolicy(hitlSource, runtimetypes.LocalTenantID, store, tracker, "")
	approvalCeiling, err := parseHITLApprovalCeiling(config.HITLApprovalTimeout)
	if err != nil {
		return err
	}
	hitlservice.SetApprovalCeiling(hitlSvc, approvalCeiling)
	// The durability backstop for pending approvals: resolves any row whose
	// deadline (rule TimeoutS or the ceiling just above) has passed, applying
	// its stored OnTimeout. Covers both a requester whose own bounded wait
	// already returned without touching the row, and one whose process
	// restarted before it could. Mirrors startTerminalReaper's shape below.
	stopHITLApprovalSweeper := startHITLApprovalSweeper(ctx, hitlSvc, hitlApprovalSweepInterval)
	defer stopHITLApprovalSweeper()

	// serve runs many ACP WebSocket connections (each its own acpsvc.Transport)
	// behind this SINGLE shared engine, so the engine's one AskApproval callback
	// cannot close over a single transport the way the stdio ACP path does. The
	// router is a stable shared object created BEFORE the engine (no late-binding
	// gymnastics): the per-connection transports built later (acpsvc.New below)
	// register their live sessions into it, and the AskApproval closure consults
	// it per request. Nil out of the box until a transport binds a session.
	permissionRouter := acpsvc.NewPermissionRouter()

	engine, err := enginesvc.Build(ctx, db, enginesvc.Config{
		DefaultModel:       opts.EffectiveDefaultModel,
		DefaultProvider:    opts.EffectiveDefaultProvider,
		AltDefaultModel:    opts.EffectiveAltDefaultModel,
		AltDefaultProvider: opts.EffectiveAltDefaultProvider,
		ContextLength:      opts.EffectiveContext,
		NoDeleteModels:     opts.EffectiveNoDeleteModels,
		LocalTools:         localTools,
		EnableHITL:         true,
		// Dispatch HITL approval per request: when the contenox session (from ctx)
		// is bound to a live beam ACP WS transport, bridge to that connection's
		// session/request_permission flow (beam's PermissionGate answers it).
		// Otherwise fall back to hitlSvc.RequestApproval for headless/API
		// callers, which have no live ACP connection to prompt: it is durable
		// (the ask survives a restart, see hitl_approvals in schema.sql) and
		// bounded (a matched rule's own TimeoutS, or the HITL_APPROVAL_TIMEOUT
		// ceiling below when it sets none), so this no longer hangs forever.
		// A REST/CLI surface now answers it early — GET/POST /api/approvals,
		// `contenox approvals list|answer` (fleet-consolidation.md slice C2,
		// wired below via serverapi.Dependencies.HITL) — and until an operator
		// uses it, an unanswered ask is still resolved automatically once its
		// deadline passes (startHITLApprovalSweeper below).
		AskApproval: func(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
			if allowed, err := permissionRouter.AskApproval(ctx, req); !errors.Is(err, acpsvc.ErrNoBoundSession) {
				return allowed, err
			}
			return hitlSvc.RequestApproval(ctx, req, taskEventSink)
		},
		HITLService:      hitlSvc,
		Bus:              bus,
		KVStore:          kvMgr,
		Tracker:          tracker,
		Tracing:          opts.EffectiveTracing,
		TaskEventSink:    taskEventSink,
		WorkspaceID:      workspaceID,
		HITLPolicySource: hitlSource,
	})
	if err != nil {
		return fmt.Errorf("build engine (run `contenox setup` to configure a model): %w", err)
	}
	if engine.State == nil {
		return fmt.Errorf("build engine: runtime state is not configured")
	}
	defer engine.Stop()

	chainFiles, err := localfileservice.NewPrivileged(contenoxDir)
	if err != nil {
		return fmt.Errorf("chain files: %w", err)
	}
	chains := taskchainservice.NewLocal(chainFiles)
	agent := agentservice.New(agentservice.Deps{
		Engine:      engine,
		DB:          db,
		WorkspaceID: workspaceID,
	})
	terminalCfg, err := resolveTerminalConfig(config, workspaceRoot)
	if err != nil {
		return err
	}
	terminalSvc, err := terminalservice.New(terminalCfg, db, nodeID, workspaceID)
	if err != nil {
		return fmt.Errorf("create terminal service: %w", err)
	}
	if terminalCfg.Enabled {
		terminalSvc = terminalservice.WithActivityTracker(terminalSvc, tracker)
		defer func() { _ = terminalSvc.CloseAll(context.Background()) }()
		stopTerminalReaper := startTerminalReaper(ctx, terminalSvc, terminalReapInterval(terminalCfg.IdleTimeout))
		defer stopTerminalReaper()
	}

	// External-agent instances live OFF any single connection: the Manager owns each
	// declared external agent's subprocess on its own root context, so an agent's
	// process (and thus its context) survives a browser disconnect/reload and a
	// reloaded session re-attaches to the still-running instance. Owned by the serve
	// process and torn down (every subprocess killed) on shutdown.
	// Passive fleet telemetry: lifecycle facts (state changes, attach/detach,
	// unsupervised denies) are reported through the shared tracker for
	// after-the-fact audit — recorded, triggering nothing. Noop unless tracing is
	// enabled, like every other subsystem here.
	//
	// agentRegistry is shared between the Manager (agent resolution/spawn) and
	// fleetservice (the Enabled policy check) below, rather than constructing a
	// second agentregistryservice.Service over the same db.
	agentRegistry := agentregistryservice.New(db)

	// Durable mission records — the manifest's other half. Backed by the same DB;
	// missions outlive the sessions/instances they reference. Built BEFORE the
	// Manager because the unattended-permission answerer wired into it below
	// resolves a unit's envelope from its mission.
	//
	// The bus wiring is the supervision edge's producer half: AddReport publishes
	// a ReportAddedEvent that the report router (below) consumes to deliver a
	// report to whoever fired the mission — a live parent session, or the operator
	// inbox. Best effort by contract: a publish failure never fails AddReport,
	// because the report is already the durable fact.
	missions := missionservice.New(db, missionservice.WithEventPublisher(bus))

	instances := agentinstance.New(
		agentRegistry,
		agentinstance.WithEventSink(newInstanceEventSink(tracker)),
		// THE HITL PATH FOR UNATTENDED UNITS. A dispatched unit runs with no
		// viewer attached by design, so its permission requests reach a session
		// with no controller — which the kernel denies. That silently killed
		// every mission at its first gated action and left the approval inbox
		// empty. This fallback answers those requests instead: it resolves the
		// unit's mission, evaluates that mission's HITL policy (its envelope),
		// and either answers inside the bounds the operator declared or raises a
		// durable ask that GET/POST /api/approvals and `contenox approvals`
		// serve. The kernel learns nothing about approvals from this — it calls
		// an injected function and returns what comes back.
		agentinstance.WithPermissionFallback(fleetservice.NewUnattendedPermissionAnswerer(
			fleetservice.UnattendedPermissionDeps{
				HITL:     hitlSvc,
				Missions: missions,
				Sink:     taskEventSink,
				// serve passes no fallbackPolicy to hitlservice above ("" = the
				// service's own built-in default), so a unit with no mission
				// behind it is governed by that same chain rather than by a
				// second rule set invented here.
				DefaultPolicyName: "",
				Tracker:           tracker,
			})),
	)
	defer func() { _ = instances.Close() }()

	// Seed the registry from the operator's own task chains, so the chains they
	// already author are fireable as fleet units without a second registration
	// step. It SEEDS ONLY: nothing in the spawn path learns a second lookup, so
	// the registry stays the single source of truth for what can be fired.
	// Workspace .contenox/ shadows ~/.contenox/, the precedence every other
	// config file here follows. A failure must not take serve down — the fleet
	// keeps working with whatever the registry already holds.
	discoverChainAgents(ctx, agentRegistry, contenoxDir)

	// The fleet's lifecycle-policy layer (runtime/fleetservice), wrapping
	// instances: the /fleet REST routes below are a thin surface over this.
	fleet := fleetservice.New(instances, agentRegistry, missions, workspaceFactory, workspaceRoot, tracker,
		// Refuse a dispatch that names a nonexistent HITL envelope, over the same
		// policy source the approval gate reads.
		fleetservice.WithPolicyValidator(hitlservice.NewPolicyValidator(hitlSource, runtimetypes.LocalTenantID, "")))

	// The supervision edge's delivery half. A durable operator inbox for reports
	// that reached no live supervisor (surfaced by /operator-inbox below), and the
	// report router that consumes the ReportAddedEvent missions publishes and
	// routes each report: into its parent session's stream when one fired the
	// mission (the Manager is the SessionDeliverer), or into the inbox when an
	// operator fired directly or the parent session has since ended. The router
	// runs off the bus, so nothing it does can fail the AddReport that produced
	// the event — routing is best-effort delivery on top of a durable report.
	operatorInbox := operatorinbox.New(db)
	reportRouter, err := reportrouter.New(reportrouter.Deps{
		Bus:      bus,
		Sessions: instances,
		Inbox:    operatorInbox,
		Tracker:  tracker,
	})
	if err != nil {
		return fmt.Errorf("build report router: %w", err)
	}
	stopReportRouter, err := reportRouter.Start(ctx)
	if err != nil {
		return fmt.Errorf("start report router: %w", err)
	}
	defer stopReportRouter()

	// /acp serves the same acpsvc agent `contenox acp` speaks over stdio, over
	// a WebSocket instead — see acp_ws.go. It reuses the engine/db/workspace
	// already built above; only the ACP chain registry is looked up fresh
	// (it lives outside enginesvc.Config). A missing chain file must not take
	// serve down: log and skip registering /acp rather than failing startup.
	var acpAgentFactory libacp.AgentFactory
	if acpChains, err := acpsvc.LoadChainRegistry(); err != nil {
		slog.Warn("contenox serve: /acp transport disabled: ACP chain registry unavailable", "error", err)
	} else {
		acpAgentFactory = acpsvc.New(acpsvc.Deps{
			Engine:             engine,
			DB:                 db,
			ChainRegistry:      acpChains,
			DefaultModel:       opts.EffectiveDefaultModel,
			DefaultProvider:    opts.EffectiveDefaultProvider,
			DefaultAltModel:    opts.EffectiveAltDefaultModel,
			DefaultAltProvider: opts.EffectiveAltDefaultProvider,
			DefaultMaxTokens:   opts.EffectiveMaxTokens,
			DefaultThink:       opts.EffectiveThink,
			WorkspaceID:        workspaceID,
			ContenoxDir:        contenoxDir,
			WorkspaceRoots:     workspaceFactory,
			ShellSessions:      shellSessions,
			// Share the same router the engine's AskApproval consults, so each WS
			// connection's transport registers its live sessions and gated tool
			// calls route back to the client that raised them.
			PermissionRouter: permissionRouter,
			// External-agent sessions attach to Manager-owned instances that survive
			// client disconnect/reload (a reload re-attaches to the same instance).
			Instances: instances,
			// The `/mission` slash command fires through the same fleetservice and
			// resolves agent names through the same registry the REST/CLI paths use —
			// one dispatch implementation, one source of "what can I fire".
			Fleet:         fleet,
			Agents:        agentRegistry,
			KnownPolicies: embeddedPolicyNames(),
			// serve passes no fallbackPolicy to hitlservice.NewWithDefaultPolicy
			// above ("" = the service's own built-in default), so there is no
			// named policy to report here either; HITLDefaultPolicyName is
			// display-only (the ACP /policy command).
			HITLDefaultPolicyName: "",
		})
	}

	runtimeDefaults := stateservice.RuntimeDefaults{
		ChainRef:    firstNonEmptyStr(opts.EffectiveChain, "default-chain.json"),
		Model:       opts.EffectiveDefaultModel,
		Provider:    opts.EffectiveDefaultProvider,
		AltModel:    opts.EffectiveAltDefaultModel,
		AltProvider: opts.EffectiveAltDefaultProvider,
		MaxTokens:   opts.EffectiveMaxTokens,
		Think:       opts.EffectiveThink,
	}

	apiMux := http.NewServeMux()
	cleanupAPI, err := serverapi.New(ctx, apiMux, nodeID, "local", config, serverapi.Dependencies{
		DB:                   db,
		PubSub:               bus,
		State:                engine.State,
		ToolsProviderService: toolsProviderSvc,
		Agent:                agent,
		Chains:               chains,
		Fleet:                fleet,
		Missions:             missions,
		// The attention layer's per-mission changed-files/diff/scope view, folded
		// from the live kernel journal `instances` owns and the mission→session
		// binding `missions` holds. The kernel's SessionJournal read is an optional
		// capability off the Manager interface (the SessionAgentText precedent), so
		// the concrete Manager is reached by assertion here rather than widening the
		// lifecycle interface every sibling mock would then have to satisfy.
		MissionChanges: missionchanges.New(missions, instances.(missionchanges.SessionJournalReader)),
		// The /operator-inbox read surface: reports the router landed here because
		// they reached no live supervisor (operator-fired missions, or a parent
		// session that had ended). Same durable store the router above writes.
		OperatorInbox: operatorInbox,
		// The /approvals inbox (fleet-consolidation.md slice C2): the same
		// hitlSvc the engine's AskApproval falls back to above, so answering
		// a pending ask over REST/CLI resolves the exact row a headless
		// requester is (or was) parked on.
		HITL:            hitlSvc,
		Tracker:         tracker,
		TerminalService: terminalSvc,
		TerminalEnabled: terminalCfg.Enabled,
		WorkspaceID:     workspaceID,
		ProjectRoot:     workspaceRoot,
		ContenoxDir:     contenoxDir,
		WorkspaceRoots:  workspaceFactory,
		// The authenticated grant verbs (POST/DELETE /workspace/roots): persist a
		// grant, apply it to workspaceFactory live, and ring the reload doorbell —
		// the LAN operator's browser-side equivalent of `contenox workspace`.
		WorkspaceRootMutators: workspaceReloader.mutators(bus),
		// Feed the /files `agent` view filter the same HITL policy source serve
		// uses so its verdicts match the live agent's gates. Fallback policy is ""
		// (the service's built-in default), mirroring hitlSvc above.
		HITLPolicySource:      hitlSource,
		HITLDefaultPolicyName: "",
		Defaults:              runtimeDefaults,
	})
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}
	defer func() { _ = cleanupAPI() }()

	// The board's OBSERVED section, additive beside serverapi's GET /fleet (which
	// is unchanged): GET /api/fleet/presence lists the self-registered
	// editor/serve instances, marked external (observed, not kernel-managed). The
	// literal /fleet/presence is more specific than serverapi's GET
	// /fleet/{instanceID}, so the two coexist on apiMux without conflict.
	fleetapi.AddPresenceRoutes(apiMux, presenceStore)

	rootMux := http.NewServeMux()
	serverapi.AddHealthRoutes(rootMux)
	serverapi.AddVersionRoutes(rootMux, version.Get(), nodeID, "local")
	// When a TOKEN is configured, EVERY /api/* request (all methods, incl. GET)
	// requires a valid credential — a session-cookie JWT or the raw token as a
	// bearer — closing the same-origin-read hole. Without a token (loopback dev),
	// browser-originated mutations must be same-origin or explicitly allowed.
	// StripPrefix lets route packages register clean paths (/state, /models, ...).
	rootMux.Handle("/api/", http.StripPrefix("/api", serverapi.ProtectAPI(config.Token, config.AllowedAPIOrigins, apiMux)))
	// Beam remote-access login: /ui/login issues an HttpOnly session cookie for
	// the configured TOKEN, /ui/logout clears it, /ui/auth-status reports whether
	// login is required and the caller is authenticated. Registered directly on
	// the root mux (not under the /api protection wrapper) so /ui/login is
	// reachable before the browser holds the cookie it issues. stdio ACP is
	// unaffected — these routes exist only on the serve HTTP surface.
	serverapi.AddUIAuthRoutes(rootMux, config.Token)
	// OpenAI- and Ollama-compatible aliases at the root, for clients configured
	// with a bare base URL (OpenAI: /v1/*; Ollama-native: /api/tags, /api/chat,
	// ...). These literal patterns are more specific than the /api/ subtree
	// handler above, so they win without shadowing any /api product route (no
	// product route registers /chat, /tags, /ps, /show, or /generate). Mutating
	// compat routes enforce config.Token themselves (authorizeCompatRequest);
	// the /api/openai/* form is registered inside serverapi.New behind
	// ProtectAPI like the rest of the product API.
	compatDeps := compatapi.CompatDeps{
		Agent:        agent,
		Chains:       chains,
		StateService: stateservice.New(engine.State, db, workspaceID),
		Defaults:     runtimeDefaults,
		Token:        config.Token,
	}
	compatapi.AddRootRoutes(rootMux, compatDeps)
	compatapi.AddOllamaRoutes(rootMux, compatDeps)
	if acpAgentFactory != nil {
		rootMux.Handle("/acp", acpWebSocketHandler(acpAgentFactory, config.Token))
	}
	uiHandler := internalweb.SPAHandler()
	if strings.TrimSpace(config.BeamDevProxyURL) != "" {
		devProxy, err := internalweb.DevProxyHandler(config.BeamDevProxyURL)
		if err != nil {
			return fmt.Errorf("beam dev proxy: %w", err)
		}
		uiHandler = devProxy
		fmt.Fprintf(cmd.OutOrStdout(), "Beam dev proxy: %s\n", config.BeamDevProxyURL)
	}
	rootMux.Handle("/", uiHandler)

	handler := middleware.EnableCORS(&middleware.CORSConfig{
		AllowedAPIOrigins: firstNonEmptyStr(config.AllowedAPIOrigins, middleware.DefaultAllowedAPIOrigins),
		AllowedMethods:    middleware.DefaultAllowedMethods,
		AllowedHeaders:    middleware.DefaultAllowedHeaders,
		ProxyOrigin:       config.ProxyOrigin,
	}, apiframework.RequestIDMiddleware(rootMux))

	srv := &http.Server{
		Addr:              net.JoinHostPort(config.Addr, config.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(cmd.OutOrStdout(), "contenox serve %s ready: http://%s\n", version.Get(), srv.Addr)
		// Off loopback (--remote or a LAN ADDR) the "0.0.0.0" bind is not browsable,
		// so print the concrete LAN URLs other machines use, and flag the plain-HTTP
		// exposure (the TOKEN travels in cleartext).
		if !serverapi.IsLoopbackAddress(config.Addr) {
			for _, u := range lanURLs(config.Port) {
				fmt.Fprintf(cmd.OutOrStdout(), "  reachable on LAN: %s\n", u)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  note: plain HTTP — the TOKEN travels in cleartext; front with a TLS reverse proxy on untrusted networks\n")
		}
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("serve: %w", err)
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func resolveTerminalConfig(config *serverapi.Config, workspaceRoot string) (terminalservice.Config, error) {
	if config == nil {
		config = &serverapi.Config{}
	}
	terminalEnabled := strings.TrimSpace(config.TerminalEnabled)
	if terminalEnabled == "" {
		terminalEnabled = "true"
	}
	allowedRoot := strings.TrimSpace(config.TerminalAllowedRoot)
	if terminalservice.IsEnabled(terminalEnabled) && allowedRoot == "" {
		allowedRoot = workspaceRoot
	}
	cfg, err := terminalservice.ParseEnv(
		terminalEnabled,
		allowedRoot,
		config.TerminalShell,
		config.TerminalIdleTimeout,
		config.TerminalMaxSessions,
	)
	if err != nil {
		return terminalservice.Config{}, err
	}
	return cfg, nil
}

func terminalReapInterval(idleTimeout time.Duration) time.Duration {
	if idleTimeout <= 0 {
		return 0
	}
	interval := time.Minute
	if half := idleTimeout / 2; half > 0 && half < interval {
		interval = half
	}
	if interval < time.Second {
		return time.Second
	}
	return interval
}

func startTerminalReaper(ctx context.Context, svc terminalservice.Service, interval time.Duration) func() {
	if svc == nil || interval <= 0 {
		return func() {}
	}
	reaperCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-reaperCtx.Done():
				return
			case <-ticker.C:
				_ = svc.ReapIdle(reaperCtx)
			}
		}
	}()
	return cancel
}

// hitlApprovalSweepInterval is how often startHITLApprovalSweeper resolves
// pending approvals past their deadline. Fixed rather than configurable: a
// rule's own TimeoutS (seen in existing tests as low as a few seconds) can be
// far shorter than the HITL_APPROVAL_TIMEOUT ceiling, so this needs to stay
// short regardless of the ceiling's value.
const hitlApprovalSweepInterval = 30 * time.Second

// parseHITLApprovalCeiling parses HITL_APPROVAL_TIMEOUT (a Go duration
// string, e.g. "1h"). Empty keeps hitlservice's built-in
// DefaultApprovalCeiling; SetApprovalCeiling(svc, 0) is a no-op, so returning
// a zero duration for "unset" is safe. Mirrors terminalservice.ParseEnv's
// validation style for a serve config setting.
func parseHITLApprovalCeiling(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid HITL_APPROVAL_TIMEOUT %q: must be a positive Go duration (e.g. 1h)", raw)
	}
	return d, nil
}

// startHITLApprovalSweeper periodically resolves pending human-in-the-loop
// approvals whose deadline (a matched rule's own TimeoutS, or the serve-level
// ceiling when the rule set none) has passed, applying the stored OnTimeout.
// It is the durability backstop for a requester whose own bounded wait
// already returned without touching its row, and for one whose process
// restarted before it could — see hitlservice.Service.SweepExpired's doc.
// Mirrors startTerminalReaper's ticker/shutdown shape immediately above.
func startHITLApprovalSweeper(ctx context.Context, svc hitlservice.Service, interval time.Duration) func() {
	if svc == nil || interval <= 0 {
		return func() {}
	}
	sweepCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-sweepCtx.Done():
				return
			case <-ticker.C:
				_, _ = svc.SweepExpired(sweepCtx)
			}
		}
	}()
	return cancel
}

// discoverChainAgents runs one chain-agent discovery pass over the two
// directories every other contenox config file is resolved through — the
// workspace .contenox/ first, then ~/.contenox/ — so a chain named by the
// agent-* convention in either, and the agent-shaped chains `contenox init`
// ships, are declared as fleet-dispatchable agents without an operator
// registering anything by hand.
//
// It is BEST EFFORT by design. Discovery only seeds the registry; the registry
// is what the fleet actually resolves against, so a failed pass degrades to
// "the fleet has whatever was already declared" rather than to a serve that
// will not start. The outcome is logged either way, because an agent silently
// failing to appear is exactly the kind of half-built surface this must not
// manufacture.
func discoverChainAgents(ctx context.Context, agents agentregistryservice.Service, contenoxDir string) {
	roots := []string{contenoxDir}
	if homeDir, err := globalContenoxDir(); err == nil {
		roots = append(roots, homeDir)
	}
	res, err := chainagents.Discover(ctx, agents, roots...)
	if err != nil {
		slog.Warn("contenox serve: chain-agent discovery failed; the fleet keeps the agents already declared",
			"error", err, "roots", roots)
		return
	}
	if len(res.Created) > 0 || len(res.Updated) > 0 || len(res.Disabled) > 0 || len(res.Skipped) > 0 {
		slog.Info("contenox serve: chain agents discovered",
			"created", res.Created, "updated", res.Updated,
			"disabled", res.Disabled, "skipped_name_taken", res.Skipped,
			"unchanged", len(res.Unchanged))
	}
}
