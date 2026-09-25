package hitlservice

import (
	"context"
	"path"
	"runtime"
	"strings"

	"github.com/contenox/contenox/internal/services/shellline"
	"mvdan.cc/sh/v3/syntax"
)

// ShellKind names the shell that will interpret a gated command line. It is
// [shellline.Kind], the same value the tool layer reads a command line with, so
// the gate and the tool cannot disagree about what a line says.
type ShellKind = shellline.Kind

const (
	// ShellKindPOSIX is the only kind structural analysis runs on: `sh -c`.
	ShellKindPOSIX = shellline.KindPOSIX
	// ShellKindPowerShell is what local_shell spawns on Windows; mvdan
	// cannot parse it, so it never reaches the parser.
	ShellKindPowerShell = shellline.KindPowerShell
	// ShellKindCmd is cmd.exe — same treatment as powershell.
	ShellKindCmd = shellline.KindCmd
	// ShellKindUnknown is any kind this package does not recognize; distinct
	// from "" so an unrecognized kind fails closed.
	ShellKindUnknown = shellline.KindUnknown
)

// WithShellKind marks ctx with the shell that will interpret command lines
// evaluated under it, enabling structural analysis when the shell is POSIX.
func WithShellKind(ctx context.Context, kind string) context.Context {
	return shellline.WithKind(ctx, kind)
}

// ShellKindFromContext returns the trusted shell kind set by WithShellKind,
// or "" when none was set.
func ShellKindFromContext(ctx context.Context) ShellKind {
	return shellline.KindFromContext(ctx)
}

var clearedAssignmentNames = map[string]bool{}

var unclearedCommandNames = map[string]bool{
	// re-entry
	"eval": true, "exec": true, "source": true, ".": true,
	"sh": true, "bash": true, "dash": true, "ash": true, "ksh": true, "zsh": true, "fish": true,
	"env": true, "command": true, "builtin": true, "xargs": true,
	"nohup": true, "setsid": true, "timeout": true, "watch": true, "time": true,
	"sudo": true, "doas": true, "su": true, "find": true,
	// environment mutation
	"export": true, "set": true, "unset": true, "readonly": true, "local": true,
	"alias": true, "unalias": true, "declare": true, "typeset": true, "trap": true,
	"shift": true, "getopts": true, "ulimit": true, "umask": true,
}

type shellReading struct {
	analyzed     bool
	parsed       bool
	commands     []shellCommandView
	redirects    []shellRedirect
	hasCmdSubst  bool
	hasProcSubst bool
	hasArithmExp bool
	upgradable   bool
}

// shellCommandView is one simple command as read from the line, named leniently
// so a command spelled through quotes or escapes still resolves.
type shellCommandView = shellline.Step

type shellRedirect struct {
	op      string
	target  string
	heredoc bool
}

func analyzeShellArgs(trusted ShellKind, args map[string]any) shellReading {
	var r shellReading
	if len(args) == 0 {
		return r
	}
	line, hasLine := shellline.LineFromArgs(args, trusted, runtime.GOOS)
	if !hasLine || !line.Readable {
		return r
	}
	r.analyzed = true
	if !line.Parsed {
		return r
	}
	r.parsed = true
	collectShellNodes(line.File, &r, 0)
	// bashOnly forces upgradable off: sh -c is the executor, so a reading sh itself would reject cannot ground an allow.
	r.upgradable = !line.BashOnly && fileClearedForUpgrade(line.File)
	return r
}

func collectShellNodes(f *syntax.File, r *shellReading, depth int) {
	assigns := shellline.LiteralAssignments(f)
	syntax.Walk(f, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Stmt:
			for _, rd := range n.Redirs {
				r.redirects = append(r.redirects, redirectView(rd))
			}
			switch cmd := n.Cmd.(type) {
			case *syntax.CallExpr:
				if len(cmd.Args) > 0 {
					r.commands = append(r.commands, shellline.StepFromCall(cmd))
				}
				peelWrapper(shellline.LenientWords(cmd), r, depth)
				if words, ok := shellline.ResolveAssignedWords(cmd, assigns); ok {
					revealWords(words, r, depth)
				}
			case *syntax.BinaryCmd:
				if cmd.Op == syntax.Pipe {
					revealPipedPayload(cmd, r, depth)
				}
			}
		case *syntax.CmdSubst:
			r.hasCmdSubst = true
		case *syntax.ProcSubst:
			r.hasProcSubst = true
		case *syntax.ArithmExp:
			r.hasArithmExp = true
		}
		return true
	})
}

const (
	maxRevealDepth      = 4
	maxRevealedCommands = 256
)

