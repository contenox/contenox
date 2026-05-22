package hitlservice

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/contenox/agent/runtime/vfsservice"
)

// Action is the outcome of policy evaluation for a tool call.
type Action string

const (
	// ActionAllow passes the tool call through without any approval step.
	ActionAllow Action = "allow"
	// ActionApprove blocks execution and requests human approval before proceeding.
	ActionApprove Action = "approve"
	// ActionDeny rejects the tool call immediately with a soft message to the LLM.
	ActionDeny Action = "deny"
)

// ApprovalRequest describes a tool invocation that requires human review.
// Diff is populated for file-mutation tools (write_file, sed) to show the
// unified diff of what would change.
type ApprovalRequest struct {
	ToolCallID string
	ToolsName  string
	ToolName   string
	Args       map[string]any
	Diff       string
	DiffOld    string
	DiffNew    string
}

// ConditionOp is the comparison operator for a rule condition.
type ConditionOp string

const (
	// OpEq requires the argument value to equal the condition value exactly.
	OpEq ConditionOp = "eq"
	// OpGlob matches the argument value against a glob pattern.
	// Both value and pattern are normalized with path.Clean before matching,
	// preventing path-traversal bypass (e.g. ./src/../etc/passwd → etc/passwd).
	// Supports * (within a path component), ? (single char), and ** (across separators).
	OpGlob ConditionOp = "glob"
)

// Condition is a single key/op/value predicate applied to the args of a tool call.
type Condition struct {
	Key   string      `json:"key"`
	Op    ConditionOp `json:"op"`
	Value string      `json:"value"`
}

// Rule matches a tools+tool pair (with optional conditions) and assigns an action.
// When contains zero conditions the name match alone is sufficient.
// All conditions in When must hold for the rule to match (AND semantics).
type Rule struct {
	Tools  string      `json:"tools"`
	Tool   string      `json:"tool"`
	When   []Condition `json:"when,omitempty"`
	Action Action      `json:"action"`
	// TimeoutS is the number of seconds to wait for a human response when Action is
	// ActionApprove. Zero means no timeout (block indefinitely until ctx is cancelled).
	TimeoutS int `json:"timeout_s,omitempty"`
	// OnTimeout is the fallback action when the approval window expires.
	// Only "deny" and "approve" are valid (allow would silently bypass approval).
	OnTimeout Action `json:"on_timeout,omitempty"`
}

// Policy is the top-level document stored as hitl-policy.json in the VFS.
// Rules are evaluated in order; the first matching rule wins.
// DefaultAction is applied when no rule matches; it is fail-closed to "approve"
// when absent so an unaccounted-for tool pauses for a human.
type Policy struct {
	DefaultAction Action `json:"default_action,omitempty"`
	Rules         []Rule `json:"rules"`
}

// Reason constants used in EvaluationResult.Reason.
const (
	ReasonMatchedRule   = "matched_rule"
	ReasonDefaultAction = "default_action"
)

// EvaluationResult carries the policy decision plus introspection data.
type EvaluationResult struct {
	Action      Action
	MatchedRule *int   // nil when DefaultAction was applied (no rule matched)
	Reason      string // ReasonMatchedRule or ReasonDefaultAction
	TimeoutS    int
	OnTimeout   Action
}

// evaluate returns the EvaluationResult for the given tools, tool name, and call args.
func evaluate(p *Policy, toolsName, toolName string, args map[string]any) EvaluationResult {
	for i, r := range p.Rules {
		if ruleMatches(r, toolsName, toolName, args) {
			idx := i
			return EvaluationResult{
				Action:      r.Action,
				MatchedRule: &idx,
				Reason:      ReasonMatchedRule,
				TimeoutS:    r.TimeoutS,
				OnTimeout:   r.OnTimeout,
			}
		}
	}
	defaultAction := p.DefaultAction
	if defaultAction == "" {
		defaultAction = ActionApprove
	}
	return EvaluationResult{
		Action: defaultAction,
		Reason: ReasonDefaultAction,
	}
}

func ruleMatches(r Rule, toolsName, toolName string, args map[string]any) bool {
	toolsOK := r.Tools == "" || r.Tools == "*" || r.Tools == toolsName
	toolOK := r.Tool == "" || r.Tool == "*" || r.Tool == toolName
	if !toolsOK || !toolOK {
		return false
	}
	for _, c := range r.When {
		if !conditionMatches(c, args) {
			return false
		}
	}
	return true
}

func conditionMatches(c Condition, args map[string]any) bool {
	val, ok := args[c.Key]
	if !ok {
		return false
	}
	for _, s := range conditionValues(val) {
		switch c.Op {
		case OpEq:
			if s == c.Value {
				return true
			}
		case OpGlob:
			if globMatch(c.Value, s) {
				return true
			}
		}
	}
	return false
}

// conditionValues flattens an argument value into the strings a condition is
// tested against. A slice argument (e.g. {"path": ["a","/etc/passwd"]}) is
// tested element-wise so a single stringified "[a /etc/passwd]" cannot slip a
// restricted entry past a glob.
func conditionValues(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			out = append(out, fmt.Sprintf("%v", e))
		}
		return out
	default:
		return []string{fmt.Sprintf("%v", v)}
	}
}

