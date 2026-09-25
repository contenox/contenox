package acpsvc

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/contenox/contenox/internal/kernel/reasoning"
	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/models/modelcapability"
	"github.com/contenox/contenox/internal/services/chatservice"
	"github.com/contenox/contenox/internal/services/missiontools"
	"github.com/contenox/contenox/internal/services/settings"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/version"
	libacp "github.com/contenox/contenox/libacp"
	"github.com/google/uuid"
)

// allACPCommands is the full, capability-unfiltered admin command set. Use
// (*Transport).acpCommands for anything reaching a client.
func allACPCommands() []libacp.AvailableCommand {
	return []libacp.AvailableCommand{
		{Name: "settings", Description: "Inspect session settings, their limits and where to save defaults.", Input: &libacp.AvailableCommandInput{Hint: "[setting [value|inherit]]"}},
		{Name: "context", Description: "Session context window in tokens; auto follows model capacity, inherit uses saved defaults.", Input: &libacp.AvailableCommandInput{Hint: "[tokens|auto|inherit]"}},
		{Name: "output", Description: "Maximum output tokens per call for this session; auto uses the backend default.", Input: &libacp.AvailableCommandInput{Hint: "[tokens|auto|inherit]"}},
		{Name: "reasoning", Description: "Reasoning effort for this session; auto uses the backend default.", Input: &libacp.AvailableCommandInput{Hint: "[auto|off|minimal|low|medium|high|xhigh]"}},
		{Name: "permissions", Description: "Tool permission policy for this session.", Input: &libacp.AvailableCommandInput{Hint: "[policy-name]"}},
		{Name: "help", Description: "List the available commands."},
		{Name: "doctor", Description: "Check provider/model/backend readiness, this build's provenance, and the tools this session holds (read-only — no test prompt is sent)."},
		{Name: "clear", Description: "Clear this session's conversation history."},
		{Name: "compact", Description: "Summarize older history into a single message to reclaim context.", Input: &libacp.AvailableCommandInput{Hint: "[keep]"}},
		{Name: "rename", Description: "Show or set this session's title: /rename <title> (- resets it).", Input: &libacp.AvailableCommandInput{Hint: "[title|-]"}},
		{Name: "model", Description: "Show or choose this session’s model: /model <name> or <provider/model>.", Input: &libacp.AvailableCommandInput{Hint: "[model-name]"}},
		{Name: "provider", Description: "Show or choose this session’s model provider.", Input: &libacp.AvailableCommandInput{Hint: "[provider-name]"}},
		{Name: "max-tokens", Description: "Alias of /output; changes this session only.", Input: &libacp.AvailableCommandInput{Hint: "[count]"}},
		{Name: "think", Description: "Show or set this session's reasoning level: /think <level|off|auto>.", Input: &libacp.AvailableCommandInput{Hint: "[level|off|auto]"}},
		{Name: "capability", Description: "Show or set persistent provider/model capability overrides.", Input: &libacp.AvailableCommandInput{Hint: "set|show|unset <provider> <model> [--think true|false]"}},
		{Name: "policy", Description: "Alias of /permissions; changes this session only.", Input: &libacp.AvailableCommandInput{Hint: "[policy-name]"}},
		{Name: "mission", Description: "Fire a mission from this session; alone, lists the envelopes it can run under.", Input: &libacp.AvailableCommandInput{Hint: "[--policy <envelope>] [agent-name] <intent>"}},
		{Name: planCommandName, Description: "Plan a piece of work, then run each step as a subagent.", Input: &libacp.AvailableCommandInput{Hint: "<what you want done>"}},
		{Name: "answer", Description: "Answer a question one of this session's subagents is waiting on; alone, lists them.", Input: &libacp.AvailableCommandInput{Hint: "[ask-id <answer>]"}},
		{Name: "new", Description: "Start a new session in this workspace and report its id."},
		{Name: "sessions", Description: "List the sessions in this workspace, newest first."},
	}
}