var nestedShellNames = map[string]bool{
	"sh": true, "bash": true, "dash": true, "ash": true,
	"ksh": true, "zsh": true, "fish": true,
}

func revealWords(words []string, r *shellReading, depth int) {
	if depth > maxRevealDepth || len(r.commands) >= maxRevealedCommands {
		return
	}
	if len(words) == 0 || words[0] == "" {
		return
	}
	r.commands = append(r.commands, shellline.WordsFromValues(words))
	peelWrapper(words, r, depth)
}

func peelWrapper(words []string, r *shellReading, depth int) {
	if depth >= maxRevealDepth || len(words) == 0 || words[0] == "" {
		return
	}
	base := path.Base(words[0])
	switch {
	case nestedShellNames[base]:
		if payload, ok := dashCPayload(words); ok {
			revealSource(payload, r, depth+1)
		}
	case base == "xargs":
		if rest, ok := xargsCommandWords(words); ok {
			revealWords(rest, r, depth+1)
		}
	case base == "eval":
		if src, ok := joinKnown(words[1:]); ok {
			revealSource(src, r, depth+1)
		}
	}
}

func revealSource(src string, r *shellReading, depth int) {
	if depth > maxRevealDepth || len(r.commands) >= maxRevealedCommands {
		return
	}
	src = strings.TrimSpace(src)
	if src == "" || len(src) > shellline.MaxLineBytes {
		return
	}
	f, _, ok := shellline.ParseFile(src)
	if !ok {
		return
	}
	collectShellNodes(f, r, depth)
}

func dashCPayload(words []string) (string, bool) {
	for i := 1; i < len(words); i++ {
		w := words[i]
		if w == "" || w == "-" || w == "--" || !strings.HasPrefix(w, "-") {
			return "", false
		}
		if strings.ContainsRune(w[1:], 'c') {
			if i+1 < len(words) && words[i+1] != "" {
				return words[i+1], true
			}
			return "", false
		}
	}
	return "", false
}

var xargsValueOptions = map[string]bool{
	"-a": true, "-d": true, "-E": true, "-I": true, "-L": true,
	"-n": true, "-P": true, "-s": true,
	"--arg-file": true, "--delimiter": true, "--eof": true, "--replace": true,
	"--max-lines": true, "--max-args": true, "--max-procs": true,
	"--max-chars": true, "--process-slot-var": true,
}

func xargsCommandWords(words []string) ([]string, bool) {
	for i := 1; i < len(words); i++ {
		w := words[i]
		if w == "" {
			return nil, false
		}
		if !strings.HasPrefix(w, "-") || w == "-" {
			return words[i:], true
		}
		if w == "--" {
			if i+1 < len(words) {
				return words[i+1:], true
			}
			return nil, false
		}
		if xargsValueOptions[w] {
			i++
		}
	}
	return nil, false
}

func revealPipedPayload(pipe *syntax.BinaryCmd, r *shellReading, depth int) {
	if depth >= maxRevealDepth {
		return
	}
	producer, ok := stmtCallWords(pipe.X)
	if !ok || !consumerEvaluatesStdin(pipe.Y) {
		return
	}
	for _, src := range printedPayloads(producer) {
		revealSource(src, r, depth+1)
	}
}

func stmtCallWords(st *syntax.Stmt) ([]string, bool) {
	if st == nil {
		return nil, false
	}
	call, ok := st.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return nil, false
	}
	return shellline.LenientWords(call), true
}

func consumerEvaluatesStdin(st *syntax.Stmt) bool {
	words, ok := stmtCallWords(st)
	if !ok || words[0] == "" {
		return false
	}
	base := path.Base(words[0])
	return nestedShellNames[base] || base == "eval" || base == "source" || base == "."
}

func printedPayloads(words []string) []string {
	base := path.Base(words[0])
	if base != "echo" && base != "printf" {
		return nil
	}
	rest := words[1:]
	if base == "echo" {
		for len(rest) > 0 && isEchoFlag(rest[0]) {
			rest = rest[1:]
		}
	}
	if len(rest) == 0 {
		return nil
	}
	raw, ok := joinKnown(rest)
	if !ok {
		return nil
	}
	out := []string{raw}
	if decoded := shellline.DecodeEscapes(raw); decoded != raw {
		out = append(out, decoded)
	}
	return out
}

func isEchoFlag(w string) bool {
	switch w {
	case "-n", "-e", "-E", "-ne", "-en", "-nE", "-En":
		return true
	}
	return false
}

func joinKnown(words []string) (string, bool) {
	if len(words) == 0 {
		return "", false
	}
	for _, w := range words {
		if w == "" {
			return "", false
		}
	}
	return strings.Join(words, " "), true
}

