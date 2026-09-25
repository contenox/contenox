package shellline_test

import (
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/services/shellline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnit_ShellLine_EscapeDecoding pins the printf/echo -e decoding a revealed
// payload goes through.
func TestUnit_ShellLine_EscapeDecoding(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`rm -rf /`:      `rm -rf /`,
		`\162\155`:      `rm`,
		`\0162\0155`:    `rm`,
		`\x72\x6d`:      `rm`,
		`a\nb`:          "a\nb",
		`a\tb`:          "a\tb",
		`back\\slash`:   `back\slash`,
		`\q`:            `\q`,
		`trailing\`:     `trailing\`,
		`\x`:            `\x`,
		`\400`:          "\040" + "0",
		`no escapes at`: `no escapes at`,
	} {
		assert.Equalf(t, want, shellline.DecodeEscapes(in), "decoding %q", in)
	}
}

// TestUnit_ShellLine_OnlyShellLinesAreRead pins which calls carry a line at
// all; an argv call's metacharacters are ordinary bytes.
func TestUnit_ShellLine_OnlyShellLinesAreRead(t *testing.T) {
	t.Parallel()

	read := []map[string]any{
		{"command": "git status && go build"},
		{"command": "git", "args": []any{"status"}, "shell": true},
		{"command": "git", "args": "status", "shell": "true"},
	}
	for _, args := range read {
		_, hasLine := shellline.LineFromArgs(args, shellline.KindPOSIX, "linux")
		assert.Truef(t, hasLine, "%v is a shell line", args)
	}

	notRead := []map[string]any{
		{"command": "git", "args": []any{"commit", "-m", "fix; rm -rf /"}},
		{"command": "git", "args": "status && go build"},
		{"command": ""},
		{},
		{"path": "/etc/passwd"},
	}
	for _, args := range notRead {
		_, hasLine := shellline.LineFromArgs(args, shellline.KindPOSIX, "linux")
		assert.Falsef(t, hasLine, "%v is an argv call and must not be read as shell syntax", args)
	}

	line, hasLine := shellline.LineFromArgs(map[string]any{"command": "git", "args": []any{"status", "--short"}, "shell": true}, shellline.KindPOSIX, "linux")
	require.True(t, hasLine)
	require.True(t, line.Parsed)
	require.Len(t, line.Steps, 1)
	assert.True(t, line.Steps[0].Literal)
	assert.Equal(t, []string{"git", "status", "--short"}, line.Steps[0].Words)

	_, hasLine = shellline.LineFromArgs(map[string]any{"command": strings.Repeat("x", shellline.MaxLineBytes+1)}, shellline.KindPOSIX, "linux")
	assert.False(t, hasLine)
}

// TestUnit_ShellLine_StepsCarryOperators pins the execution order and the
// operator joining each step to the one before it, which is what lets a caller
// run the line without a shell.
func TestUnit_ShellLine_StepsCarryOperators(t *testing.T) {
	t.Parallel()
	line := shellline.Parse(`git status && go build || echo failed; echo done`, shellline.KindPOSIX, "linux")
	require.True(t, line.Parsed)
	require.Len(t, line.Steps, 4)
	assert.Equal(t, shellline.OpStart, line.Steps[0].Operator)
	assert.Equal(t, shellline.OpAnd, line.Steps[1].Operator)
	assert.Equal(t, shellline.OpOr, line.Steps[2].Operator)
	assert.Equal(t, shellline.OpSeq, line.Steps[3].Operator)
	assert.Equal(t, "go", line.Steps[1].Base)
}

// TestUnit_ShellLine_ShapesAShellIsNeededFor pins what a reader can see and an
// argv-only executor cannot honour.
func TestUnit_ShellLine_ShapesAShellIsNeededFor(t *testing.T) {
	t.Parallel()
	for src, check := range map[string]func(*testing.T, shellline.Line){
		`ls | grep a`:          func(t *testing.T, l shellline.Line) { assert.Equal(t, 1, l.Pipelines) },
		`ls > out`:             func(t *testing.T, l shellline.Line) { assert.Len(t, l.Redirects, 1) },
		`cat <(ls)`:            func(t *testing.T, l shellline.Line) { assert.True(t, l.ProcessSubst) },
		`echo $(id)`:           func(t *testing.T, l shellline.Line) { assert.True(t, l.CommandSubst) },
		`echo $((1+1))`:        func(t *testing.T, l shellline.Line) { assert.True(t, l.ArithmExp) },
		`ls &`:                 func(t *testing.T, l shellline.Line) { assert.True(t, l.Background) },
		`! ls`:                 func(t *testing.T, l shellline.Line) { assert.True(t, l.Negated) },
		`if true; then ls; fi`: func(t *testing.T, l shellline.Line) { assert.True(t, l.Control) },
		`(ls)`:                 func(t *testing.T, l shellline.Line) { assert.True(t, l.Control) },
		`ls; rm -rf /`: func(t *testing.T, l shellline.Line) {
			require.Len(t, l.Steps, 2)
			assert.Equal(t, "rm", l.Steps[1].Base)
		},
	} {
		line := shellline.Parse(src, shellline.KindPOSIX, "linux")
		require.Truef(t, line.Parsed, "%q must parse", src)
		check(t, line)
	}
}

func TestUnit_ShellLine_KindGuardTable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		trusted shellline.Kind
		goos    string
		want    bool
	}{
		{shellline.KindPOSIX, "linux", true},
		{shellline.KindPOSIX, "windows", true},
		{"sh", "windows", true},
		{shellline.KindPowerShell, "windows", false},
		{shellline.KindCmd, "windows", false},
		{shellline.KindUnknown, "linux", false},
		{"", "linux", true},
		{"", "windows", false},
	} {
		assert.Equalf(t, tc.want, shellline.Structural(tc.trusted, tc.goos), "%q on %s", tc.trusted, tc.goos)
	}
}

func TestUnit_ShellLine_UnreadableLineIsNotParsed(t *testing.T) {
	t.Parallel()
	for _, src := range []string{`echo 'unterminated`, `ls &&`, `|| ls`} {
		line := shellline.Parse(src, shellline.KindPOSIX, "linux")
		assert.Truef(t, line.Readable, "%q: the kind is readable", src)
		assert.Falsef(t, line.Parsed, "%q must not parse", src)
	}
}