// globMatch reports whether s matches the glob pattern.
// Both pattern and s are normalized with path.Clean before comparison to prevent
// path-traversal bypasses. Supports *, ?, and ** (which matches across path separators).
func globMatch(pattern, s string) bool {
	s = path.Clean(s)
	for _, expanded := range expandGlobBraces(pattern) {
		if globMatchOne(expanded, s) {
			return true
		}
	}
	return false
}

func globMatchOne(pattern, s string) bool {
	pattern = path.Clean(pattern)
	if !strings.ContainsAny(pattern, "*?[") {
		return pattern == s
	}
	if !strings.Contains(pattern, "**") {
		matched, err := path.Match(pattern, s)
		return err == nil && matched
	}
	return matchDoubleGlob(pattern, s)
}

// expandGlobBraces expands {a,b,c} alternations into the cross product of
// concrete patterns, supporting nesting and slashes inside an alternative.
// Go's path.Match has no brace support, so "**/{.ssh,.gnupg}/**" would
// otherwise match a literal "{" and silently never fire. Unbalanced braces
// are treated literally. Expansion is bounded; past the cap the original
// pattern is returned unexpanded.
const maxGlobExpansions = 4096

func expandGlobBraces(pattern string) []string {
	const maxExpansions = maxGlobExpansions
	out := []string{pattern}
	for {
		next := make([]string, 0, len(out))
		changed := false
		for _, p := range out {
			open, closeIdx, ok := firstBracePair(p)
			if !ok {
				next = append(next, p)
				continue
			}
			changed = true
			prefix, suffix := p[:open], p[closeIdx+1:]
			for _, alt := range splitTopLevelCommas(p[open+1 : closeIdx]) {
				next = append(next, prefix+alt+suffix)
			}
		}
		if !changed {
			return next
		}
		if len(next) > maxExpansions {
			return next[:maxExpansions]
		}
		out = next
	}
}

func firstBracePair(p string) (open, closeIdx int, ok bool) {
	for i := 0; i < len(p); i++ {
		if p[i] != '{' {
			continue
		}
		if c, found := matchingBrace(p, i); found {
			return i, c, true
		}
	}
	return 0, 0, false
}

func matchingBrace(s string, open int) (int, bool) {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return -1, false
}

func splitTopLevelCommas(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// matchDoubleGlob handles patterns that contain **.
// ** matches zero or more path components.
func matchDoubleGlob(pattern, s string) bool {
	idx := strings.Index(pattern, "**")
	prefix := strings.TrimSuffix(pattern[:idx], "/")
	after := strings.TrimPrefix(pattern[idx+2:], "/")

	if prefix != "" {
		if s == prefix {
			return after == ""
		}
		if !strings.HasPrefix(s, prefix+"/") {
			return false
		}
		s = s[len(prefix)+1:]
	}

	if after == "" {
		return true
	}

	// Try matching `after` against every path suffix of s (split at each /).
	for {
		if matchSuffix(after, s) {
			return true
		}
		slash := strings.Index(s, "/")
		if slash < 0 {
			break
		}
		s = s[slash+1:]
	}
	return false
}

func matchSuffix(pattern, s string) bool {
	if !strings.Contains(pattern, "**") {
		matched, err := path.Match(pattern, s)
		return err == nil && matched
	}
	return matchDoubleGlob(pattern, s)
}

func loadPolicy(ctx context.Context, vfs vfsservice.Service, policyPath string) (*Policy, error) {
	f, err := vfs.GetFileByID(ctx, policyPath)
	if err != nil {
		return nil, fmt.Errorf("read hitl policy %q: %w", policyPath, err)
	}
	var p Policy
	if err := json.Unmarshal(f.Data, &p); err != nil {
		return nil, fmt.Errorf("parse hitl policy %q: %w", policyPath, err)
	}
	if err := validatePolicy(&p); err != nil {
		return nil, fmt.Errorf("invalid hitl policy %q: %w", policyPath, err)
	}
	return &p, nil
}

// validatePolicy checks semantic constraints that cannot be expressed in the JSON schema.
func validatePolicy(p *Policy) error {
	validActions := map[Action]bool{ActionAllow: true, ActionApprove: true, ActionDeny: true}
	if p.DefaultAction != "" && !validActions[p.DefaultAction] {
		return fmt.Errorf("unknown default_action %q", p.DefaultAction)
	}
	for i, r := range p.Rules {
		if !validActions[r.Action] {
			return fmt.Errorf("rule %d: unknown action %q", i, r.Action)
		}
		if r.OnTimeout == ActionAllow {
			return fmt.Errorf("rule %d: on_timeout=%q is not permitted (would silently bypass approval)", i, ActionAllow)
		}
		if r.OnTimeout != "" && !validActions[r.OnTimeout] {
			return fmt.Errorf("rule %d: unknown on_timeout %q", i, r.OnTimeout)
		}
		for j, c := range r.When {
			if c.Op != OpEq && c.Op != OpGlob {
				return fmt.Errorf("rule %d, condition %d: unknown op %q", i, j, c.Op)
			}
			if c.Op == OpGlob {
				if err := validateGlobValue(c.Value); err != nil {
					return fmt.Errorf("rule %d, condition %d: %w", i, j, err)
				}
			}
		}
	}
	return nil
}

// validateGlobValue rejects glob patterns that would silently fail to match at
// runtime: unbalanced braces (path.Match would treat "{" literally) and brace
// expressions that explode past the expansion cap. A rejected policy fails to
// load, so evaluation falls back to the built-in default rather than running
// with a deny rule that never fires.
func validateGlobValue(value string) error {
	depth := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return fmt.Errorf("unbalanced '}' in glob %q", value)
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("unbalanced '{' in glob %q", value)
	}
	if len(expandGlobBraces(value)) >= maxGlobExpansions {
		return fmt.Errorf("glob %q expands past the %d-pattern limit", value, maxGlobExpansions)
	}
	return nil
}

