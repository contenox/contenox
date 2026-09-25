package localtools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/shellline"
	"github.com/getkin/kin-openapi/openapi3"
)

const LocalExecToolsName = "local_shell"

// LocalExecResult is the structured result returned by the local_shell tools.
type LocalExecResult struct {
	ExitCode        int     `json:"exit_code"`
	Stdout          string  `json:"stdout"`
	Stderr          string  `json:"stderr"`
	Success         bool    `json:"success"`
	Error           string  `json:"error,omitempty"`
	DurationSeconds float64 `json:"duration_seconds"`
	Command         string  `json:"command,omitempty"`
	// Steps are the commands a multi-step line ran, in order, each with the
	// directory it ran in. Empty for a single command.
	Steps []string `json:"steps,omitempty"`
	Shell string   `json:"shell,omitempty"`
	OS    string   `json:"os,omitempty"`
}

// LocalExecTools runs commands on the local host (same machine as the process), opt-in and restrictable by an allowlist and optional denylist (enable via -enable-local-exec).
type LocalExecTools struct {
	defaultTimeout  time.Duration
	allowedDir      string
	allowedCommands []string
	deniedCommands  []string
	runner          CommandRunner
	shell           PlatformShell
}

// LocalExecOption configures LocalExecTools.
type LocalExecOption func(*LocalExecTools)

// WithLocalExecTimeout sets the default execution timeout.
func WithLocalExecTimeout(d time.Duration) LocalExecOption {
	return func(h *LocalExecTools) {
		h.defaultTimeout = d
	}
}

// WithLocalExecAllowedDir restricts execution to scripts/binaries under this directory.
func WithLocalExecAllowedDir(dir string) LocalExecOption {
	return func(h *LocalExecTools) {
		h.allowedDir = filepath.Clean(dir)
	}
}

// WithLocalExecAllowedCommands restricts execution to these executable names/paths.
func WithLocalExecAllowedCommands(commands []string) LocalExecOption {
	return func(h *LocalExecTools) {
		h.allowedCommands = commands
	}
}

// WithLocalExecDeniedCommands forbids these executable basenames or paths (checked before allowlist).
func WithLocalExecDeniedCommands(commands []string) LocalExecOption {
	return func(h *LocalExecTools) {
		h.deniedCommands = commands
	}
}

// WithLocalExecShell sets the detected platform shell used for shell:true calls
// and for tool schema descriptions.
func WithLocalExecShell(shell PlatformShell) LocalExecOption {
	return func(h *LocalExecTools) {
		h.shell = shell.WithDefaults()
	}
}

// NewLocalExecTools creates a new LocalExecTools with the given options.
func NewLocalExecTools(opts ...LocalExecOption) taskengine.ToolsRepo {
	return NewLocalExecToolsWith(nil, opts...)
}

func NewLocalExecToolsWith(runner CommandRunner, opts ...LocalExecOption) taskengine.ToolsRepo {
	h := &LocalExecTools{
		// Generous default: verification commands (build, test suite, npm ci) are slow; per-call `timeout` still narrows this for a command that should be quick.
		defaultTimeout: 10 * time.Minute,
		shell:          DetectPlatformShell(),
	}
	for _, opt := range opts {
		opt(h)
	}
	if runner == nil {
		runner = noTerminalRunner{}
	}
	h.runner = runner
	return h
}

// policy is the resolved command policy for one call: the chain's tools_policies
// entry when the call runs under one, the struct defaults otherwise.
func (h *LocalExecTools) policy(ctx context.Context) commandPolicy {
	allowedCommands, allowedDir, deniedCommands := h.resolvePolicy(ctx)
	return commandPolicy{allowed: allowedCommands, denied: deniedCommands, dir: allowedDir}
}

func (h *LocalExecTools) resolvePolicy(ctx context.Context) (allowedCommands []string, allowedDir string, deniedCommands []string) {
	if args := taskengine.ToolsArgsFromContext(ctx, LocalExecToolsName); len(args) > 0 {
		if v := args["_allowed_commands"]; v != "" {
			allowedCommands = splitTrimmed(v)
		}
		if v := args["_allowed_dir"]; v != "" {
			allowedDir = filepath.Clean(v)
		}
		if v := args["_denied_commands"]; v != "" {
			deniedCommands = splitTrimmed(v)
		}
		return
	}
	return h.allowedCommands, h.allowedDir, h.deniedCommands
}