// acpCommands is the admin command set advertised to this transport's ACP
// clients: allACPCommands filtered by commandAvailable.
func (t *Transport) acpCommands() []libacp.AvailableCommand {
	all := allACPCommands()
	out := make([]libacp.AvailableCommand, 0, len(all))
	for _, c := range all {
		if !t.commandAvailable(c.Name) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// commandAvailable reports whether a command can actually run on this transport.
// parseCommand still recognizes a filtered command, so typing it gets the
// handler's teaching error rather than "unknown command".
func (t *Transport) commandAvailable(name string) bool {
	switch name {
	case "mission", planCommandName:
		return t.hasMissionCapability()
	case "answer":
		return t.hasAnswerCapability()
	default:
		return true
	}
}

// acpCommandNames is built from allACPCommands, not the per-transport
// advertised list.
var acpCommandNames = func() map[string]struct{} {
	m := make(map[string]struct{}, len(allACPCommands()))
	for _, c := range allACPCommands() {
		m[c.Name] = struct{}{}
	}
	return m
}()

// parseCommand recognizes a leading slash command whose first token is one of
// the admin commands; a pasted path or prose mentioning one is left alone.
func parseCommand(input string) (name, args string, ok bool) {
	s := strings.TrimSpace(input)
	if !strings.HasPrefix(s, "/") {
		return "", "", false
	}
	rest := s[1:]
	first := rest
	if i := strings.IndexFunc(rest, unicode.IsSpace); i >= 0 {
		first = rest[:i]
		args = strings.TrimSpace(rest[i+1:])
	}
	if _, known := acpCommandNames[first]; !known {
		return "", "", false
	}
	return first, args, true
}

// commandShapeRE is narrower than parseCommand's "up to the first space" so
// that paths and prose mentioning one reach the model untouched.
var commandShapeRE = regexp.MustCompile(`^[a-z0-9-]+$`)

func unknownCommandName(input string) (string, bool) {
	s := strings.TrimSpace(input)
	if !strings.HasPrefix(s, "/") {
		return "", false
	}
	first := s[1:]
	if i := strings.IndexFunc(first, unicode.IsSpace); i >= 0 {
		first = first[:i]
	}
	if !commandShapeRE.MatchString(first) {
		return "", false
	}
	if _, known := acpCommandNames[first]; known {
		return "", false
	}
	return first, true
}

func unknownCommandMessage(name string) string {
	return fmt.Sprintf("⚠️  unknown command: /%s — /help lists commands", name)
}

// answerUnknownCommand ends the turn locally with the teaching line and no model
// call. It skips persistCommandTurn: nothing happened, so a typo must not reach
// the durable transcript.
func (t *Transport) answerUnknownCommand(ctx context.Context, sid libacp.SessionID, name string) libacp.PromptResponse {
	t.sendUpdate(ctx, libacp.SessionNotification{
		SessionID: sid,
		Update:    libacp.NewAgentMessageChunk(unknownCommandMessage(name)),
	})
	return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}
}

// dispatchCommand runs an admin command and reports the outcome to the client as
// an agent message. Failures are surfaced inline rather than as protocol errors.
func (t *Transport) dispatchCommand(ctx context.Context, sid libacp.SessionID, sess *sessionEntry, name, args string) (libacp.PromptResponse, error) {
	reportErr, _, end := t.tracker().Start(ctx, "command", "acp_session", "session_id", string(sid), "command", name)
	defer end()

	var (
		out string
		err error
	)
	switch name {
	case "help":
		out = t.handleHelp()
	case "doctor":
		out, err = t.handleDoctor(ctx, sess)
	case "model":
		out, err = t.handleSessionSetting(ctx, sess, settings.Model, args)
	case "provider":
		out, err = t.handleSessionSetting(ctx, sess, "inference.provider", args)
	case "max-tokens", "output":
		out, err = t.handleSessionSetting(ctx, sess, settings.MaxOutputTokens, args)
	case "think", "reasoning":
		out, err = t.handleThink(sess, args)
	case "capability":
		out, err = t.handleCapability(ctx, args)
	case "policy", "permissions":
		out, err = t.handleSessionSetting(ctx, sess, settings.PermissionPolicy, args)
	case "context":
		out, err = t.handleSessionSetting(ctx, sess, settings.ContextWindowTokens, args)
	case "settings":
		out, err = t.handleSettings(ctx, sess, args)
	case "mission":
		out, err = t.handleMission(ctx, sess, args)
	case "answer":
		out, err = t.handleAnswer(ctx, sess, args)
	case "new":
		out, err = t.handleNewSessionCommand(ctx, sess)
	case "sessions":
		out, err = t.handleSessions(ctx, sess)
	case "clear":
		out, err = t.handleClear(ctx, sid, sess)
	case "compact":
		out, err = t.handleCompact(ctx, sid, sess, args)
	case "rename":
		out, err = t.handleRename(ctx, sess, args)
	default:
		err = libacp.NewErrorf(libacp.ErrInvalidParams, "unknown command %q", name)
	}

	if err != nil {
		reportErr(err)
		t.sendUpdate(ctx, libacp.SessionNotification{
			SessionID: sid,
			Update:    libacp.NewAgentMessageChunk("⚠️  " + err.Error()),
		})
		t.persistCommandTurn(ctx, sess, name, args, "⚠️  "+err.Error())
		return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}, nil
	}
	if out != "" {
		t.sendUpdate(ctx, libacp.SessionNotification{
			SessionID: sid,
			Update:    libacp.NewAgentMessageChunk(out),
		})
	}
	t.persistCommandTurn(ctx, sess, name, args, out)
	if commandUpdatesSessionInfo(name) {
		// A command returns before Prompt's own AfterResponse push.
		libacp.AfterResponse(ctx, func() {
			update := libacp.SessionUpdate{
				SessionUpdate: libacp.SessionUpdateSessionInfo,
				UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
			}
			if title := t.sessionInfoTitle(ctx, sess.InternalSessionID); title != "" {
				update.Title = title
			}
			t.sendUpdate(ctx, libacp.SessionNotification{SessionID: sid, Update: update})
		})
	}
	if commandUpdatesConfigOptions(name) {
		t.sendConfigOptionUpdate(ctx, sid, sess)
		if t.conn != nil {
			libacp.AfterResponse(ctx, func() { t.sendResumedUsageUpdate(ctx, sid, sess) })
		}
	}
	return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}, nil
}

