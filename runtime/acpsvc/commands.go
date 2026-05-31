package acpsvc

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	libacp "github.com/contenox/agent/libacp"
	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/internal/clikv"
	"github.com/contenox/agent/runtime/runtimetypes"
)

// acpCommands is the admin command set advertised to ACP clients. The protocol
// only uses this list as a client-side autocomplete/menu hint — an invoked
// command arrives back as ordinary prompt text, which Prompt intercepts.
func acpCommands() []libacp.AvailableCommand {
	return []libacp.AvailableCommand{
		{Name: "help", Description: "List the available commands."},
		{Name: "doctor", Description: "Check provider/model/backend readiness (read-only — no test prompt is sent)."},
		{Name: "clear", Description: "Clear this session's conversation history."},
		{Name: "compact", Description: "Summarize older history into a single message to reclaim context.", Input: &libacp.AvailableCommandInput{Hint: "[keep]"}},
		{Name: "model", Description: "Show the current model, or set it: /model <name>.", Input: &libacp.AvailableCommandInput{Hint: "[model-name]"}},
		{Name: "provider", Description: "Show the current provider, or set it: /provider <name>.", Input: &libacp.AvailableCommandInput{Hint: "[provider-name]"}},
		{Name: "policy", Description: "Show the active HITL policy, or switch it: /policy <name>.", Input: &libacp.AvailableCommandInput{Hint: "[policy-name]"}},
	}
}

// acpCommandNames is the set of recognized command names, used by parseCommand.
var acpCommandNames = func() map[string]struct{} {
	m := make(map[string]struct{}, len(acpCommands()))
	for _, c := range acpCommands() {
		m[c.Name] = struct{}{}
	}
	return m
}()

// parseCommand recognizes a leading slash command whose first token is one of
// the advertised admin commands. It matches the first token ONLY when it leads
// the input, so a pasted path ("/home/user/x") or text that merely mentions a
// slash ("what does /etc/passwd do") is left as a normal prompt.
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

// dispatchCommand runs an admin command and reports the outcome to the client
// as an agent message. Command failures are surfaced inline (not as a protocol
// error) and still end the turn, so the editor shows them in the conversation.
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
		out, err = t.handleDoctor(ctx)
	case "model":
		out, err = t.handleModel(ctx, args)
	case "provider":
		out, err = t.handleProvider(ctx, args)
	case "policy":
		out, err = t.handlePolicy(ctx, args)
	case "clear":
		out, err = t.handleClear(ctx, sid, sess)
	case "compact":
		out, err = t.handleCompact(ctx, sid, sess, args)
	default:
		err = libacp.NewErrorf(libacp.ErrInvalidParams, "unknown command %q", name)
	}

	if err != nil {
		reportErr(err)
		t.sendUpdate(ctx, libacp.SessionNotification{
			SessionID: sid,
			Update:    libacp.NewAgentMessageChunk("⚠️  " + err.Error()),
		})
		return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}, nil
	}
	if out != "" {
		t.sendUpdate(ctx, libacp.SessionNotification{
			SessionID: sid,
			Update:    libacp.NewAgentMessageChunk(out),
		})
	}
	return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}, nil
}

// sendAvailableCommands advertises the admin command set for a session. The
// client uses it to populate its slash-command menu.
func (t *Transport) sendAvailableCommands(ctx context.Context, sid libacp.SessionID) {
	t.sendUpdate(ctx, libacp.SessionNotification{
		SessionID: sid,
		Update: libacp.SessionUpdate{
			SessionUpdate:     libacp.SessionUpdateAvailableCommands,
			AvailableCommands: acpCommands(),
		},
	})
}

func (t *Transport) handleHelp() string {
	cmds := acpCommands()
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, c := range cmds {
		fmt.Fprintf(&b, "  /%-9s %s\n", c.Name, c.Description)
	}
	return strings.TrimRight(b.String(), "\n")
}

// handleDoctor reports current provider/model/backend readiness. It recomputes
// from live runtime state via the engine — read-only, never a model completion.
func (t *Transport) handleDoctor(ctx context.Context) (string, error) {
	if t.deps.Engine == nil || t.deps.Engine.SetupStatus == nil {
		return "", fmt.Errorf("readiness check unavailable")
	}
	res, err := t.deps.Engine.SetupStatus(ctx)
	if err != nil {
		return "", fmt.Errorf("readiness check failed: %w", err)
	}
	return res.Summary(), nil
}

func (t *Transport) handleModel(ctx context.Context, args string) (string, error) {
	value := strings.TrimSpace(args)
	if value == "" {
		return fmt.Sprintf("Model: %s", t.model()), nil
	}
	if err := t.persistConfig(ctx, "default-model", value); err != nil {
		return "", err
	}
	t.setModel(value)
	return fmt.Sprintf("Model set to %s.", value), nil
}

func (t *Transport) handleProvider(ctx context.Context, args string) (string, error) {
	value := strings.TrimSpace(args)
	if value == "" {
		current := t.provider()
		if current == "" {
			return "Provider: (default)", nil
		}
		return fmt.Sprintf("Provider: %s", current), nil
	}
	if err := t.persistConfig(ctx, "default-provider", value); err != nil {
		return "", err
	}
	t.setProvider(value)
	return fmt.Sprintf("Provider set to %s.", value), nil
}

// handlePolicy shows or switches the active HITL approval policy. Switching
// writes the global cli.hitl-policy-name key — the key the engine reads live on
// every gated tool call — so the change takes effect on the next gated call. It
// just does what it's told: the operator owns the policy.
func (t *Transport) handlePolicy(ctx context.Context, args string) (string, error) {
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	value := strings.TrimSpace(args)
	if value == "" {
		return t.policyStatus(clikv.ReadHITLPolicy(ctx, store)), nil
	}
	cfgCtx := libtracker.WithNewRequestID(ctx)
	if err := clikv.SetHITLPolicy(cfgCtx, store, value); err != nil {
		return "", fmt.Errorf("set hitl policy: %w", err)
	}
	return fmt.Sprintf("HITL policy set to %s. Applies to the next gated tool call.", value), nil
}

// policyStatus renders the effective policy and the selectable presets. With no
// override set, the effective policy is the engine's fallback default.
func (t *Transport) policyStatus(active string) string {
	effective := active
	if effective == "" {
		effective = t.deps.HITLDefaultPolicyName
	}
	var b strings.Builder
	if active == "" {
		fmt.Fprintf(&b, "Active HITL policy: %s (default)\n", effective)
	} else {
		fmt.Fprintf(&b, "Active HITL policy: %s\n", effective)
	}
	if len(t.deps.KnownPolicies) > 0 {
		b.WriteString("Presets:\n")
		for _, name := range t.deps.KnownPolicies {
			marker := "  "
			if name == effective {
				marker = "* "
			}
			fmt.Fprintf(&b, "%s%s\n", marker, name)
		}
		b.WriteString("Switch with: /policy <name>")
	}
	return strings.TrimRight(b.String(), "\n")
}

// persistConfig writes a global CLI config value, mirroring `contenox config
// set` so the change also applies to future sessions and CLI invocations.
func (t *Transport) persistConfig(ctx context.Context, key, value string) error {
	store := runtimetypes.New(t.deps.DB.WithoutTransaction())
	cfgCtx := libtracker.WithNewRequestID(ctx)
	if err := clikv.WriteConfig(cfgCtx, store, t.workspaceID(), key, value); err != nil {
		return fmt.Errorf("persist %s: %w", key, err)
	}
	return nil
}
