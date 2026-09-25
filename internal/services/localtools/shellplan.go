package localtools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/contenox/contenox/internal/services/shellline"
)

// commandPolicy is one call's resolved command policy: the programs that may
// run, the ones that may not, and the directory a program has to stay inside.
// A zero field is a dimension the policy does not constrain.
type commandPolicy struct {
	allowed []string
	denied  []string
	dir     string
}

func (p commandPolicy) active() bool {
	return len(p.allowed) > 0 || len(p.denied) > 0 || strings.TrimSpace(p.dir) != ""
}

// commandStep is one command of a planned line: its argv, the directory it runs
// in, and the operator joining it to the step before.
type commandStep struct {
	op   shellline.Operator
	argv []string
	cwd  string
	// cd is the directory this step moves to, which the step reports instead
	// of running anything.
	cd string
	// useShell is set only where no policy is active, where the call may ask
	// for the shell the platform provides.
	useShell bool
}

// displayNames the step the way a result or a refusal quotes it.
func (s commandStep) display() string {
	if len(s.argv) == 0 {
		return ""
	}
	return shellline.DisplayWords(s.argv)
}

// planLine reads one tool call into the commands it would run and checks every
// one of them against the policy. A call that supplies args is a single argv
// and is checked as one; a command string is a command line, so `git status`,
// `cd sub && go test ./...` and `a && b` are read as the commands they name.
// shell: true does not change that: under a policy no shell is spawned at all,
// so the flag only widens which shapes reach the checks — and a shape needing a
// real shell is refused by name instead of being interpreted by one.
//
// What the policy cannot vouch for is refused rather than guessed at: anything
// only known at run time (globs, expansions, substitutions, escapes), anything
// that needs a shell to interpret (pipelines, redirection, background), and any
// shell whose lines this build cannot read. Refusing is the whole point — an
// unreadable line that ran anyway would make the policy decorative.
func (h *LocalExecTools) planLine(command string, argsSlice []string, cwd string, useShell bool, policy commandPolicy) ([]commandStep, error) {
	kind := shellline.Kind(shellKindOf(h.shell))
	if !policy.active() {
		return []commandStep{{argv: append([]string{command}, argsSlice...), cwd: cwd, useShell: useShell}}, nil
	}
	// shell: true asks for a shell, and under a policy none is ever spawned:
	// the line is read and run as arguments instead, so what the policy checked
	// is exactly what runs. Shapes that only a shell could honour are refused
	// below, by name, rather than being handed to one.
	if useShell && len(argsSlice) > 0 {
		return h.planCommandLine(strings.TrimSpace(command+" "+strings.Join(argsSlice, " ")), cwd, kind, policy)
	}
	if len(argsSlice) > 0 {
		if err := h.checkProgramPolicy(command, cwd, policy); err != nil {
			return nil, err
		}
		return []commandStep{{argv: append([]string{command}, argsSlice...), cwd: cwd}}, nil
	}
	return h.planCommandLine(command, cwd, kind, policy)
}

func (h *LocalExecTools) planCommandLine(command, cwd string, kind shellline.Kind, policy commandPolicy) ([]commandStep, error) {
	line := shellline.Parse(command, kind, runtime.GOOS)
	if !line.Readable {
		return nil, fmt.Errorf("local_shell: command lines on this shell cannot be checked against the command policy; "+
			"pass the executable in command and its flags in args, one call per step (%s)", shellline.DisplayWords([]string{command}))
	}
	if !line.Parsed {
		return nil, fmt.Errorf("local_shell: %s could not be read as a command line; "+
			"pass the executable in command and its flags in args, and quote nothing that has to survive as one argument",
			shellline.DisplayWords([]string{command}))
	}
	switch {
	case line.Control:
		return nil, refusalNeedsShell(command, "is a compound construct (if/for/while/case/subshell/function)")
	case line.Negated, line.Coprocess, line.Background:
		return nil, refusalNeedsShell(command, "runs in the background or negates a status")
	case len(line.Redirects) > 0:
		return nil, refusalNeedsShell(command, "redirects a stream")
	case line.CommandSubst, line.ProcessSubst, line.ArithmExp:
		return nil, refusalNeedsShell(command, "contains a command, process or arithmetic substitution")
	case line.BashOnly:
		return nil, refusalNeedsShell(command, "is read by the wider shell grammar only, so what runs is not what was read")
	case line.Pipelines > 0:
		return nil, refusalNeedsShell(command, "joins commands with a pipe")
	case len(line.Steps) == 0:
		return nil, fmt.Errorf("local_shell: %s names no command to run", shellline.DisplayWords([]string{command}))
	}

	base := cwd
	if strings.TrimSpace(base) == "" {
		if wd, err := os.Getwd(); err == nil {
			base = wd
		}
	}
	steps := make([]commandStep, 0, len(line.Steps))
	for _, step := range line.Steps {
		argv, next, err := h.planStep(step, base, policy)
		if err != nil {
			return nil, err
		}
		steps = append(steps, commandStep{op: step.Operator, argv: argv, cwd: base, cd: changeDir(step, base)})
		base = next
	}
	return steps, nil
}

// planStep checks one simple command and returns the argv to run and the
// directory the steps after it run in. `cd` is interpreted, never executed: it
// names the directory of the steps that follow, so it needs no allowlist entry
// of its own — and it cannot leave the policy's directory.
func (h *LocalExecTools) planStep(step shellline.Step, cwd string, policy commandPolicy) ([]string, string, error) {
	if len(step.Assigns) > 0 {
		return nil, cwd, fmt.Errorf("local_shell: %s sets %s in front of the command, which changes the environment of everything after it; "+
			"pass the variable to the program as an argument instead", step.Display, strings.Join(step.Assigns, ", "))
	}
	if !step.Literal || len(step.Words) == 0 {
		return nil, cwd, fmt.Errorf("local_shell: %s carries a value only known at run time (a glob, expansion or escape), "+
			"so what would run cannot be checked; pass the literal value, or use find/grep to select paths", step.Display)
	}
	if step.Base == "cd" {
		return planChangeDir(step, cwd, policy)
	}
	if err := h.checkProgramPolicy(step.Words[0], cwd, policy); err != nil {
		return nil, cwd, err
	}
	return step.Words, cwd, nil
}

