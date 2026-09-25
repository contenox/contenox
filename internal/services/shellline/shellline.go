// Package shellline reads a command line into the commands it would run.
//
// It is the structural layer two policy surfaces share: the HITL gate that
// decides ask/allow/deny before a tool call runs, and the shell tool's own
// command policy that decides what may execute at all. Parsing lives here so
// both reason about the same commands; what to do about them stays with the
// caller.
package shellline

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync/atomic"

	"mvdan.cc/sh/v3/syntax"
)

// Kind names the shell that will interpret a command line.
type Kind string

const (
	// KindPOSIX is the only kind structural analysis runs on: `sh -c`.
	KindPOSIX Kind = "sh"
	// KindPowerShell is what the shell tool spawns on Windows; mvdan
	// cannot parse it, so it never reaches the parser.
	KindPowerShell Kind = "powershell"
	// KindCmd is cmd.exe — same treatment as powershell.
	KindCmd Kind = "cmd"
	// KindUnknown is any kind this package does not recognize; distinct
	// from "" so an unrecognized kind fails closed.
	KindUnknown Kind = "unknown"
)

type kindContextKey struct{}

// WithKind marks ctx with the shell that will interpret command lines
// evaluated under it, enabling structural analysis when the shell is POSIX.
func WithKind(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, kindContextKey{}, NormalizeKind(kind))
}

// KindFromContext returns the trusted shell kind set by WithKind, or "" when
// none was set.
func KindFromContext(ctx context.Context) Kind {
	kind, _ := ctx.Value(kindContextKey{}).(Kind)
	return kind
}

// NormalizeKind maps a shell name to the kind that parses it.
func NormalizeKind(s string) Kind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return ""
	case "sh", "bash", "dash", "ash", "posix":
		// bash and dash parse the same cleared node set as POSIX.
		return KindPOSIX
	case "powershell", "pwsh":
		return KindPowerShell
	case "cmd", "cmd.exe":
		return KindCmd
	default:
		return KindUnknown
	}
}

// Structural reports whether a command line of this kind can be read at all.
// A trusted kind is required: the kind a call claims about itself is a claim,
// not evidence, so an empty kind is only trusted where the host can produce
// nothing else.
func Structural(trusted Kind, goos string) bool {
	switch NormalizeKind(string(trusted)) {
	case KindPOSIX:
		return true
	case "":
	default:
		return false
	}
	return goos != "windows"
}

// Operator joins a step to the one before it.
type Operator string

const (
	// OpStart is the first step of a line.
	OpStart Operator = ""
	// OpAnd runs the step only when the step before it succeeded.
	OpAnd Operator = "&&"
	// OpOr runs the step only when the step before it failed.
	OpOr Operator = "||"
	// OpSeq runs the step whatever the step before it did.
	OpSeq Operator = ";"
	// OpPipe feeds the step's output to the step after it.
	OpPipe Operator = "|"
)

// Step is one simple command: a program and its literal arguments, or, when
// Unreadable, only what could be read of it.
type Step struct {
	Operator Operator
	// Name is the program as spelled, Base its basename, and Display the
	// command as a decision or a refusal should quote it.
	Name    string
	Base    string
	Display string
	// Words is the full argv, program included, only when Literal is true.
	Words   []string
	Literal bool
	// ArgCount counts Words as spelled, which is what a prefix match needs
	// when the words themselves are not knowable.
	ArgCount int
	// Assigns are the environment assignments prefixed to the call.
	Assigns []string
}

// Line is what one command line would do, in execution order.
type Line struct {
	// Readable is false when structural analysis does not apply to the shell
	// that would interpret this line, which is a refusal for any caller that
	// would otherwise allow something: an unreadable line is not a safe line.
	Readable bool
	// Parsed is false when the line could not be read at all, which is a
	// refusal for any caller that would otherwise allow something.
	Parsed bool
	// BashOnly marks a line only the wider second parse accepts: sh, the
	// executor, would reject the reading, so it can never ground an allow.
	BashOnly bool
	// Steps are the simple commands, top level and in order.
	Steps []Step
	// File is the parsed line, for a caller that reasons about nodes this
	// record does not describe (nesting, pipes, producers).
	File *syntax.File
	// Redirects are the redirects on top-level statements.
	Redirects []Redirect
	// CommandSubst, ProcessSubst and ArithmExp mark run-time values in the
	// line: `$(…)`, `<(…)` and `$((…))`.
	CommandSubst bool
	ProcessSubst bool
	ArithmExp    bool
	// Background, Negated, Coprocess and Control mark statements that are
	// not a plain sequence: `cmd &`, `! cmd`, `coproc`, and compound
	// constructs such as if/for/while/case/subshell/function.
	Background bool
	Negated    bool
	Coprocess  bool
	Control    bool
	// Pipelines counts steps joined by `|`, which no argv-only executor can
	// honour.
	Pipelines int
}