func splitTrimmed(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Exec implements taskengine.ToolsRepo: input is stdin when it is a string or the map has "stdin", command falls back to input when tools.Args is nil, and accepted args are command (required), args, cwd, timeout, shell.
func (h *LocalExecTools) Exec(ctx context.Context, startTime time.Time, input any, debug bool, tools *taskengine.ToolsCall) (any, taskengine.DataType, error) {
	if tools == nil {
		return nil, taskengine.DataTypeAny, errors.New("local_shell: tools required")
	}
	if tools.Args == nil {
		tools.Args = make(map[string]string)
	}
	command, argsSlice, cwd, timeout, useShell, stdin, err := h.parseArgs(tools, input)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	steps, err := h.planLine(command, argsSlice, cwd, useShell, h.policy(ctx))
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	result, err := h.runPlan(ctx, command, steps, timeout, stdin)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	return result, taskengine.DataTypeJSON, nil
}

var _ taskengine.Prechecker = (*LocalExecTools)(nil)

// Precheck applies the command policy (denylist, allowed dir, allowlist) and runs nothing, satisfying [taskengine.Prechecker]; it is an early copy, never a replacement — [LocalExecTools.Exec] applies the same policy at execution time since this boundary must hold with nothing wrapping it.
func (h *LocalExecTools) Precheck(ctx context.Context, input any, tools *taskengine.ToolsCall) error {
	if tools == nil {
		return errors.New("local_shell: tools required")
	}
	command, argsSlice, cwd, _, useShell, _, err := h.parseArgs(tools, input)
	if err != nil {
		return err
	}
	_, err = h.planLine(command, argsSlice, cwd, useShell, h.policy(ctx))
	return err
}

func (h *LocalExecTools) parseArgs(tools *taskengine.ToolsCall, input any) (command string, argsSlice []string, cwd string, timeout time.Duration, useShell bool, stdin string, err error) {
	timeout = h.defaultTimeout
	get := func(k string) string { return tools.Args[k] }
	if cmd := get("command"); cmd != "" {
		command = cmd
	}
	if a := get("args"); a != "" {
		argsSlice = splitShellArgs(a)
	}
	if d := get("cwd"); d != "" {
		cwd = filepath.Clean(d)
		if absCwd, err := filepath.Abs(cwd); err == nil {
			cwd = absCwd
		}
	}
	if t := get("timeout"); t != "" {
		if d, e := time.ParseDuration(t); e == nil {
			timeout = d
		}
	}
	if s := get("shell"); s != "" {
		useShell = strings.EqualFold(s, "true") || s == "1"
	}
	switch v := input.(type) {
	case string:
		stdin = v
		if command == "" {
			command = v
			if useShell {
				argsSlice = nil
			}
		}
	case map[string]any:
		if err := rejectUnknownArgs(LocalExecToolsName, v, "command", "args", "cwd", "timeout", "shell", "stdin"); err != nil {
			return "", nil, "", 0, false, "", err
		}
		if cmd, ok := v["command"].(string); ok && command == "" {
			command = cmd
		}
		if s, ok := v["stdin"].(string); ok {
			stdin = s
		}
		if s, ok := v["shell"].(bool); ok && !useShell {
			useShell = s
		} else if s, ok := v["shell"].(string); ok && !useShell {
			useShell = strings.EqualFold(s, "true") || s == "1"
		}
		if a, ok := v["args"]; ok && len(argsSlice) == 0 {
			parsed, err := stringSliceArg(LocalExecToolsName, "args", a)
			if err != nil {
				return "", nil, "", 0, false, "", err
			}
			argsSlice = parsed
		}
		if d, ok := v["cwd"].(string); ok && cwd == "" {
			cwd = filepath.Clean(d)
			if absCwd, err := filepath.Abs(cwd); err == nil {
				cwd = absCwd
			}
		}
		if t, ok := v["timeout"].(string); ok {
			if d, e := time.ParseDuration(t); e == nil && timeout == h.defaultTimeout {
				timeout = d
			}
		}
	}
	if command == "" {
		return "", nil, "", 0, false, "", errors.New("local_shell: command is required (tools.args.command or input)")
	}
	return command, argsSlice, cwd, timeout, useShell, stdin, nil
}

type capWriter struct {
	buf       bytes.Buffer
	limit     int64
	written   int64
	truncated bool
}

func (cw *capWriter) Write(p []byte) (n int, err error) {
	if cw.limit > 0 {
		if cw.written >= cw.limit {
			cw.truncated = true
			return 0, io.ErrShortWrite
		}
		writeLen := int64(len(p))
		if cw.written+writeLen > cw.limit {
			writeLen = cw.limit - cw.written
			cw.buf.Write(p[:writeLen])
			cw.written += writeLen
			cw.truncated = true
			return int(writeLen), io.ErrShortWrite
		}
	}
	n, err = cw.buf.Write(p)
	cw.written += int64(n)
	return n, err
}

func (h *LocalExecTools) runPlan(ctx context.Context, command string, steps []commandStep, timeout time.Duration, stdinStr string) (*LocalExecResult, error) {
	start := time.Now()
	shell := h.shell.WithDefaults()
	result := &LocalExecResult{Command: command, Shell: shell.Summary(), OS: shell.OS}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	limit := int64(2 * 1024 * 1024) // 2MB default fallback if no budget is provided
	if val, ok := ctx.Value(taskengine.ContextKeyOutputByteLimit).(int64); ok {
		limit = val
	}

	// Never silent: spool writers keep a 20%-head/80%-tail slice inline (errors cluster at the tail) and spill the full stream to a durable file named in the result.
	stdout := newSpoolWriter(ctx, "local_shell-stdout", limit)
	stderr := newSpoolWriter(ctx, "local_shell-stderr", limit)

	// A line's steps share one budget and one timeout, the way one shell line would. `&&` runs the next step only after a success, `||` only after a failure, `;` either way; the line's own status is the last status it reached.
	var failures []string
	lastCode := 0
	lastOK := true
	lastErr := error(nil)
	shortCircuit := ""
	for i, step := range steps {
		switch step.op {
		case shellline.OpAnd:
			if !lastOK {
				shortCircuit = fmt.Sprintf("step %d (%s) exited with status %d, so the steps joined to it with '&&' did not run", i, steps[i-1].display(), lastCode)
			}
		case shellline.OpOr:
			if lastOK {
				continue
			}
		}
		if shortCircuit != "" {
			break
		}
		if len(steps) > 1 {
			result.Steps = append(result.Steps, stepDisplay(step))
		}
		if len(step.argv) == 0 {
			// A `cd` step ran nothing: it named the directory of the steps that
			// follow, and the planner already checked that it is one.
			continue
		}
		spec := CommandSpec{
			Command:  step.argv[0],
			Args:     step.argv[1:],
			Cwd:      step.cwd,
			Timeout:  timeout,
			UseShell: step.useShell,
			Shell:    shell,
		}
		if i == 0 {
			spec.Stdin = stdinStr
		}
		code, runErr := h.runner.Run(runCtx, spec, stdout, stderr)
		lastCode, lastErr, lastOK = code, runErr, runErr == nil && code == 0
		if !lastOK {
			failures = append(failures, stepFailure(i, step, code, runErr))
		}
	}
	result.DurationSeconds = time.Since(start).Seconds()

	if errors.Is(lastErr, ErrOutputBudgetExceeded) {
		// Backend already truncated its own stream; the partial bytes are from an incomplete write and must be discarded, not surfaced (a poisoned partial is worse than nothing).
		stdout.discard()
		stderr.discard()
		result.Steps = nil
		result.Success = false
		result.ExitCode = -1
		result.Error = fmt.Sprintf("Output truncated: command exceeded the context budget (%d bytes). Re-run with a narrower scope or redirect output to a file. %s", limit, severityRecoverable)
		return result, nil
	}

	stdoutPath := stdout.close()
	stderrPath := stderr.close()

	result.Stdout = strings.TrimRight(stdout.inlineOutput(), "\r\n")
	result.Stderr = strings.TrimRight(stderr.inlineOutput(), "\r\n")

	if stdout.truncated() || stderr.truncated() {
		// Never silent: Stdout/Stderr carry the 20/80 split with an embedded spool pointer; a spool that could not be written is the only fatal case here (disk full / spool unwritable).
		result.Success = false
		result.ExitCode = -1
		var parts []string
		if stdout.truncated() {
			parts = append(parts, spoolNotice("stdout", stdout, stdoutPath, limit))
		}
		if stderr.truncated() {
			parts = append(parts, spoolNotice("stderr", stderr, stderrPath, limit))
		}
		result.Error = strings.Join(parts, " ") + " " + truncationSeverity(stdout, stderr)
		return result, nil
	}

	result.ExitCode = lastCode
	result.Success = lastOK
	switch {
	case shortCircuit != "":
		result.Error = shortCircuit + " " + severityRecoverable
	case lastErr != nil:
		result.Error = lastErr.Error() + " " + severityRecoverable
	case len(failures) > 0:
		// The line carried on past a failure because of how it is joined, and the
		// last status is what the caller branches on — so say what failed anyway.
		result.Error = strings.Join(failures, "; ") + " " + severityRecoverable
	case !lastOK:
		result.Error = fmt.Sprintf("command exited with status %d %s", lastCode, severityRecoverable)
	}
	return result, nil
}

func stepFailure(index int, step commandStep, code int, runErr error) string {
	reason := fmt.Sprintf("exited with status %d", code)
	if runErr != nil {
		reason = runErr.Error()
	}
	return fmt.Sprintf("step %d (%s) %s", index, step.display(), reason)
}

func stepDisplay(step commandStep) string {
	if step.cd != "" {
		return "cd " + step.cd
	}
	return fmt.Sprintf("%s [in %s]", step.display(), step.cwd)
}

func spoolNotice(stream string, w *spoolWriter, path string, budget int64) string {
	if w.spoolErr != nil {
		return fmt.Sprintf("Output truncated: %s exceeded the context budget (%d bytes); the full output could not be spooled (%v).", stream, budget, w.spoolErr)
	}
	if w.overCap {
		return fmt.Sprintf("Output truncated: %s exceeded the context budget (%d bytes); showing the first 20%% and last 80%%; full output (first %s of a larger stream): %s.", stream, budget, humanSize(w.spooled), path)
	}
	return fmt.Sprintf("Output truncated: %s exceeded the context budget (%d bytes); showing the first 20%% and last 80%%; full output: %s (%s).", stream, budget, path, humanSize(w.total))
}

func truncationSeverity(stdout, stderr *spoolWriter) string {
	if stdout.spoolErr != nil || stderr.spoolErr != nil {
		reason := "spool unwritable"
		if isDiskFull(stdout.spoolErr) || isDiskFull(stderr.spoolErr) {
			reason = "disk full"
		}
		return "(fatal: " + reason + ")"
	}
	return severityRecoverable
}

func splitShellArgs(s string) []string {
	var args []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			continue
		}
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if (r == ' ' || r == '\t' || r == '\n') && !inSingle && !inDouble {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// Supports implements taskengine.ToolsRegistry.
func (h *LocalExecTools) Supports(ctx context.Context) ([]string, error) {
	return []string{LocalExecToolsName}, nil
}

// GetSchemasForSupportedTools implements taskengine.ToolsWithSchema.
func (h *LocalExecTools) GetSchemasForSupportedTools(ctx context.Context) (map[string]*openapi3.T, error) {
	shellDesc := h.shell.ShellModeDescription()
	schema := &openapi3.T{
		OpenAPI: "3.1.0",
		Info:    &openapi3.Info{Title: "Local Exec Tools", Description: "Run commands on the local host. command is a command line — `git status`, `cd sub && go test ./...` — and every program in it is checked against the command policy. " + shellDesc, Version: "1.0.0"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: map[string]*openapi3.SchemaRef{
				"LocalExecRequest": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{openapi3.TypeObject},
						Properties: map[string]*openapi3.SchemaRef{
							"command": {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}, Description: "The command line to run: {\"command\": \"ls -F\"} or {\"command\": \"cd sub && go test ./...\"}. Every program in it must be allowed by the command policy; cd moves the commands after it and needs no allowlist entry"}},
							"args": {Value: &openapi3.Schema{
								OneOf: []*openapi3.SchemaRef{
									{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}, Description: "Space-separated arguments string"}},
									{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}}, Description: "Array of argument strings"}},
								},
								Description: "Everything after the executable, as an array of strings or a space-separated string",
							}},
							"cwd":     {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}, Description: "Working directory"}},
							"timeout": {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}, Description: "Duration e.g. 30s"}},
							"shell":   {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeBoolean}, Description: shellDesc}},
						},
						Required: []string{"command"},
					},
				},
				"LocalExecResponse": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{openapi3.TypeObject},
						Properties: map[string]*openapi3.SchemaRef{
							"exit_code":        {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}},
							"stdout":           {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
							"stderr":           {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
							"success":          {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeBoolean}}},
							"error":            {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
							"duration_seconds": {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeNumber}}},
							"command":          {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
							"steps":            {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}}, Description: "Each command a multi-step line ran, with the directory it ran in"}},
							"shell":            {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
							"os":               {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
						},
					},
				},
			},
		},
	}
	return map[string]*openapi3.T{LocalExecToolsName: schema}, nil
}