// persistCommandTurn records a slash-command exchange in the session's durable
// transcript, skipping commandRewritesHistory commands.
func (t *Transport) persistCommandTurn(ctx context.Context, sess *sessionEntry, name, args, out string) {
	if t.deps.DB == nil || sess == nil || commandRewritesHistory(name) {
		return
	}
	internalID := sess.InternalSessionID
	if internalID == "" || strings.TrimSpace(out) == "" {
		return
	}
	typed := "/" + name
	if args = strings.TrimSpace(args); args != "" {
		typed += " " + args
	}
	now := time.Now().UTC()
	msgs := []taskengine.Message{
		{ID: uuid.NewString(), Role: "user", Content: typed, Timestamp: now},
		{ID: uuid.NewString(), Role: "assistant", Content: out, Timestamp: now.Add(time.Millisecond)},
	}
	cleanCtx := context.WithoutCancel(ctx)
	mgr := chatservice.NewManager(sess.WorkspaceID)
	if err := mgr.PersistDiff(cleanCtx, t.deps.DB.WithoutTransaction(), internalID, msgs); err != nil {
		reportErr, _, end := t.tracker().Start(cleanCtx, "persist", "acp_command_turn", "session_id", internalID, "command", name)
		reportErr(err)
		end()
	}
}

// commandRewritesHistory reports whether a command owns the transcript itself.
func commandRewritesHistory(name string) bool {
	switch name {
	case "clear", "compact":
		return true
	default:
		return false
	}
}

func commandUpdatesSessionInfo(name string) bool {
	return name == "rename"
}

