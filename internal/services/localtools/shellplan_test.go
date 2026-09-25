package localtools_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/localtools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runLine(t *testing.T, policy map[string]string, args map[string]string) (*localtools.LocalExecResult, error) {
	t.Helper()
	h := localtools.NewLocalExecToolsWith(localtools.NewTestHostRunner()).(*localtools.LocalExecTools)
	ctx := taskengine.WithToolsArgs(context.Background(), "local_shell", policy)
	toolsCall := &taskengine.ToolsCall{Name: "local_shell", Args: args}
	out, _, err := h.Exec(ctx, time.Now().UTC(), nil, false, toolsCall)
	if err != nil {
		return nil, err
	}
	res, ok := out.(*localtools.LocalExecResult)
	require.True(t, ok)
	return res, nil
}

func TestUnit_ShellPlan_CommandIsALineNotJustAProgram(t *testing.T) {
	res, err := runLine(t, map[string]string{"_allowed_commands": "echo"}, map[string]string{"command": "echo one two"})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Error)
	assert.Equal(t, "one two", strings.TrimSpace(res.Stdout))
	assert.Empty(t, res.Steps, "a single command needs no step list")
}

func TestUnit_ShellPlan_EveryProgramOfALineIsChecked(t *testing.T) {
	_, err := runLine(t, map[string]string{"_allowed_commands": "echo"}, map[string]string{"command": "echo ok && ls /"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ls is not in this chain's allowed commands")

	res, err := runLine(t, map[string]string{"_allowed_commands": "echo,ls"}, map[string]string{"command": "echo ok && ls -d /tmp"})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Error)
	assert.Contains(t, res.Stdout, "ok")
	assert.Contains(t, res.Stdout, "/tmp")
	assert.Len(t, res.Steps, 2, "both commands of the line are reported")
}

func TestUnit_ShellPlan_ChangeDirAppliesToTheStepsAfterIt(t *testing.T) {
	dir := t.TempDir()
	res, err := runLine(t, map[string]string{"_allowed_commands": "pwd"}, map[string]string{"command": "cd " + dir + " && pwd"})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Error)
	assert.Equal(t, dir, strings.TrimSpace(res.Stdout))
	assert.Len(t, res.Steps, 2)
	assert.Contains(t, res.Steps[0], "cd "+dir)
}