// secretDenyRules is the universal known-bad prefix: paths that never belong
// in any agent workflow on any machine (credential stores, key material,
// shell/persistence init, system dirs). It is kept in sync with
// hitl-policy-acp.json by TestSeededACPPolicySecretInvariant.
func secretDenyRules() []Rule {
	const allTools = "*"
	q := func(g string) Rule {
		return Rule{Tools: "local_fs", Tool: allTools, Action: ActionDeny, When: []Condition{{Key: "path", Op: OpGlob, Value: g}}}
	}
	w := func(g string) []Rule {
		return []Rule{
			{Tools: "local_fs", Tool: "write_file", Action: ActionDeny, When: []Condition{{Key: "path", Op: OpGlob, Value: g}}},
			{Tools: "local_fs", Tool: "sed", Action: ActionDeny, When: []Condition{{Key: "path", Op: OpGlob, Value: g}}},
		}
	}
	rules := []Rule{
		q("**/{.ssh,.gnupg,.aws,.azure,.kube,.config/gcloud,.config/doctl,.oci}/**"),
		q("**/{.password-store,.local/share/keyrings,Library/Keychains,.config/1Password,.config/Bitwarden,.config/keepassxc}/**"),
		q("**/{.electrum,.bitcoin,.ethereum/keystore}/**"),
		q("**/{wallet.dat,*.kdbx}"),
		q("**/{.mozilla,.config/google-chrome,.config/chromium,.config/BraveSoftware}/**"),
		q("**/Library/Application Support/{Google/Chrome,Firefox,BraveSoftware}/**"),
		q("**/{.bash_history,.zsh_history,.python_history,.netrc,.git-credentials,.npmrc,.pypirc}"),
		q("**/.docker/config.json"),
		q("**/{id_rsa,id_dsa,id_ecdsa,id_ed25519}*"),
	}
	rules = append(rules, w("**/{.ssh,.gnupg}/**")...)
	rules = append(rules, w("**/{.config/autostart,.config/systemd/user,Library/LaunchAgents,Library/LaunchDaemons}/**")...)
	rules = append(rules, w("**/{.bashrc,.zshrc,.profile,.bash_profile,.zprofile,.bash_login,.zshenv,.kshrc,crontab,.crontab}")...)
	rules = append(rules, w("**/.config/fish/config.fish")...)
	rules = append(rules, w("/{etc,usr,bin,sbin,boot,lib,lib64,opt,System}/**")...)
	rules = append(rules, w("**/hitl-policy*.json")...)
	return rules
}

func defaultPolicy() *Policy {
	return &Policy{
		DefaultAction: ActionApprove,
		Rules: append(secretDenyRules(), []Rule{
			{Tools: "local_fs", Tool: "read_file", Action: ActionAllow},
			{Tools: "local_fs", Tool: "read_file_range", Action: ActionAllow},
			{Tools: "local_fs", Tool: "list_dir", Action: ActionAllow},
			{Tools: "local_fs", Tool: "grep", Action: ActionAllow},
			{Tools: "local_fs", Tool: "stat_file", Action: ActionAllow},
			{Tools: "local_fs", Tool: "count_stats", Action: ActionAllow},
			{Tools: "local_fs", Tool: "write_file", Action: ActionApprove},
			{Tools: "local_fs", Tool: "sed", Action: ActionApprove},
			{Tools: "local_shell", Tool: "local_shell", Action: ActionApprove},
			{Tools: "webtools", Tool: "web_get", Action: ActionAllow},
			{Tools: "webtools", Tool: "web_head", Action: ActionAllow},
			{Tools: "webtools", Tool: "web_post", Action: ActionApprove},
			{Tools: "webtools", Tool: "web_put", Action: ActionApprove},
			{Tools: "webtools", Tool: "web_patch", Action: ActionApprove},
			{Tools: "webtools", Tool: "web_delete", Action: ActionApprove},
			{Tools: "echo", Action: ActionAllow},
			{Tools: "print", Action: ActionAllow},
		}...),
	}
}