func commandUpdatesConfigOptions(name string) bool {
	switch name {
	case "model", "provider", "policy", "think", "reasoning", "context", "output", "max-tokens", "permissions", "settings":
		return true
	default:
		return false
	}
}

// sendAvailableCommands advertises the admin command set for a session. Callers
// must schedule it via libacp.AfterResponse: a client drops an unmapped session
// id, silently disabling the menu.
func (t *Transport) sendAvailableCommands(ctx context.Context, sid libacp.SessionID) {
	t.sendUpdate(ctx, libacp.SessionNotification{
		SessionID: sid,
		Update: libacp.SessionUpdate{
			SessionUpdate:     libacp.SessionUpdateAvailableCommands,
			AvailableCommands: t.acpCommands(),
		},
	})
}

func (t *Transport) handleHelp() string {
	cmds := t.acpCommands()
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, c := range cmds {
		fmt.Fprintf(&b, "  /%-9s %s\n", c.Name, c.Description)
	}
	return strings.TrimRight(b.String(), "\n")
}

// handleDoctor reports current provider/model/backend readiness from live
// runtime state, this build's provenance, and the tools this session's turns
// advertise; read-only, never a model completion.
func (t *Transport) handleDoctor(ctx context.Context, sess *sessionEntry) (string, error) {
	if t.deps.Engine == nil || t.deps.Engine.SetupStatus == nil {
		return "", fmt.Errorf("readiness check unavailable")
	}
	res, err := t.deps.Engine.SetupStatus(ctx)
	if err != nil {
		return "", fmt.Errorf("readiness check failed: %w", err)
	}
	summary := res.Summary()

	ceiling := res.DefaultMaxOutputTokens
	if ceiling > 0 {
		maxTok := t.maxTokens()
		if maxTok != "" {
			if n, convErr := strconv.Atoi(maxTok); convErr == nil && n > ceiling {
				summary += fmt.Sprintf(
					"\n⚠️  Advisory: inference.generation.max_output_tokens=%d exceeds %s provider ceiling (%d). Requests will be clamped automatically.",
					n, t.provider(), ceiling)
			}
		}
	}
	summary += "\n\n" + buildProvenanceLine()
	// Partial answers over none: the readiness half stays useful when the
	// roster cannot be enumerated.
	if roster, rosterErr := t.sessionToolRoster(ctx, sess); rosterErr != nil {
		summary += "\nTools: unavailable (" + rosterErr.Error() + ")"
	} else {
		summary += "\n" + roster
	}
	return summary, nil
}

// buildProvenanceLine names the running build: version plus the VCS state that
// separates a dirty working-tree build from the release version.txt claims.
func buildProvenanceLine() string {
	if p := version.GetProvenance().String(); p != "" {
		return fmt.Sprintf("Build: %s (%s)", version.Get(), p)
	}
	return fmt.Sprintf("Build: %s", version.Get())
}