// GetToolsForToolsByName implements taskengine.ToolsWithSchema; chain-level policy (allowed/denied commands, allowed dir) is read from ctx via ToolsArgsFromContext when present, falling back to the struct defaults.
func (h *LocalExecTools) GetToolsForToolsByName(ctx context.Context, name string) ([]taskengine.Tool, error) {
	if name != LocalExecToolsName {
		return nil, fmt.Errorf("unknown tools: %s", name)
	}
	allowedCommands, allowedDir, deniedCommands := h.resolvePolicy(ctx)
	shellDesc := h.shell.ShellModeDescription()
	desc := "Run a terminal command on the local host. command is a command line: {\"command\": \"git status\"} runs git status, {\"command\": \"cd sub && go test ./...\"} moves into sub first. Every program in the line is checked against the command policy; `cd` is interpreted — it moves the commands after it, cannot leave the allowed directory, and needs no allowlist entry. Alternatively put the executable in command and its operands in args, which are passed through as one argv and never read as syntax. shell: true is accepted but spawns no shell while a policy is active: the line is read either way, and a shape that needs a real shell is refused rather than interpreted. Under a command policy the tool refuses anything it cannot check before it runs: pipes, redirection, globs, $VAR and $(...) expansions, escapes, & backgrounding, `!`, variable assignments as prefixes, and compound constructs (if/for/while/case/subshell). Run those as separate calls, and select paths with find or grep instead of a glob. A refusal against a command policy is machine configuration that no approval can widen, so answer it with a different command rather than an escalation. Returns {stdout, stderr, exitCode, success, durationSeconds}. Output is capped at the remaining context budget; when it exceeds the cap the FULL output is spooled to a file and stdout/stderr instead carry a 20%-head/80%-tail slice (errors cluster at the tail) with an inline pointer, while the error field names the spool concretely ('full output: <path> (N KiB)') so nothing is lost silently. Errors carry a severity marker you can key on: '(recoverable: adjust parameters and retry)' for anything a corrected call fixes (output too large, bad command, denied path), and '(fatal: <reason>)' only when the environment is broken (disk full, spool unwritable) and retrying will not help. For file operations prefer local_fs.* tools: read_file, read_file_range, write_file, edit_file, sed. They enforce sandbox boundaries, size limits, and a read-before-write contract that local_shell does not. Use local_shell for operations with no dedicated tool: running tests, builds, git commands, environment inspection. " + shellDesc
	if len(allowedCommands) > 0 {
		desc += " Allowed commands: " + strings.Join(allowedCommands, ", ") + "."
	}
	if allowedDir != "" {
		desc += " Commands must reside under: " + allowedDir + "."
	}
	if len(deniedCommands) > 0 {
		desc += " Denied commands: " + strings.Join(deniedCommands, ", ") + "."
	}

	// Shell mode is rejected at execution time when a policy is active; communicated in prose here since Gemini rejects boolean enum values in tool declarations.
	policyActive := len(allowedCommands) > 0 || allowedDir != "" || len(deniedCommands) > 0
	var shellProp map[string]interface{}
	if policyActive {
		shellProp = map[string]interface{}{
			"type":        "boolean",
			"description": "Accepted but inert while a command policy is active: no shell is spawned, command is read as a command line, and anything needing a real shell is refused.",
		}
	} else {
		shellProp = map[string]interface{}{
			"type":        "boolean",
			"description": shellDesc,
		}
	}

	return []taskengine.Tool{
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "local_shell",
				Description: desc,
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "The command line to run (required): {\"command\": \"ls -F\"}, or {\"command\": \"cd sub && go test ./...\"}. Every program in it must be allowed by the command policy; `cd` moves the commands after it and needs no allowlist entry",
						},
						"args": map[string]interface{}{
							"oneOf": []interface{}{
								map[string]interface{}{
									"type":        "string",
									"description": "Space-separated arguments string",
								},
								map[string]interface{}{
									"type":        "array",
									"items":       map[string]interface{}{"type": "string"},
									"description": "Array of argument strings",
								},
							},
							"description": "Operands for a single executable, as an array of strings or a space-separated string; passed through literally, never read as syntax",
						},
						"cwd": map[string]interface{}{
							"type":        "string",
							"description": "Working directory",
						},
						"timeout": map[string]interface{}{
							"type":        "string",
							"description": "Duration e.g. 30s",
						},
						"shell": shellProp,
					},
					"required": []string{"command"},
				},
			},
		},
	}, nil
}

var _ taskengine.ToolsRepo = (*LocalExecTools)(nil)

type noTerminalRunner struct{}

func (noTerminalRunner) Run(context.Context, CommandSpec, io.Writer, io.Writer) (int, error) {
	return 0, ErrNoTerminal
}