func planChangeDir(step shellline.Step, cwd string, policy commandPolicy) ([]string, string, error) {
	if len(step.Words) != 2 {
		return nil, cwd, fmt.Errorf("local_shell: %s needs exactly one directory; `cd` alone has no meaning here — "+
			"pass the directory, or the call's cwd parameter", step.Display)
	}
	if step.Words[1] == "-" || step.Words[1] == "--" {
		return nil, cwd, fmt.Errorf("local_shell: %s has no meaning here: the shell's previous directory is not tracked", step.Display)
	}
	target := step.Words[1]
	if !filepath.IsAbs(target) {
		target = filepath.Join(cwd, target)
	}
	// Resolved before it is trusted: a symlink inside the allowed directory
	// that points out of it would otherwise carry every later step with it.
	resolved, err := filepath.EvalSymlinks(filepath.Clean(target))
	if err != nil {
		return nil, cwd, fmt.Errorf("local_shell: cd %s: %w", step.Words[1], err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, cwd, fmt.Errorf("local_shell: cd %s: %w", step.Words[1], err)
	}
	if !info.IsDir() {
		return nil, cwd, fmt.Errorf("local_shell: cd %s: not a directory", step.Words[1])
	}
	if err := checkDirPolicy(resolved, policy, "cd "+step.Words[1]); err != nil {
		return nil, cwd, err
	}
	return nil, resolved, nil
}

// checkProgramPolicy resolves one program and applies the denylist, the
// allowlist and the directory rule to it. Resolution follows what the executor
// will do: an absolute or pathed word resolves against the directory the step
// runs in, a bare name is looked up on PATH.
func (h *LocalExecTools) checkProgramPolicy(spelled, cwd string, policy commandPolicy) error {
	resolved := resolveProgramPath(spelled, cwd)
	if err := checkDenied(resolved, spelled, policy.denied); err != nil {
		return err
	}
	if err := checkDirPolicy(resolved, policy, spelled); err != nil {
		return err
	}
	return checkAllowed(resolved, spelled, policy.allowed)
}

func resolveProgramPath(spelled, cwd string) string {
	if filepath.IsAbs(spelled) {
		return filepath.Clean(spelled)
	}
	if strings.ContainsRune(spelled, filepath.Separator) {
		return filepath.Clean(filepath.Join(cwd, spelled))
	}
	if found, err := exec.LookPath(spelled); err == nil {
		return found
	}
	return filepath.Clean(spelled)
}

func checkDenied(resolved, spelled string, denied []string) error {
	if len(denied) == 0 {
		return nil
	}
	base := filepath.Base(resolved)
	for _, d := range denied {
		dClean := filepath.Clean(d)
		if dClean == resolved || dClean == spelled || filepath.Base(dClean) == base || dClean == base {
			return fmt.Errorf("local_shell: command %s is denied by policy", spelled)
		}
	}
	return nil
}

func checkDirPolicy(resolved string, policy commandPolicy, spelled string) error {
	if strings.TrimSpace(policy.dir) == "" {
		return nil
	}
	absDir, err := filepath.Abs(policy.dir)
	if err != nil {
		return fmt.Errorf("local_shell: allowed dir invalid: %w", err)
	}
	if withinDir(absDir, resolved) {
		return nil
	}
	return fmt.Errorf("local_shell: command %s is not under allowed dir %s", spelled, policy.dir)
}

func withinDir(dir, candidate string) bool {
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(dir, abs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func checkAllowed(resolved, spelled string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	for _, c := range allowed {
		cClean := filepath.Clean(c)
		if cClean == resolved || cClean == spelled {
			return nil
		}
		if path, err := exec.LookPath(c); err == nil && path == resolved {
			return nil
		}
	}
	return fmt.Errorf("local_shell: command %s is not in this chain's allowed commands (%s); "+
		"no approval can grant it — add it to _allowed_commands under the local_shell entry in "+
		"the chain's tools_policies, or run it outside the agent", spelled, strings.Join(allowed, ", "))
}

// changeDir names the directory a `cd` step moves to, for the result's step
// list; a step that runs a program has none.
func changeDir(step shellline.Step, cwd string) string {
	if step.Base != "cd" || len(step.Words) != 2 {
		return ""
	}
	target := step.Words[1]
	if !filepath.IsAbs(target) {
		target = filepath.Join(cwd, target)
	}
	return filepath.Clean(target)
}

func refusalNeedsShell(command, why string) error {
	return fmt.Errorf("local_shell: %s %s, which needs a shell to interpret; a policy is active, so it cannot be run. "+
		"Run each step as its own call and pass paths to the program instead of to a shell",
		shellline.DisplayWords([]string{command}), why)
}

// shellKindOf names the shell a platform shell spawns, which is what decides
// whether a command line can be read at all.
func shellKindOf(shell PlatformShell) string {
	switch shell.WithDefaults().Kind {
	case ShellKindSh:
		return string(shellline.KindPOSIX)
	case ShellKindPowerShell:
		return string(shellline.KindPowerShell)
	case ShellKindCmd:
		return string(shellline.KindCmd)
	default:
		return string(shellline.KindUnknown)
	}
}