// MaxLineBytes bounds what is parsed; a longer line is not read at all.
const MaxLineBytes = 64 * 1024

// Redirect is one redirection, with its target when that target is literal.
type Redirect struct {
	Op      string
	Target  string
	Heredoc bool
}

var parses atomic.Int64

// Parses counts the lines parsed in this process, which is how a caller proves
// a line was read rather than guessed at.
func Parses() int64 { return parses.Load() }

// Parse reads src as a command line of the given kind. A line that cannot be
// read, or that this kind of shell would not interpret, comes back with
// Parsed false.
func Parse(src string, kind Kind, goos string) Line {
	var line Line
	if !Structural(kind, goos) {
		return line
	}
	line.Readable = true
	if len(src) > MaxLineBytes || strings.TrimSpace(src) == "" {
		return line
	}
	file, bashOnly, ok := ParseFile(src)
	if !ok {
		return line
	}
	line.Parsed = true
	line.BashOnly = bashOnly
	line.File = file
	collect(&line, file)
	return line
}

// ParseFile reads src with the POSIX grammar first and the bash grammar
// second. bashOnly reports a line only the second parse accepts.
func ParseFile(src string) (file *syntax.File, bashOnly bool, ok bool) {
	parses.Add(1)
	file, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(src), "")
	if err == nil && file != nil {
		return file, false, true
	}
	parses.Add(1)
	file, err = syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
	if err != nil || file == nil {
		return nil, false, false
	}
	return file, true, true
}

func collect(line *Line, file *syntax.File) {
	syntax.Walk(file, func(node syntax.Node) bool {
		switch node.(type) {
		case *syntax.CmdSubst:
			line.CommandSubst = true
		case *syntax.ProcSubst:
			line.ProcessSubst = true
		case *syntax.ArithmExp:
			line.ArithmExp = true
		}
		return true
	})
	for i, stmt := range file.Stmts {
		op := OpStart
		if i > 0 {
			op = OpSeq
		}
		walkStmt(line, stmt, op)
	}
}

func walkStmt(line *Line, stmt *syntax.Stmt, op Operator) {
	if stmt == nil {
		return
	}
	if stmt.Background {
		line.Background = true
	}
	if stmt.Negated {
		line.Negated = true
	}
	if stmt.Coprocess {
		line.Coprocess = true
	}
	for _, redir := range stmt.Redirs {
		line.Redirects = append(line.Redirects, RedirectView(redir))
	}
	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		step := StepFromCall(cmd)
		step.Operator = op
		line.Steps = append(line.Steps, step)
	case *syntax.BinaryCmd:
		switch cmd.Op {
		case syntax.AndStmt:
			walkStmt(line, cmd.X, op)
			walkStmt(line, cmd.Y, OpAnd)
		case syntax.OrStmt:
			walkStmt(line, cmd.X, op)
			walkStmt(line, cmd.Y, OpOr)
		case syntax.Pipe, syntax.PipeAll:
			line.Pipelines++
			walkStmt(line, cmd.X, op)
			walkStmt(line, cmd.Y, OpPipe)
		default:
			line.Control = true
		}
	default:
		line.Control = true
	}
}

// StepFromCall reads one simple command, resolving its words only when every
// word is knowable without running anything.
func StepFromCall(call *syntax.CallExpr) Step {
	var step Step
	for _, as := range call.Assigns {
		name := ""
		if as.Name != nil {
			name = as.Name.Value
		}
		step.Assigns = append(step.Assigns, name)
	}
	if len(call.Args) == 0 {
		return step
	}
	step.ArgCount = len(call.Args)
	if name, ok := WordName(call.Args[0]); ok && name != "" {
		step.Name = name
		step.Base = path.Base(name)
	}
	words := make([]string, 0, len(call.Args))
	literal := true
	for _, word := range call.Args {
		resolved, ok := LiteralWord(word)
		if !ok {
			literal = false
			break
		}
		words = append(words, resolved)
	}
	if literal {
		step.Words = words
		step.Literal = true
	}
	step.Display = displayOf(call)
	return step
}