// sessionToolRoster renders the tools a turn in sess would advertise right
// now: the engine's aggregate repo enumerated under the session's runtime
// allowlist, mission role, and the attached client's capabilities — the same
// resolution a prompt runs through, so the report cannot drift from it.
func (t *Transport) sessionToolRoster(ctx context.Context, sess *sessionEntry) (string, error) {
	if t.deps.Engine == nil || t.deps.Engine.Tools == nil {
		return "", fmt.Errorf("tool roster unavailable")
	}
	rosterCtx := ctx
	var store runtimetypes.Store
	if t.deps.DB != nil {
		store = runtimetypes.New(t.deps.DB.WithoutTransaction())
		var names []string
		if sess != nil {
			names = sess.McpServerNames
		}
		allowlist, err := t.runtimeToolsAllowlist(ctx, store, names)
		if err != nil {
			return "", err
		}
		rosterCtx = taskengine.WithRuntimeToolsAllowlist(rosterCtx, allowlist)
	}
	if sess != nil && sess.InternalSessionID != "" {
		rosterCtx = context.WithValue(rosterCtx, runtimetypes.SessionIDContextKey, sess.InternalSessionID)
		// Mirrors the prompt path's mission decoration: the mission toolset
		// advertises by session role.
		if sess.MissionID != "" {
			rosterCtx = missiontools.WithMissionID(rosterCtx, sess.MissionID)
		} else if t.hasMissionCapability() {
			rosterCtx = missiontools.WithParentSessionID(rosterCtx, sess.InternalSessionID)
		}
	}

	names, err := t.deps.Engine.Tools.Supports(rosterCtx)
	if err != nil {
		return "", err
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("Tools this session holds (tool — toolset — origin):\n")
	if len(names) == 0 {
		b.WriteString("  (none)")
		return b.String(), nil
	}
	for _, name := range names {
		kind, label := t.toolsetOrigin(rosterCtx, store, name)
		if kind == originRemoteProvider {
			// Enumerating a remote provider fetches its spec over HTTP; /doctor
			// stays local and names the provider instead.
			fmt.Fprintf(&b, "  %s — %s; tools listed by `contenox tools show %s`\n", name, label, name)
			continue
		}
		tools, err := t.deps.Engine.Tools.GetToolsForToolsByName(rosterCtx, name)
		if err != nil {
			fmt.Fprintf(&b, "  %s — %s — unavailable: %v\n", name, label, err)
			continue
		}
		if len(tools) == 0 {
			if capName := clientProxiedCapability(name); kind == originClientCapability && capName != "" {
				fmt.Fprintf(&b, "  %s — nothing advertised (the attached client grants no %s)\n", name, capName)
			} else {
				fmt.Fprintf(&b, "  %s — nothing advertised in this session\n", name)
			}
			continue
		}
		for _, tool := range tools {
			origin := label
			if kind == originClientCapability {
				// A toolset can be client-backed and still hold in-process tools
				// (local_fs browses here and reads and writes through the client),
				// so each tool states its own backing.
				if cap := RequiredClientCapability(name, tool.Function.Name); cap != "" {
					origin = "client capability " + cap
				} else {
					origin = "local (in-process)"
				}
			}
			fmt.Fprintf(&b, "  %s — %s — %s\n", tool.Function.Name, name, origin)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

type toolsetOriginKind int

const (
	originLocal toolsetOriginKind = iota
	originClientCapability
	originMCPServer
	originRemoteProvider
)

// toolsetOrigin classifies where name's tools run: in-process, proxied to the
// ACP client, an MCP server, or a store-registered remote OpenAPI provider.
func (t *Transport) toolsetOrigin(ctx context.Context, store runtimetypes.Store, name string) (toolsetOriginKind, string) {
	if slices.Contains(t.deps.Engine.LocalTools, name) {
		if capName := clientProxiedCapability(name); capName != "" {
			return originClientCapability, "client capability " + capName
		}
		return originLocal, "local (in-process)"
	}
	if store != nil {
		if srv, err := store.GetMCPServerByName(ctx, name); err == nil {
			if runtimetypes.IsACPManagedMCPServerName(srv.Name) {
				return originMCPServer, "MCP server " + srv.Name + " (session-scoped, supplied by the client)"
			}
			return originMCPServer, "MCP server " + srv.Name
		}
	}
	return originRemoteProvider, "remote tool provider (OpenAPI)"
}

func ceilingLabel(ceiling int) string {
	if ceiling > 0 {
		return strconv.Itoa(ceiling)
	}
	return "unknown"
}

func normalizeMaxTokensValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return "", fmt.Errorf("max-tokens must be a non-negative integer, got %q", value)
	}
	if n < 0 {
		return "", fmt.Errorf("max-tokens must be non-negative, got %d", n)
	}
	return strconv.Itoa(n), nil
}

func (t *Transport) handleThink(sess *sessionEntry, args string) (string, error) {
	value := strings.TrimSpace(args)
	if value == "" {
		return fmt.Sprintf("Think: %s", sess.think()), nil
	}
	level, err := reasoning.Normalize(value)
	if err != nil {
		return "", err
	}
	sess.setThink(level)
	return fmt.Sprintf("Think set to %s for this session.", level), nil
}

func (t *Transport) handleCapability(ctx context.Context, args string) (string, error) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return t.capabilityUsage(ctx), nil
	}
	switch fields[0] {
	case "show":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /capability show <provider> <model>")
		}
		return t.capabilityShow(ctx, fields[1], fields[2])
	case "set":
		provider, model, canThink, err := parseCapabilitySetArgs(fields)
		if err != nil {
			return "", err
		}
		store := runtimetypes.New(t.deps.DB.WithoutTransaction())
		override, err := modelcapability.New(store).SetThink(ctx, provider, model, canThink)
		if err != nil {
			return "", fmt.Errorf("set capability override: %w", err)
		}
		return fmt.Sprintf("Capability override set for %s/%s: think=%t.", override.Provider, override.Model, canThink), nil
	case "unset":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /capability unset <provider> <model>")
		}
		store := runtimetypes.New(t.deps.DB.WithoutTransaction())
		removed, err := modelcapability.New(store).Unset(ctx, fields[1], fields[2])
		if err != nil {
			return "", fmt.Errorf("unset capability override: %w", err)
		}
		_, provider, model, keyErr := modelcapability.Key(fields[1], fields[2])
		if keyErr != nil {
			return "", keyErr
		}
		if !removed {
			return fmt.Sprintf("No capability override for %s/%s.", provider, model), nil
		}
		return fmt.Sprintf("Capability override removed for %s/%s.", provider, model), nil
	default:
		return "", fmt.Errorf("usage: /capability set|show|unset <provider> <model> [--think true|false]")
	}
}