func redirectView(rd *syntax.Redirect) shellRedirect {
	out := shellRedirect{op: rd.Op.String(), heredoc: rd.Hdoc != nil}
	if rd.Word != nil {
		if target, ok := shellline.WordName(rd.Word); ok {
			out.target = target
		} else {
			out.target = shellline.UnknowableTarget
		}
	}
	return out
}

func fileClearedForUpgrade(f *syntax.File) bool {
	if f == nil || len(f.Stmts) != 1 {
		return false
	}
	return stmtClearedForUpgrade(f.Stmts[0])
}

func stmtClearedForUpgrade(st *syntax.Stmt) bool {
	if st == nil || st.Cmd == nil {
		return false
	}
	// `cmd &` detaches from the gating approval; `! cmd` inverts && status; |&/&| are shell-specific.
	if st.Negated || st.Background || st.Coprocess || st.Disown {
		return false
	}
	// A redirect is not an argument: an allowlisted reader with one is a writer.
	if len(st.Redirs) > 0 {
		return false
	}
	switch cmd := st.Cmd.(type) {
	case *syntax.CallExpr:
		return callClearedForUpgrade(cmd)
	case *syntax.BinaryCmd:
		switch cmd.Op {
		case syntax.AndStmt, syntax.OrStmt, syntax.Pipe:
			// Flow-insensitive on purpose: `a || b` prices b even though it may never run.
			return stmtClearedForUpgrade(cmd.X) && stmtClearedForUpgrade(cmd.Y)
		}
		return false
	default:
		return false
	}
}

func callClearedForUpgrade(call *syntax.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	// Assignment prefixes are a hijack channel, not decoration.
	for _, as := range call.Assigns {
		if as.Name == nil || !clearedAssignmentNames[as.Name.Value] {
			return false
		}
	}
	for _, w := range call.Args {
		if _, ok := shellline.LiteralWord(w); !ok {
			return false
		}
	}
	name, ok := shellline.LiteralWord(call.Args[0])
	if !ok || name == "" {
		return false
	}
	return !unclearedCommandNames[path.Base(name)]
}

func structuralCommandInList(r shellReading, list string) (shellCommandView, bool) {
	if !r.parsed || strings.TrimSpace(list) == "" {
		return shellCommandView{}, false
	}
	for _, cmd := range r.commands {
		if cmd.Base == "" {
			continue
		}
		for _, name := range strings.Split(list, ",") {
			if name = strings.TrimSpace(name); name != "" && cmd.Base == name {
				return cmd, true
			}
		}
	}
	return shellCommandView{}, false
}

func structuralCommandSubstitution(r shellReading) bool {
	return r.parsed && (r.hasCmdSubst || r.hasProcSubst || r.hasArithmExp)
}

func structuralPrefixAllowed(r shellReading, prefixList string) bool {
	if strings.TrimSpace(prefixList) == "" {
		return false
	}
	if !r.analyzed || !r.parsed || !r.upgradable || len(r.commands) < 2 {
		return false
	}
	for _, cmd := range r.commands {
		if !cmd.Literal || len(cmd.Words) == 0 || len(cmd.Assigns) > 0 {
			return false
		}
		if unclearedCommandNames[cmd.Base] {
			return false
		}
		if !prefixListMatchesTokens(tokensOf(cmd), prefixList) {
			return false
		}
	}
	return true
}

func tokensOf(cmd shellCommandView) []string {
	var words []string
	if cmd.Literal && len(cmd.Words) > 0 {
		words = append([]string(nil), cmd.Words...)
	} else if cmd.Name != "" {
		// An unknowable argument still has a knowable name; later slots get a sentinel no prefix word can equal.
		words = []string{cmd.Name}
		for i := 1; i < cmd.ArgCount; i++ {
			words = append(words, unknowableWord)
		}
	} else {
		return nil
	}
	// Mirrors allowlistProgramWord: a pathed word keeps its path, so structure never grants what the tokenizer refused.
	words[0] = allowlistProgramWord(words[0])
	return words
}

const unknowableWord = "\x00 not statically knowable"

func structuralContradictsPrefix(r shellReading, prefixList string) bool {
	if !r.parsed || strings.TrimSpace(prefixList) == "" || len(r.commands) == 0 {
		return false
	}
	for _, cmd := range r.commands {
		tokens := tokensOf(cmd)
		if len(tokens) == 0 {
			// No statically knowable name: the tokenizer couldn't read it either.
			continue
		}
		if !prefixListMatchesTokens(tokens, prefixList) {
			return true
		}
	}
	return false
}