func TestUnit_ShellPlan_ChangeDirCannotLeaveTheAllowedDir(t *testing.T) {
	root := t.TempDir()
	script := writeScript(t, filepath.Join(root, "bin"))
	policy := map[string]string{"_allowed_commands": script, "_allowed_dir": root}

	res, err := runLine(t, policy, map[string]string{"command": "./run.sh", "cwd": filepath.Join(root, "bin")})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Error)
	assert.Equal(t, "ran", strings.TrimSpace(res.Stdout))

	_, err = runLine(t, policy, map[string]string{"command": "cd ../.. && ./run.sh", "cwd": filepath.Join(root, "bin")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not under allowed dir")

	_, err = runLine(t, policy, map[string]string{"command": "cd / && ./run.sh", "cwd": filepath.Join(root, "bin")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not under allowed dir")

	res, err = runLine(t, policy, map[string]string{"command": "cd sub && ../run.sh", "cwd": filepath.Join(root, "bin")})
	if err == nil {
		assert.False(t, res.Success, "a path outside the allowed dir must not run")
	}
	_ = script
}

func TestUnit_ShellPlan_RefusesWhatThePolicyCannotVouchFor(t *testing.T) {
	for src, want := range map[string]string{
		`cat *.go`:                "only known at run time",
		`cat a?b`:                 "only known at run time",
		`echo $HOME`:              "only known at run time",
		`echo a\ b`:               "only known at run time",
		`ls > out`:                "redirects a stream",
		`cat < in`:                "redirects a stream",
		`ls | grep a`:             "joins commands with a pipe",
		`ls &`:                    "runs in the background",
		`! ls`:                    "negates a status",
		`echo $(id)`:              "substitution",
		`cat <(ls)`:               "substitution",
		`echo $((1+1))`:           "substitution",
		`if true; then ls; fi`:    "compound construct",
		`(ls)`:                    "compound construct",
		`for f in *; do ls; done`: "compound construct",
		`FOO=1 ls`:                "changes the environment",
		`echo 'unterminated`:      "could not be read as a command line",
		`cd`:                      "needs exactly one directory",
		`cd -`:                    "has no meaning here",
		`cd /nonexistent/dir`:     "no such file or directory",
	} {
		_, err := runLine(t, map[string]string{"_allowed_commands": "ls,cat,echo,true"}, map[string]string{"command": src})
		require.Errorf(t, err, "%q must be refused", src)
		assert.Containsf(t, err.Error(), want, "refusing %q", src)
	}
}

func TestUnit_ShellPlan_ShortCircuitsAndReportsTheStepThatStoppedIt(t *testing.T) {
	res, err := runLine(t, map[string]string{"_allowed_commands": "ls,echo"}, map[string]string{"command": "ls /nonexistent && echo after"})
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.NotContains(t, res.Stdout, "after")
	assert.Contains(t, res.Error, "did not run")
	assert.Contains(t, res.Error, "&&")
}

func TestUnit_ShellPlan_SequenceContinuesAndStillReportsTheFailure(t *testing.T) {
	res, err := runLine(t, map[string]string{"_allowed_commands": "ls,echo"}, map[string]string{"command": "ls /nonexistent ; echo after"})
	require.NoError(t, err)
	assert.True(t, res.Success, "the line's own status is the last command's")
	assert.Equal(t, 0, res.ExitCode)
	assert.Contains(t, res.Stdout, "after")
	assert.Contains(t, res.Error, "step 0", "a failure the line carried on past is still reported")
}

func TestUnit_ShellPlan_FallbackRunsAfterAFailure(t *testing.T) {
	res, err := runLine(t, map[string]string{"_allowed_commands": "ls,echo"}, map[string]string{"command": "ls /nonexistent || echo fallback"})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Error)
	assert.Contains(t, res.Stdout, "fallback")
}

func TestUnit_ShellPlan_ArgsStayOneArgv(t *testing.T) {
	res, err := runLine(t, map[string]string{"_allowed_commands": "echo"}, map[string]string{"command": "echo", "args": "a && ls /"})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Error)
	assert.Equal(t, "a && ls /", strings.TrimSpace(res.Stdout), "args are arguments, never syntax")
}

func TestUnit_ShellPlan_ShellModeNoLongerStopsACheckableLine(t *testing.T) {
	res, err := runLine(t, map[string]string{"_allowed_commands": "echo"}, map[string]string{"command": "echo hi", "shell": "true"})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Error)
	assert.Equal(t, "hi", strings.TrimSpace(res.Stdout))

	res, err = runLine(t, map[string]string{"_allowed_commands": "echo"}, map[string]string{"command": "echo", "args": "one && echo two", "shell": "true"})
	require.NoError(t, err, "with shell mode the args are part of the line")
	assert.True(t, res.Success, res.Error)
	assert.Contains(t, res.Stdout, "one")
	assert.Contains(t, res.Stdout, "two")

	_, err = runLine(t, map[string]string{"_allowed_commands": "echo,cat"}, map[string]string{"command": "echo hi | cat", "shell": "true"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "joins commands with a pipe")
}

func TestUnit_ShellPlan_WithoutPolicyTheShellIsStillAvailable(t *testing.T) {
	res, err := runLine(t, nil, map[string]string{"command": "echo one && echo two", "shell": "true"})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Error)
	assert.Contains(t, res.Stdout, "one")
	assert.Contains(t, res.Stdout, "two")
}

func TestUnit_ShellPlan_DeniedProgramInsideALineIsRefused(t *testing.T) {
	_, err := runLine(t, map[string]string{"_allowed_commands": "echo,ls", "_denied_commands": "ls"}, map[string]string{"command": "echo ok && ls"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied by policy")
}

func TestUnit_ShellPlan_PrecheckRefusesWithoutRunning(t *testing.T) {
	h := localtools.NewLocalExecToolsWith(localtools.NewTestHostRunner()).(*localtools.LocalExecTools)
	ctx := taskengine.WithToolsArgs(context.Background(), "local_shell", map[string]string{"_allowed_commands": "echo"})
	prechecker, ok := any(h).(taskengine.Prechecker)
	require.True(t, ok)

	require.Error(t, prechecker.Precheck(ctx, nil, &taskengine.ToolsCall{Name: "local_shell", Args: map[string]string{"command": "ls /"}}))
	require.NoError(t, prechecker.Precheck(ctx, nil, &taskengine.ToolsCall{Name: "local_shell", Args: map[string]string{"command": "cd /tmp && echo ok"}}))
}

// The shape that failed in a live session: a model that prefixes `cd`, under
// the allowlist the shipped chains carry. It must run, not be refused.
func TestUnit_ShellPlan_ChangeDirNeedsNoAllowlistEntry(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init", "-q").Run())

	policy := map[string]string{"_allowed_commands": "ls,cat,pwd,git,go,make"}
	res, err := runLine(t, policy, map[string]string{"command": "cd " + repo + " && git rev-parse --is-inside-work-tree"})
	require.NoError(t, err, "cd is interpreted, so it needs no entry of its own")
	assert.True(t, res.Success, res.Error)
	assert.Equal(t, "true", strings.TrimSpace(res.Stdout), "the second command ran in the directory cd named")
	assert.Len(t, res.Steps, 2)
}

func writeScript(t *testing.T, dir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "run.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho ran\n"), 0o755))
	return path
}