func (t *Transport) capabilityUsage(ctx context.Context) string {
	usage := "Usage:\n  /capability show <provider> <model>\n  /capability set <provider> <model> --think true|false\n  /capability unset <provider> <model>\n\nThis persists a provider/model capability override. It is separate from /think, which only changes this session's reasoning level."
	provider := strings.TrimSpace(t.provider())
	model := strings.TrimSpace(t.model())
	if provider == "" || model == "" {
		return usage
	}
	status, err := t.capabilityShow(ctx, provider, model)
	if err != nil {
		return usage
	}
	return usage + "\n\nCurrent default:\n" + status
}

func (t *Transport) capabilityShow(ctx context.Context, provider, model string) (string, error) {
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	override, ok, err := modelcapability.New(store).Get(ctx, provider, model)
	if err != nil {
		return "", fmt.Errorf("show capability override: %w", err)
	}
	if !ok || override.CanThink == nil {
		_, p, m, keyErr := modelcapability.Key(provider, model)
		if keyErr != nil {
			return "", keyErr
		}
		return fmt.Sprintf("No capability override for %s/%s.", p, m), nil
	}
	return fmt.Sprintf("Capability override for %s/%s: think=%t.", override.Provider, override.Model, *override.CanThink), nil
}

func parseCapabilitySetArgs(fields []string) (string, string, bool, error) {
	if len(fields) < 4 {
		return "", "", false, fmt.Errorf("usage: /capability set <provider> <model> --think true|false")
	}
	provider, model := fields[1], fields[2]
	var canThink bool
	seenThink := false
	for i := 3; i < len(fields); i++ {
		arg := fields[i]
		value := ""
		if strings.HasPrefix(arg, "--think=") {
			value = strings.TrimPrefix(arg, "--think=")
		} else if arg == "--think" {
			if i+1 >= len(fields) {
				return "", "", false, fmt.Errorf("--think requires true or false")
			}
			i++
			value = fields[i]
		} else {
			return "", "", false, fmt.Errorf("unknown capability flag %q", arg)
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true":
			canThink = true
		case "false":
			canThink = false
		default:
			return "", "", false, fmt.Errorf("--think must be true or false")
		}
		seenThink = true
	}
	if !seenThink {
		return "", "", false, fmt.Errorf("--think is required")
	}
	return provider, model, canThink, nil
}