// RedirectView reads one redirection, naming its target when that is literal.
func RedirectView(redir *syntax.Redirect) Redirect {
	out := Redirect{Op: redir.Op.String(), Heredoc: redir.Hdoc != nil}
	if redir.Word != nil {
		if target, ok := WordName(redir.Word); ok {
			out.Target = target
		} else {
			out.Target = UnknowableTarget
		}
	}
	return out
}

// UnknowableTarget stands in for a redirect target that only exists at run
// time, so a decision message can say so rather than showing an empty string.
const UnknowableTarget = "(not statically knowable)"

// LiteralAssignments collects the plain `NAME=value` assignments in a line,
// which is what lets a later `$NAME` resolve to a word.
func LiteralAssignments(file *syntax.File) map[string]string {
	out := map[string]string{}
	syntax.Walk(file, func(node syntax.Node) bool {
		as, ok := node.(*syntax.Assign)
		if !ok || as.Append || as.Naked || as.Name == nil || as.Index != nil || as.Value == nil {
			return true
		}
		if v, ok := WordName(as.Value); ok && v != "" {
			out[as.Name.Value] = v
		}
		return true
	})
	return out
}

// ResolveAssignedWords re-reads a call's words with the line's assignments
// substituted, so `CMD=x; $CMD -v` resolves to `x -v`. It reports false when
// nothing was resolved or the program itself stays unknowable.
func ResolveAssignedWords(call *syntax.CallExpr, assigns map[string]string) ([]string, bool) {
	if len(assigns) == 0 || len(call.Args) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(call.Args))
	resolved := false
	for _, word := range call.Args {
		if v, ok := WordName(word); ok {
			out = append(out, v)
			continue
		}
		if name, ok := SoleParamName(word); ok {
			if v, ok := assigns[name]; ok {
				out = append(out, v)
				resolved = true
				continue
			}
		}
		out = append(out, "")
	}
	if !resolved || out[0] == "" {
		return nil, false
	}
	return out, true
}

// SoleParamName reports a word that is exactly one parameter reference, which
// is the only shape an assignment can be substituted into.
func SoleParamName(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) != 1 {
		return "", false
	}
	pe, ok := word.Parts[0].(*syntax.ParamExp)
	if !ok || pe.Param == nil {
		return "", false
	}
	if pe.Excl || pe.Length || pe.Width || pe.IsSet ||
		pe.Index != nil || pe.Slice != nil || pe.Repl != nil || pe.Exp != nil ||
		len(pe.Modifiers) > 0 {
		return "", false
	}
	return pe.Param.Value, true
}

// LenientWords reads a call's words, using "" for one that only exists at run
// time, so a caller can still resolve the program and its shape.
func LenientWords(call *syntax.CallExpr) []string {
	out := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		v, ok := WordName(word)
		if !ok {
			v = ""
		}
		out = append(out, v)
	}
	return out
}

// WordsFromValues renders words that came from a decoded payload, which is how
// a revealed command is spelled back into a decision.
func WordsFromValues(words []string) Step {
	if len(words) == 0 {
		return Step{}
	}
	step := Step{
		Name:     words[0],
		Base:     path.Base(words[0]),
		ArgCount: len(words),
		Display:  FlattenDisplay(words),
	}
	for _, word := range words {
		if word == "" {
			return step
		}
	}
	step.Words = append([]string(nil), words...)
	step.Literal = true
	return step
}

const maxDisplayRunes = 60

// FlattenDisplay renders a command for a message, with control characters
// flattened so a decision message cannot forge a log line or terminal escape,
// and truncated so one command cannot fill the line.
func FlattenDisplay(parts []string) string {
	s := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, strings.Join(parts, " "))
	if r := []rune(s); len(r) > maxDisplayRunes {
		s = string(r[:maxDisplayRunes-1]) + "…"
	}
	return s
}

func displayOf(call *syntax.CallExpr) string {
	parts := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		if v, ok := WordName(word); ok {
			parts = append(parts, v)
			continue
		}
		parts = append(parts, "…")
	}
	return FlattenDisplay(parts)
}

// DisplayWords renders already-resolved words for a message.
func DisplayWords(words []string) string {
	parts := make([]string, 0, len(words))
	for _, word := range words {
		if word == "" {
			word = "…"
		}
		parts = append(parts, word)
	}
	return FlattenDisplay(parts)
}

const wordUnsafeMeta = "*?[]{}~\\$"

// LiteralWord resolves a word to its exact value when nothing about it depends
// on run time. Globs, expansions, substitutions, tilde and backslash escapes
// all make a word unknowable, because the value that reaches the program is
// then not the value in the line.
func LiteralWord(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}
	var sb strings.Builder
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if strings.ContainsAny(p.Value, wordUnsafeMeta) {
				return "", false
			}
			sb.WriteString(p.Value)
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar {
				return "", false
			}
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok || strings.Contains(lit.Value, "\\") {
					return "", false
				}
				sb.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return sb.String(), true
}

// WordName resolves a word the way a lenient reader must: quotes and escapes
// are peeled, so `r”m` names rm, while a word whose value only exists at run
// time has no name at all.
func WordName(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}
	var sb strings.Builder
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			sb.WriteString(UnescapeUnquoted(p.Value))
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return "", false
				}
				sb.WriteString(UnescapeDoubleQuoted(lit.Value))
			}
		default:
			return "", false
		}
	}
	return sb.String(), true
}

// UnescapeUnquoted peels backslash escapes outside quotes; a trailing
// backslash is a line continuation and disappears.
func UnescapeUnquoted(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			sb.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			break
		}
		i++
		if s[i] == '\n' {
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// UnescapeDoubleQuoted peels the escapes bash honors inside double quotes,
// leaving the rest as written.
func UnescapeDoubleQuoted(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			sb.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case '$', '`', '"', '\\':
			i++
			sb.WriteByte(s[i])
		case '\n':
			i++
		default:
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

// DecodeEscapes resolves the escapes printf and `echo -e` would expand, which
// is how a payload printed as text becomes the command it spells.
func DecodeEscapes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			sb.WriteByte(s[i])
			continue
		}
		i++
		switch c := s[i]; c {
		case '\\':
			sb.WriteByte('\\')
		case 'a':
			sb.WriteByte(0x07)
		case 'b':
			sb.WriteByte(0x08)
		case 'e':
			sb.WriteByte(0x1b)
		case 'f':
			sb.WriteByte(0x0c)
		case 'n':
			sb.WriteByte('\n')
		case 'r':
			sb.WriteByte('\r')
		case 't':
			sb.WriteByte('\t')
		case 'v':
			sb.WriteByte(0x0b)
		case 'x':
			if n, width, ok := ParseEscapedNumber(s[i+1:], 16, 2); ok {
				sb.WriteByte(n)
				i += width
				continue
			}
			sb.WriteString(`\x`)
		case '0', '1', '2', '3', '4', '5', '6', '7':
			start := i
			if c == '0' {
				start = i + 1
			}
			if n, width, ok := ParseEscapedNumber(s[start:], 8, 3); ok {
				sb.WriteByte(n)
				i = start + width - 1
				continue
			}
			sb.WriteByte(c)
		default:
			sb.WriteByte('\\')
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// ParseEscapedNumber reads at most maxDigits digits in base, refusing a value
// that does not fit a byte.
func ParseEscapedNumber(s string, base, maxDigits int) (value byte, width int, ok bool) {
	n := 0
	for width < len(s) && width < maxDigits {
		d := DigitValue(s[width])
		if d < 0 || d >= base {
			break
		}
		next := n*base + d
		if next > 0xff {
			break
		}
		n = next
		width++
	}
	if width == 0 {
		return 0, 0, false
	}
	return byte(n), width, true
}

// DigitValue is the value of one ASCII digit, or -1 when it is not one.
func DigitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return -1
	}
}

// ModeRequested reports whether a call asked for shell interpretation, which
// is what makes its command a line rather than a single argv.
func ModeRequested(args map[string]any) bool {
	switch v := args["shell"].(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "y", "on":
			return true
		}
	}
	return false
}

// ArgTokens reads a tool call's args field, which arrives as a list or as one
// already-split string.
func ArgTokens(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return strings.Fields(t)
	case []string:
		return append([]string(nil), t...)
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

// LineFromArgs reads the command line a tool call carries: the command, plus
// its args when the call asked for shell interpretation, and nothing when the
// call keeps them apart (that shape is a single argv, not a line).
func LineFromArgs(args map[string]any, kind Kind, goos string) (Line, bool) {
	var line Line
	command, _ := args["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return line, false
	}
	extra := ArgTokens(args["args"])
	src := command
	if ModeRequested(args) {
		if len(extra) > 0 {
			src += " " + strings.Join(extra, " ")
		}
	} else if len(extra) > 0 {
		return line, false
	}
	if len(src) > MaxLineBytes {
		return line, false
	}
	return Parse(src, kind, goos), true
}