func (t *Transport) handleSettings(ctx context.Context, sess *sessionEntry, args string) (string, error) {
	parts := strings.Fields(args)
	if len(parts) > 0 {
		value := ""
		if len(parts) > 1 {
			value = strings.Join(parts[1:], " ")
		}
		return t.handleSessionSetting(ctx, sess, settings.Canonical(parts[0]), value)
	}
	var out strings.Builder
	out.WriteString("Session settings — changes apply to the next turn or tool call, without saving defaults.\n\n")
	for _, option := range t.sessionConfigOptions(ctx, sess) {
		fmt.Fprintf(&out, "%s = %s\n  %s\n", option.ID, option.CurrentValue, option.Description)
	}
	if t.deps.ChainRegistry != nil {
		if chain := t.deps.ChainRegistry.Default(); chain != nil {
			for _, task := range chain.Tasks {
				if task.ExecuteConfig != nil && task.ExecuteConfig.MaxTokens != nil {
					fmt.Fprintf(&out, "  Chain task %s has an explicit output cap: %d tokens.\n", task.ID, *task.ExecuteConfig.MaxTokens)
				}
			}
		}
	}
	out.WriteString("\nChange here: /settings <setting> <value>; /context, /output, /model, /reasoning, /permissions are shortcuts.\nSave for future launches in your terminal: contenox config set <setting> <value>\nInspect saved inheritance: contenox config get <setting> --explain\nAgent limits: ~/.contenox/agents.toml, then .contenox/agents.toml, then [agents.<name>]; explicit custom chains retain their own limits.")
	return out.String(), nil
}

func (t *Transport) handleSessionSetting(ctx context.Context, sess *sessionEntry, key, args string) (string, error) {
	key = settings.Canonical(key)
	value := strings.TrimSpace(args)
	if key == "inference.provider" {
		if value != "" {
			sess.setModelSelection(value, sess.modelOrDefault(t.model()))
		}
		return fmt.Sprintf("Session inference.provider = %s. Choose a provider/model pair with /model. Save defaults with contenox config set inference.provider <provider>.", sess.providerOrDefault(t.provider())), nil
	}
	if key == settings.Model && value != "" {
		option := t.modelConfigOption(ctx, sess)
		if !configOptionHasValue(option, value) {
			qualified := modelConfigValue(sess.providerOrDefault(t.provider()), value)
			if configOptionHasValue(option, qualified) {
				value = qualified
			} else {
				candidates := []string{}
				for _, candidate := range option.Options.AllValues() {
					_, model := splitModelConfigValue(candidate.Value)
					if model == value {
						candidates = append(candidates, candidate.Value)
					}
				}
				if len(candidates) == 1 {
					value = candidates[0]
				} else if len(candidates) > 1 {
					return "", fmt.Errorf("model %q is ambiguous; choose %s", value, strings.Join(candidates, " or "))
				}
			}
		}
	}
	if key == settings.PermissionPolicy && value != "" {
		if value == "inherit" {
			value = hitlPolicyDefaultValue
		}
		option := t.hitlPolicyConfigOption(sess)
		if !configOptionHasValue(option, value) {
			for _, candidate := range option.Options.AllValues() {
				if candidate.Name == value {
					value = candidate.Value
					break
				}
			}
		}
	}
	if value != "" {
		if err := t.setSessionConfigOption(ctx, sess, key, value); err != nil {
			return "", err
		}
	}
	for _, option := range t.sessionConfigOptions(ctx, sess) {
		if option.ID == key {
			save := fmt.Sprintf("Save a default for future launches: contenox config set %s <value>", key)
			if key == settings.Model {
				save = fmt.Sprintf("Save both defaults: contenox config set inference.provider %s; contenox config set inference.model %s", sess.providerOrDefault(t.provider()), sess.modelOrDefault(t.model()))
			}
			return fmt.Sprintf("Session %s = %s\n%s\n%s", key, option.CurrentValue, option.Description, save), nil
		}
	}
	return "", fmt.Errorf("%q is not a session setting; use /settings to list the available settings", key)
}
