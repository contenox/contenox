package localtools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/contenox/runtime/runtime/vfs"
	"github.com/getkin/kin-openapi/openapi3"
)

const LocalFSToolsName = "local_fs"

// readBeforeWriteDenial is the LLM-facing message returned when the model tries
// to mutate an existing file it has not read in this session. The model treats
// it as a normal tool result and is expected to call read_file then retry. It
// carries the Rec 5 recoverable-by-correction severity marker like every other
// correctable local-tools condition.
const readBeforeWriteDenial = "local_fs: cannot modify existing file %s without reading it first. Call local_fs.read_file(%q) to confirm the current contents, then retry. " + severityRecoverable

// fileUnchangedStub is the tool-result text returned when the model re-reads a
// file whose content hash is already recorded in this session. The earlier
// read_file result is still in the conversation context, so re-sending the full
// content wastes tokens without providing new information.
const fileUnchangedStub = "File unchanged since last read — the content from your earlier read_file call in this conversation is still current."

const readBeforeWriteFullReadDenial = "local_fs: cannot overwrite existing file %s after only reading a line range. Call local_fs.read_file(%q) to read the full current contents, then retry. " + severityRecoverable

const readBeforeWriteStaleReadDenial = "local_fs: cannot modify existing file %s because it changed since you read it. Call local_fs.read_file(%q) to refresh the current contents, then retry. " + severityRecoverable

type readRequirement int

const (
	// requireAnyFileRead is enough for targeted mutators such as sed.
	requireAnyFileRead readRequirement = iota
	// requireFullFileRead is required for full-file overwrite via write_file.
	requireFullFileRead
)

// LocalFSTools provides direct filesystem access tools.
//
// The tool tracks its own per-session read history in the local_fs_reads table
// so that write_file / sed against an existing file can be blocked unless that
// file has been read first this session. State ownership lives entirely with
// this tool — the engine never sees the rule.
type LocalFSTools struct {
	allowedDir  string
	db          libdb.DBManager
	fileIO      FileIO
	name        string
	cwdResolver func(context.Context) string
}

// NewLocalFSTools creates a new instance of LocalFSTools. db may be nil; when
// nil, the read-before-write guard degrades to a no-op (used by tests and
// callers without a DB).
func NewLocalFSTools(allowedDir string, db libdb.DBManager) taskengine.ToolsRepo {
	return NewLocalFSToolsWith(allowedDir, db, nil, LocalFSToolsName, nil)
}

func NewLocalFSToolsWith(allowedDir string, db libdb.DBManager, io FileIO, name string, cwdResolver func(context.Context) string) taskengine.ToolsRepo {
	if io == nil {
		io = osFileIO{}
	}
	if name == "" {
		name = LocalFSToolsName
	}
	cleaned := allowedDir
	if cleaned != "" {
		cleaned = filepath.Clean(cleaned)
	}
	return &LocalFSTools{
		allowedDir:  cleaned,
		db:          db,
		fileIO:      io,
		name:        name,
		cwdResolver: cwdResolver,
	}
}

// Exec handles filesystem tool execution. It is a thin wrapper over execDispatch
// that stamps every returned error with a fatal-vs-recoverable severity marker
// (Rec 5, tool-hardening.md): callers can key on "(recoverable: ...)" vs
// "(fatal: ...)" to decide whether a corrected retry is worthwhile. Soft
// tool-result strings (read-before-write denials, sed no-match suggestions) carry
// their own recoverable marker inline and flow through the nil-error path
// untouched here.
func (h *LocalFSTools) Exec(ctx context.Context, startTime time.Time, input any, debug bool, toolsCall *taskengine.ToolsCall) (any, taskengine.DataType, error) {
	res, dt, err := h.execDispatch(ctx, startTime, input, debug, toolsCall)
	return res, dt, markSeverity(err)
}

func (h *LocalFSTools) execDispatch(ctx context.Context, startTime time.Time, input any, debug bool, toolsCall *taskengine.ToolsCall) (any, taskengine.DataType, error) {
	if toolsCall == nil {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: tools required")
	}

	args, ok := input.(map[string]any)
	if !ok {
		// Declarative `tools` tasks carry their arguments on the ToolsCall
		// (like local_shell); fall back to them when the chain input isn't
		// an args map (e.g. chat history flowing through a gated tool task).
		if len(toolsCall.Args) > 0 {
			args = make(map[string]any, len(toolsCall.Args))
			for k, v := range toolsCall.Args {
				args[k] = v
			}
		} else {
			return nil, taskengine.DataTypeAny, errors.New("local_fs: input must be a map (or provide tools.args)")
		}
	}

	toolName := toolsCall.ToolName
	if toolName == "" {
		toolName = toolsCall.Name
	}

	switch toolName {
	case "read_file":
		if err := rejectUnknownArgs("local_fs.read_file", args, "path", "start_line", "end_line"); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
		return h.readFile(ctx, args)
	case "write_file":
		if err := rejectUnknownArgs("local_fs.write_file", args, "path", "content"); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
		return h.writeFile(ctx, args)
	case "list_dir":
		if err := rejectUnknownArgs("local_fs.list_dir", args, "path", "recursive", "max_depth"); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
		return h.listDir(ctx, args)
	case "grep":
		if err := rejectUnknownArgs("local_fs.grep", args, "path", "pattern", "regex", "start_line", "end_line"); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
		return h.grep(ctx, args)
	case "find_files":
		if err := rejectUnknownArgs("local_fs.find_files", args, "pattern", "path"); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
		return h.findFiles(ctx, args)
	case "sed":
		if err := rejectUnknownArgs("local_fs.sed", args, "path", "pattern", "replacement"); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
		return h.sed(ctx, args)
	case "count_stats":
		if err := rejectUnknownArgs("local_fs.count_stats", args, "path"); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
		return h.countStats(ctx, args)
	case "read_file_range":
		if err := rejectUnknownArgs("local_fs.read_file_range", args, "path", "start_line", "end_line"); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
		return h.readFileRange(ctx, args)
	case "stat_file":
		if err := rejectUnknownArgs("local_fs.stat_file", args, "path"); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
		return h.statFile(ctx, args)
	default:
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: unknown tool %s", toolName)
	}
}

func (h *LocalFSTools) baseDir(ctx context.Context) (string, error) {
	if args := taskengine.ToolsArgsFromContext(ctx, h.name); len(args) > 0 {
		if policyDir := strings.TrimSpace(args["_allowed_dir"]); policyDir != "" {
			cleaned := filepath.Clean(policyDir)
			if filepath.IsAbs(cleaned) {
				return cleaned, nil
			}
			if h.cwdResolver != nil {
				if cwd := h.cwdResolver(ctx); cwd != "" {
					return filepath.Clean(filepath.Join(cwd, cleaned)), nil
				}
			}
			return cleaned, nil
		}
	}
	base := h.allowedDir
	if base == "" && h.cwdResolver != nil {
		if r := h.cwdResolver(ctx); r != "" {
			base = filepath.Clean(r)
		}
	}
	if base == "" {
		return "", errors.New("local_fs: no allowed directory configured")
	}
	return base, nil
}

// absAllowedDir returns the symlink-resolved base directory for the current
// call context. Base resolution lives in the vfs package (the single home for
// workspace-root handling).
func (h *LocalFSTools) absAllowedDir(ctx context.Context) (string, error) {
	base, err := h.baseDir(ctx)
	if err != nil {
		return "", err
	}
	resolved, err := vfs.ResolveRoot(base)
	if err != nil {
		return "", fmt.Errorf("local_fs: invalid allowed dir: %w", err)
	}
	return resolved, nil
}

// checkPath verifies if a path is within the allowed directory. Containment —
// path normalization plus symlink-escape guarding — is delegated to the vfs
// package so there is a single implementation shared with the /files browse
// API. A symlink inside the sandbox pointing outside it (e.g. ln -s /etc
// /allowed/link) is caught before any I/O is performed.
func (h *LocalFSTools) checkPath(ctx context.Context, path string) (string, error) {
	base, err := h.baseDir(ctx)
	if err != nil {
		return "", err
	}
	resolved, err := vfs.Contain(base, path)
	if err != nil {
		if errors.Is(err, vfs.ErrEscape) {
			return "", fmt.Errorf("local_fs: path %s escapes allowed directory %s", path, base)
		}
		return "", fmt.Errorf("local_fs: %w", err)
	}
	return resolved, nil
}

// argBool returns the boolean value for key when present and typed as bool (JSON booleans).
func argBool(args map[string]any, key string) (v bool, ok bool) {
	x, exists := args[key]
	if !exists {
		return false, false
	}
	b, ok := x.(bool)
	return b, ok
}

// argFloat returns a float64 for key when JSON numbers decode as float64.
func argFloat(args map[string]any, key string) (v float64, ok bool) {
	x, exists := args[key]
	if !exists {
		return 0, false
	}
	switch n := x.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// maxListDepthFromPolicy caps recursion depth for list_dir(recursive). tools_policies.local_fs: _max_list_depth — default 6.
func (h *LocalFSTools) maxListDepthFromPolicy(ctx context.Context) int {
	const defaultDepth = 6
	args := taskengine.ToolsArgsFromContext(ctx, h.name)
	if args == nil {
		return defaultDepth
	}
	s := strings.TrimSpace(args["_max_list_depth"])
	if s == "" {
		return defaultDepth
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return defaultDepth
	}
	if n > 32 {
		return 32
	}
	return n
}

// defaultSkipDirNames is the set of directory basenames that list_dir omits by
// default. These directories are typically large, machine-generated, or
// version-control internals that add noise to the model's context without
// contributing useful source information.
var defaultSkipDirNames = []string{
	".git", "node_modules", ".venv", "__pycache__",
	".next", "dist", ".cache", "vendor", "target",
	".idea", ".vscode",
}

// skipDirNamesFromPolicy returns the set of directory basenames that list_dir
// should silently omit from output and recursion.
// Policy key (tools_policies.local_fs): _skip_dir_names — comma-separated
// basenames. When the key is absent the default noise set is used. Set the
// key to "" (empty string) to disable filtering entirely and show every
// directory.
func (h *LocalFSTools) skipDirNamesFromPolicy(ctx context.Context) map[string]bool {
	args := taskengine.ToolsArgsFromContext(ctx, h.name)
	raw, keyPresent := "", false
	if args != nil {
		raw, keyPresent = args["_skip_dir_names"]
	}
	if !keyPresent {
		return skipDirNameSet(defaultSkipDirNames)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil // disabled: show everything
	}
	var names []string
	for _, s := range strings.Split(raw, ",") {
		if n := strings.TrimSpace(s); n != "" {
			names = append(names, n)
		}
	}
	return skipDirNameSet(names)
}

func skipDirNameSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// listExtensionsFromPolicy returns the set of lower-cased file extensions that
// list_dir will include. When absent or empty, all files are returned.
// Policy key (tools_policies.local_fs): _list_extensions — comma-separated
// extensions, e.g. ".go,.md,.json". A leading dot is optional.
func (h *LocalFSTools) listExtensionsFromPolicy(ctx context.Context) map[string]bool {
	args := taskengine.ToolsArgsFromContext(ctx, h.name)
	if args == nil {
		return nil
	}
	raw := strings.TrimSpace(args["_list_extensions"])
	if raw == "" {
		return nil
	}
	m := make(map[string]bool)
	for _, s := range strings.Split(raw, ",") {
		ext := strings.ToLower(strings.TrimSpace(s))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		m[ext] = true
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// maxGrepMatchesFromPolicy stops grep after this many lines (error: narrow pattern/range). tools_policies.local_fs: _max_grep_matches — default 5000.
func (h *LocalFSTools) maxGrepMatchesFromPolicy(ctx context.Context) int {
	const defaultMax = 5000
	args := taskengine.ToolsArgsFromContext(ctx, h.name)
	if args == nil {
		return defaultMax
	}
	s := strings.TrimSpace(args["_max_grep_matches"])
	if s == "" {
		return defaultMax
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return defaultMax
	}
	if n > 500000 {
		return 500000
	}
	return n
}

// grepLineRange returns 1-based inclusive [start, end] line numbers to search within numLines total lines.
func grepLineRange(args map[string]any, numLines int) (start, end int) {
	start = 1
	end = numLines
	if v, ok := argFloat(args, "start_line"); ok {
		s := int(v)
		if s < 1 {
			s = 1
		}
		start = s
	}
	if v, ok := argFloat(args, "end_line"); ok {
		e := int(v)
		if e < start {
			e = start
		}
		end = e
	}
	if end > numLines {
		end = numLines
	}
	if start > numLines {
		start = numLines + 1
	}
	if end < start {
		end = start - 1
	}
	return start, end
}

// maxOutputBytesFromPolicy caps the byte size of any tool result returned to the model (UTF-8 bytes).
// If the serialized output exceeds this, the tools returns an error so the model can narrow the query.
// Chain policy keys (tools_policies.local_fs): _max_output_bytes — default 524288 (512 KiB) when unset.
// Non-positive means unlimited.
func (h *LocalFSTools) maxOutputBytesFromPolicy(ctx context.Context) (limit int64, unlimited bool) {
	args := taskengine.ToolsArgsFromContext(ctx, h.name)
	if args == nil {
		return 512 * 1024, false
	}
	s := strings.TrimSpace(args["_max_output_bytes"])
	if s == "" {
		return 512 * 1024, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 512 * 1024, false
	}
	if n <= 0 {
		return 0, true
	}
	return n, false
}

func (h *LocalFSTools) checkToolOutputLimit(ctx context.Context, tool string, payload string) error {
	limit, unlimited := h.maxOutputBytesFromPolicy(ctx)
	if unlimited {
		return nil
	}
	if int64(len(payload)) > limit {
		return fmt.Errorf(
			"local_fs: %s output is %d bytes (max %d); narrow the path or pattern, use read_file_range, or set _max_output_bytes in tools_policies.local_fs",
			tool, len(payload), limit,
		)
	}
	return nil
}

// maxReadBytesFromPolicy returns the max bytes for a full-file read. Non-positive means unlimited.
// Chain policy keys (tools_policies.local_fs): _max_read_bytes — default 1048576 (1 MiB) when unset.
func (h *LocalFSTools) maxReadBytesFromPolicy(ctx context.Context) (limit int64, unlimited bool) {
	args := taskengine.ToolsArgsFromContext(ctx, h.name)
	if args == nil {
		return 1024 * 1024, false
	}
	s := strings.TrimSpace(args["_max_read_bytes"])
	if s == "" {
		return 1024 * 1024, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 1024 * 1024, false
	}
	if n <= 0 {
		return 0, true
	}
	return n, false
}

func (h *LocalFSTools) checkDeniedSubstrings(ctx context.Context, absPath string) error {
	base, err := h.absAllowedDir(ctx)
	if err != nil {
		return fmt.Errorf("local_fs: allowed dir: %w", err)
	}
	rel, err := filepath.Rel(base, absPath)
	if err != nil {
		return fmt.Errorf("local_fs: rel path: %w", err)
	}
	rel = filepath.ToSlash(rel)
	args := taskengine.ToolsArgsFromContext(ctx, h.name)
	if args == nil {
		return nil
	}
	raw := strings.TrimSpace(args["_denied_path_substrings"])
	if raw == "" {
		return nil
	}
	for _, pat := range strings.Split(raw, ",") {
		p := strings.TrimSpace(pat)
		if p == "" {
			continue
		}
		p = filepath.ToSlash(p)
		if strings.Contains(rel, p) {
			return fmt.Errorf("local_fs: path %q matches denied substring %q (tools_policies.local_fs._denied_path_substrings)", rel, p)
		}
	}
	return nil
}

func (h *LocalFSTools) checkFileSizeLimit(ctx context.Context, absPath string) error {
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("local_fs: stat: %w", err)
	}
	if info.IsDir() {
		return nil
	}
	limit, unlimited := h.maxReadBytesFromPolicy(ctx)
	if unlimited {
		return nil
	}
	if info.Size() > limit {
		return fmt.Errorf("local_fs: file is %d bytes (max %d); use read_file_range or set _max_read_bytes in tools_policies.local_fs", info.Size(), limit)
	}
	return nil
}

func (h *LocalFSTools) precheckFullRead(ctx context.Context, absPath string) error {
	if err := h.checkDeniedSubstrings(ctx, absPath); err != nil {
		return err
	}
	return h.checkFileSizeLimit(ctx, absPath)
}

func (h *LocalFSTools) readFile(ctx context.Context, args map[string]any) (any, taskengine.DataType, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: path required for read_file")
	}

	absPath, err := h.checkPath(ctx, path)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	if err := h.checkDeniedSubstrings(ctx, absPath); err != nil {
		return nil, taskengine.DataTypeAny, err
	}

	info, statErr := os.Stat(absPath)
	if statErr != nil {
		// Rec 7: did-you-mean over sibling names on a missing path.
		if os.IsNotExist(statErr) {
			return nil, taskengine.DataTypeAny, h.notFound("read_file", path, absPath)
		}
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: read_file: %w", statErr)
	}
	if info.IsDir() {
		return nil, taskengine.DataTypeAny, recoverablef(
			"local_fs: read_file: %s is a directory (%s); use list_dir", path, describePathForError(absPath, info))
	}

	// Rec 4: read_file gained optional start_line/end_line, so a truncated read can
	// name "use start_line: N" actionable on the SAME tool. When either is present
	// this is a ranged (paging) read, delegated to the shared range reader.
	if _, hasStart := argFloat(args, "start_line"); hasStart {
		return h.readFileRange(ctx, args)
	}
	if _, hasEnd := argFloat(args, "end_line"); hasEnd {
		return h.readFileRange(ctx, args)
	}

	outLimit, unlimitedOut := h.maxOutputBytesFromPolicy(ctx)
	readLimit, unlimitedRead := h.maxReadBytesFromPolicy(ctx)

	// A file larger than the read cap cannot be loaded whole. Rec 4: don't dump a
	// partial; name the exact next step. Reading a portion via start_line/end_line
	// streams without the size cap, or raise _max_read_bytes to load it all.
	if !unlimitedRead && info.Size() > readLimit {
		return nil, taskengine.DataTypeAny, recoverablef(
			"local_fs: read_file: %s is %s (%d bytes), over the %d-byte read cap; read a portion with read_file start_line/end_line (e.g. start_line: 1), or raise _max_read_bytes in tools_policies.local_fs to load the whole file",
			path, humanSize(info.Size()), info.Size(), readLimit)
	}

	content, err := h.fileIO.ReadFile(ctx, absPath)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read file: %w", err)
	}

	// Refuse to dump binary content into the model's transcript: raw bytes from a
	// compiled binary, image, or archive burn tokens on content the model cannot
	// use as text. This only inspects the bytes already loaded (no extra I/O) and
	// shares its heuristic with stat_file's "binary" field.
	if isBinarySample(sniffPrefix(content)) {
		detail := fmt.Sprintf("binary file (%s)", fileSizeAndExecFlag(info))
		return nil, taskengine.DataTypeAny, recoverablef(
			"local_fs: refusing to read %s: %s. Use stat_file or shell tools for binaries.",
			path, detail,
		)
	}

	// Dedup: if this session has already read this exact file version, return a
	// stub instead of re-sending the full content.
	hash := contentHash(content)
	if !h.readTrackingDisabled(ctx) && h.hasCurrentFullRead(ctx, absPath, hash) {
		return fileUnchangedStub, taskengine.DataTypeString, nil
	}

	out := string(content)
	if !unlimitedOut && int64(len(out)) > outLimit {
		// Rec 4: never truncate silently. Return a line-based head that fits the
		// output cap and name the exact next step. The file on disk is its own
		// durable copy, so read_file does not spool (a redundant, possibly huge
		// copy) — the model pages forward with start_line. Records a RANGE read
		// only: a truncated read has not seen the whole file, so it must not
		// authorize a blind write_file overwrite.
		head, lastLine, nextLine, _ := streamRange(bytes.NewReader(content), 1, math.MaxInt, outLimit)
		total := len(strings.Split(out, "\n"))
		h.recordRangeRead(ctx, absPath, content)
		notice := fmt.Sprintf(
			"local_fs: read_file truncated — showed lines 1-%d of %d; output capped at %d bytes. To read the next page call read_file with start_line: %d. %s",
			lastLine, total, outLimit, nextLine, severityRecoverable,
		)
		return head + "\n\n" + notice, taskengine.DataTypeString, nil
	}
	h.recordFullRead(ctx, absPath, content)
	return out, taskengine.DataTypeString, nil
}

// notFound builds a recoverable not-found error with a fuzzy "Did you mean:"
// clause over the parent directory's entries (Rec 7). Suggestion only — the tool
// never acts on a fuzzy match (the fuzzy law).
func (h *LocalFSTools) notFound(tool, userPath, absPath string) error {
	msg := fmt.Sprintf("local_fs: %s: %s does not exist", tool, userPath)
	if hint := didYouMean(filepath.Dir(absPath), filepath.Base(absPath)); hint != "" {
		msg += "." + hint
	}
	return recoverablef("%s", msg)
}

type FsWriteResult struct {
	Path      string `json:"path"`
	Written   bool   `json:"written"`
	OldBytes  int    `json:"old_bytes"`
	NewBytes  int    `json:"new_bytes"`
	OldSHA256 string `json:"old_sha256"`
	NewSHA256 string `json:"new_sha256"`
	OldText   string `json:"-"`
	NewText   string `json:"-"`
}

func (r FsWriteResult) ToolDiff() (string, string, string, bool) {
	if r.Path == "" || !r.Written || r.OldText == r.NewText {
		return "", "", "", false
	}
	return r.Path, r.OldText, r.NewText, true
}

type FsSedResult struct {
	Path         string `json:"path"`
	Written      bool   `json:"written"`
	Changed      bool   `json:"changed"`
	Replacements int    `json:"replacements"`
	OldBytes     int    `json:"old_bytes"`
	NewBytes     int    `json:"new_bytes"`
	OldSHA256    string `json:"old_sha256"`
	NewSHA256    string `json:"new_sha256"`
	OldText      string `json:"-"`
	NewText      string `json:"-"`
}

func (r FsSedResult) ToolDiff() (string, string, string, bool) {
	if r.Path == "" || !r.Written || r.OldText == r.NewText {
		return "", "", "", false
	}
	return r.Path, r.OldText, r.NewText, true
}

func (h *LocalFSTools) writeFile(ctx context.Context, args map[string]any) (any, taskengine.DataType, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: path required for write_file")
	}
	content, ok := args["content"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: content required for write_file")
	}

	absPath, err := h.checkPath(ctx, path)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	if err := h.checkDeniedSubstrings(ctx, absPath); err != nil {
		return nil, taskengine.DataTypeAny, err
	}

	if denial, deny := h.requireReadBeforeMutation(ctx, absPath, requireFullFileRead); deny {
		return denial, taskengine.DataTypeString, nil
	}

	oldBytes, readErr := h.fileIO.ReadFile(ctx, absPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read existing file before write: %w", readErr)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		if isDiskFull(err) {
			return nil, taskengine.DataTypeAny, fatalf("disk full", "local_fs: failed to create directories for %s", absPath)
		}
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to create directories: %w", err)
	}

	if err := h.fileIO.WriteFile(ctx, absPath, []byte(content)); err != nil {
		if isDiskFull(err) {
			return nil, taskengine.DataTypeAny, fatalf("disk full", "local_fs: failed to write file %s", absPath)
		}
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to write file: %w", err)
	}
	h.invalidateReads(ctx, absPath)

	return FsWriteResult{
		Path:      absPath,
		Written:   true,
		OldBytes:  len(oldBytes),
		NewBytes:  len(content),
		OldSHA256: contentHash(oldBytes),
		NewSHA256: contentHash([]byte(content)),
		OldText:   string(oldBytes),
		NewText:   content,
	}, taskengine.DataTypeJSON, nil
}

func (h *LocalFSTools) listDir(ctx context.Context, args map[string]any) (any, taskengine.DataType, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}
	listRootArg := filepath.Clean(path)

	absRoot, err := h.checkPath(ctx, listRootArg)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	if err := h.checkDeniedSubstrings(ctx, absRoot); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	st, err := os.Stat(absRoot)
	if err != nil {
		// Rec 7: did-you-mean over sibling names on a missing path.
		if os.IsNotExist(err) {
			return nil, taskengine.DataTypeAny, h.notFound("list_dir", listRootArg, absRoot)
		}
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: stat: %w", err)
	}
	if !st.IsDir() {
		return nil, taskengine.DataTypeAny, fmt.Errorf(
			"local_fs: list_dir: %s is not a directory: %s",
			listRootArg, describePathForError(absRoot, st),
		)
	}

	recursive, _ := argBool(args, "recursive")
	policyMaxDepth := h.maxListDepthFromPolicy(ctx)
	reqDepth := 1
	if recursive {
		reqDepth = 3
		if v, ok := argFloat(args, "max_depth"); ok && int(v) >= 1 {
			reqDepth = int(v)
		}
		if reqDepth > policyMaxDepth {
			reqDepth = policyMaxDepth
		}
	}

	skipDirs := h.skipDirNamesFromPolicy(ctx)
	allowExts := h.listExtensionsFromPolicy(ctx)

	var results []string
	if !recursive {
		entries, err := os.ReadDir(absRoot)
		if err != nil {
			return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read directory: %w", err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if entry.IsDir() {
				if skipDirs[entry.Name()] {
					continue
				}
				results = append(results, entry.Name()+"/")
			} else {
				if allowExts != nil && !allowExts[strings.ToLower(filepath.Ext(entry.Name()))] {
					continue
				}
				name := entry.Name()
				if info, infoErr := entry.Info(); infoErr == nil {
					name += fileEntrySuffix(info)
				}
				results = append(results, name)
			}
		}
	} else {
		if err := h.walkListDir(ctx, listRootArg, absRoot, "", 1, reqDepth, skipDirs, allowExts, &results); err != nil {
			return nil, taskengine.DataTypeAny, err
		}
	}

	out := strings.Join(results, "\n")
	if err := h.checkToolOutputLimit(ctx, "list_dir", out); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	return out, taskengine.DataTypeString, nil
}

// walkListDir appends paths relative to the project root, one per line; directories end with '/'.
// relFromListRoot is the path under listRootArg (POSIX slashes) for the current directory.
// skipDirs is the set of directory basenames to omit from output and recursion (nil = no filter).
// allowExts is the set of lower-cased file extensions to include (nil = all files).
func (h *LocalFSTools) walkListDir(ctx context.Context, listRootArg string, curAbs string, relFromListRoot string, depth, maxDepth int, skipDirs map[string]bool, allowExts map[string]bool, out *[]string) error {
	entries, err := os.ReadDir(curAbs)
	if err != nil {
		return fmt.Errorf("local_fs: failed to read directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if e.IsDir() && skipDirs[e.Name()] {
			continue
		}

		var rel string
		if relFromListRoot == "" {
			rel = e.Name()
		} else {
			rel = filepath.Join(relFromListRoot, e.Name())
		}
		rel = filepath.ToSlash(rel)

		var userPath string
		switch listRootArg {
		case "", ".":
			userPath = rel
		default:
			userPath = filepath.ToSlash(filepath.Join(listRootArg, rel))
		}

		absEntry, err := h.checkPath(ctx, userPath)
		if err != nil {
			continue
		}
		if err := h.checkDeniedSubstrings(ctx, absEntry); err != nil {
			continue
		}

		if e.IsDir() {
			*out = append(*out, userPath+"/")
			if depth >= maxDepth {
				continue
			}
			childAbs := filepath.Join(curAbs, e.Name())
			if err := h.walkListDir(ctx, listRootArg, childAbs, rel, depth+1, maxDepth, skipDirs, allowExts, out); err != nil {
				return err
			}
		} else {
			if allowExts != nil && !allowExts[strings.ToLower(filepath.Ext(e.Name()))] {
				continue
			}
			if info, infoErr := e.Info(); infoErr == nil {
				userPath += fileEntrySuffix(info)
			}
			*out = append(*out, userPath)
		}
	}
	return nil
}

func (h *LocalFSTools) grep(ctx context.Context, args map[string]any) (any, taskengine.DataType, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: path required for grep")
	}
	pattern, ok := args["pattern"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: pattern required for grep")
	}
	if len(pattern) > 8192 {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: pattern exceeds 8192 characters")
	}

	useRegex := false
	if b, ok := argBool(args, "regex"); ok {
		useRegex = b
	}

	var re *regexp.Regexp
	if useRegex {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: invalid regex: %w", err)
		}
	}

	absPath, err := h.checkPath(ctx, path)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	if err := h.precheckFullRead(ctx, absPath); err != nil {
		return nil, taskengine.DataTypeAny, err
	}

	content, err := h.fileIO.ReadFile(ctx, absPath)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	start, end := grepLineRange(args, len(lines))
	maxMatches := h.maxGrepMatchesFromPolicy(ctx)

	var matches []string
	for lineNo := start; lineNo <= end; lineNo++ {
		if lineNo < 1 || lineNo > len(lines) {
			continue
		}
		line := lines[lineNo-1]
		var matched bool
		if useRegex {
			matched = re.MatchString(line)
		} else {
			matched = strings.Contains(line, pattern)
		}
		if !matched {
			continue
		}
		matches = append(matches, fmt.Sprintf("%d: %s", lineNo, line))
		if len(matches) >= maxMatches {
			return nil, taskengine.DataTypeAny, fmt.Errorf(
				"local_fs: grep found at least %d matches (max %d); narrow pattern, set start_line/end_line, or raise _max_grep_matches in tools_policies.local_fs",
				maxMatches, maxMatches,
			)
		}
	}

	out := strings.Join(matches, "\n")
	if err := h.checkToolOutputLimit(ctx, "grep", out); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	return out, taskengine.DataTypeString, nil
}

// maxFindResultsFromPolicy caps find_files results. tools_policies.local_fs: _max_find_results — default 200.
func (h *LocalFSTools) maxFindResultsFromPolicy(ctx context.Context) int {
	const defaultMax = 200
	args := taskengine.ToolsArgsFromContext(ctx, h.name)
	if args == nil {
		return defaultMax
	}
	s := strings.TrimSpace(args["_max_find_results"])
	if s == "" {
		return defaultMax
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return defaultMax
	}
	if n > 5000 {
		return 5000
	}
	return n
}

// findFiles implements find_files: glob-pattern path discovery under the project root.
// Pattern is matched against the file basename (e.g. "*.go") or, when the pattern
// contains a path separator, against the full path relative to the search root.
func (h *LocalFSTools) findFiles(ctx context.Context, args map[string]any) (any, taskengine.DataType, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: pattern required for find_files")
	}
	rootArg, _ := args["path"].(string)
	if rootArg == "" {
		rootArg = "."
	}

	absRoot, err := h.checkPath(ctx, filepath.Clean(rootArg))
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}

	patternHasSlash := strings.ContainsRune(pattern, '/')
	skipDirs := h.skipDirNamesFromPolicy(ctx)
	maxResults := h.maxFindResultsFromPolicy(ctx)

	var matches []string
	truncated := false

	walkErr := filepath.WalkDir(absRoot, func(walkPath string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if truncated {
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(absRoot, walkPath)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		var matched bool
		if patternHasSlash {
			matched, _ = filepath.Match(pattern, rel)
		} else {
			matched, _ = filepath.Match(pattern, d.Name())
		}
		if matched {
			matches = append(matches, rel)
			if len(matches) >= maxResults {
				truncated = true
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: find_files: %w", walkErr)
	}
	if matches == nil {
		matches = []string{}
	}

	type findResult struct {
		Matches   []string `json:"matches"`
		Count     int      `json:"count"`
		Truncated bool     `json:"truncated,omitempty"`
	}
	out, err := json.Marshal(findResult{Matches: matches, Count: len(matches), Truncated: truncated})
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: find_files marshal: %w", err)
	}
	s := string(out)
	if err := h.checkToolOutputLimit(ctx, "find_files", s); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	return s, taskengine.DataTypeJSON, nil
}

// listSizeNoticeThreshold is the size above which list_dir appends a compact
// human-readable size next to a file name. Kept high enough that ordinary
// source/text files never carry the annotation — it exists purely to flag
// files large enough that a model should think twice before read_file'ing
// them.
const listSizeNoticeThreshold = 1 << 20 // 1 MiB

// sniffBinaryBytes bounds how much of a file this package reads to classify
// it as binary vs. text. 512 bytes is enough to catch the common binary
// formats (ELF/PE/Mach-O magic, PNG/JPEG/GIF headers, gzip/zip, ...) cheaply,
// even against a multi-gigabyte file.
const sniffBinaryBytes = 512

// binaryInvalidUTF8Fraction is the share of a sniffed sample that must fail
// to decode as UTF-8 before the sample is classified binary on that basis
// alone (independent of the NUL-byte check in isBinarySample).
const binaryInvalidUTF8Fraction = 0.3

// isExecutable reports whether info's regular-file mode has any executable
// bit set (owner, group, or other). Non-regular modes (directories, sockets,
// ...) are never executable by this check.
//
// Limitation: on Windows, os.FileMode does not carry a meaningful executable
// bit from the filesystem (there is no exec permission bit to read), so this
// always reports false there. That's a known gap in the annotation, not a
// security control — nothing in this package relies on isExecutable to deny
// access.
func isExecutable(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

// humanSize renders a byte count as a compact binary-unit string (KiB, MiB,
// GiB, ...), e.g. humanSize(50746820) == "48 MiB". Values under 1 KiB print
// as whole bytes. This is for model-facing annotations where "48 MiB" reads
// faster than "50746820"; the exact byte count is still reported alongside it
// wherever this is used, so nothing is lost to the rounding.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	val := float64(n) / float64(div)
	if val >= 10 {
		return fmt.Sprintf("%.0f %ciB", val, "KMGTPE"[exp])
	}
	return fmt.Sprintf("%.1f %ciB", val, "KMGTPE"[exp])
}

// isBinarySample applies a cheap, best-effort text/binary heuristic to a
// content prefix (see sniffBinaryBytes): the sample is classified binary if
// it contains a NUL byte — never valid in well-formed UTF-8 text — or if more
// than binaryInvalidUTF8Fraction of it fails to decode as UTF-8.
//
// Known limits, worth remembering before trusting this for anything beyond
// an agent-facing annotation: legacy 8-bit text encodings (Latin-1,
// Shift-JIS, ...) are not valid UTF-8 and can be misclassified as binary;
// conversely a binary format whose first sniffBinaryBytes happen to decode as
// valid UTF-8 (e.g. an archive with an all-ASCII header ahead of a compressed
// payload) can be misclassified as text. This trades precision for being
// cheap enough to run on every read_file/stat_file call — it is not a MIME
// sniffer.
func isBinarySample(sample []byte) bool {
	if len(sample) == 0 {
		return false
	}
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	invalid := 0
	for i := 0; i < len(sample); {
		r, size := utf8.DecodeRune(sample[i:])
		if r == utf8.RuneError && size == 1 {
			invalid++
			i++
			continue
		}
		i += size
	}
	return float64(invalid)/float64(len(sample)) > binaryInvalidUTF8Fraction
}

// sniffPrefix returns the first sniffBinaryBytes of content (or all of it, if
// shorter), for feeding to isBinarySample without re-reading from disk when
// the content is already loaded in memory.
func sniffPrefix(content []byte) []byte {
	if len(content) > sniffBinaryBytes {
		return content[:sniffBinaryBytes]
	}
	return content
}

// sniffBinaryFile classifies a file on disk as binary vs. text by reading at
// most sniffBinaryBytes from its start — independent of any read-size policy,
// so it stays cheap even against a file too large for read_file to ever load
// (e.g. a 50 MB executable). See isBinarySample for the heuristic and its
// limits.
func sniffBinaryFile(absPath string) (bool, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, sniffBinaryBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}
	return isBinarySample(buf[:n]), nil
}

// fileSizeAndExecFlag renders "<size>[, executable]" for compact use inside
// teaching error messages, e.g. "48 MiB, executable" or "312 B".
func fileSizeAndExecFlag(info os.FileInfo) string {
	desc := humanSize(info.Size())
	if isExecutable(info) {
		desc += ", executable"
	}
	return desc
}

// describePathForError renders a short, teaching-oriented description of
// what a path actually is — kind, size, and any executable/binary flags — for
// use in error messages where the model expected something else (e.g.
// list_dir called on a file). absPath is used only for the binary content
// sniff; info supplies everything else.
func describePathForError(absPath string, info os.FileInfo) string {
	kind := "regular file"
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		kind = "symlink"
	case info.IsDir():
		kind = "directory"
	case !info.Mode().IsRegular():
		kind = "special file"
	}
	desc := fmt.Sprintf("%s, %s", kind, humanSize(info.Size()))

	var flags []string
	if isExecutable(info) {
		flags = append(flags, "executable")
	}
	if info.Mode().IsRegular() {
		if binary, err := sniffBinaryFile(absPath); err == nil && binary {
			flags = append(flags, "binary")
		}
	}
	if len(flags) > 0 {
		desc += ", " + strings.Join(flags, " ")
	}
	return desc
}

// fileEntrySuffix renders the ls -F-style annotation appended to a
// non-directory list_dir entry name: "*" when the executable bit is set for
// owner, group, or other, plus a compact human size in parentheses when the
// file exceeds listSizeNoticeThreshold. Small, non-executable files — the
// common case: source files, docs, configs — get no suffix at all, so
// ordinary listings stay exactly as compact as before this annotation
// existed.
func fileEntrySuffix(info os.FileInfo) string {
	var suffix string
	if isExecutable(info) {
		suffix += "*"
	}
	if info.Size() > listSizeNoticeThreshold {
		suffix += " (" + humanSize(info.Size()) + ")"
	}
	return suffix
}

func (h *LocalFSTools) sed(ctx context.Context, args map[string]any) (any, taskengine.DataType, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: path required for sed")
	}
	pattern, ok := args["pattern"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: pattern required for sed")
	}
	replacement, ok := args["replacement"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: replacement required for sed")
	}

	absPath, err := h.checkPath(ctx, path)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	if err := h.precheckFullRead(ctx, absPath); err != nil {
		return nil, taskengine.DataTypeAny, err
	}

	if denial, deny := h.requireReadBeforeMutation(ctx, absPath, requireAnyFileRead); deny {
		return denial, taskengine.DataTypeString, nil
	}

	content, err := h.fileIO.ReadFile(ctx, absPath)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read file: %w", err)
	}

	oldText := string(content)
	replacements := strings.Count(oldText, pattern)
	if replacements == 0 {
		// Rec 7 + the fuzzy law (tool-hardening.md): the pattern was not found.
		// SUGGEST the nearest actual lines and DO NOT mutate — a fuzzy match must
		// never be applied silently (a misplaced edit is corruption). The model
		// reads the suggestion, corrects its pattern, and retries.
		msg := fmt.Sprintf(
			"local_fs: sed: pattern %q not found in %s — file left unchanged; correct the pattern and retry. %s",
			pattern, path, severityRecoverable)
		if near := suggestNearestLines(oldText, pattern, 2); near != "" {
			msg += "\nClosest lines:\n" + near
		}
		return msg, taskengine.DataTypeString, nil
	}
	newContent := strings.ReplaceAll(oldText, pattern, replacement)

	if err := h.fileIO.WriteFile(ctx, absPath, []byte(newContent)); err != nil {
		if isDiskFull(err) {
			return nil, taskengine.DataTypeAny, fatalf("disk full", "local_fs: failed to write file %s", absPath)
		}
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to write file: %w", err)
	}
	h.invalidateReads(ctx, absPath)

	newBytes := []byte(newContent)
	return FsSedResult{
		Path:         absPath,
		Written:      true,
		Changed:      replacements > 0,
		Replacements: replacements,
		OldBytes:     len(content),
		NewBytes:     len(newBytes),
		OldSHA256:    contentHash(content),
		NewSHA256:    contentHash(newBytes),
		OldText:      oldText,
		NewText:      newContent,
	}, taskengine.DataTypeJSON, nil
}

func (h *LocalFSTools) countStats(ctx context.Context, args map[string]any) (any, taskengine.DataType, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: path required for count_stats")
	}

	absPath, err := h.checkPath(ctx, path)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	if err := h.precheckFullRead(ctx, absPath); err != nil {
		return nil, taskengine.DataTypeAny, err
	}

	content, err := h.fileIO.ReadFile(ctx, absPath)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	lineCount := len(lines)
	if len(content) > 0 && content[len(content)-1] == '\n' {
		lineCount--
	}
	wordCount := len(strings.Fields(string(content)))
	byteCount := len(content)

	result := fmt.Sprintf("Lines: %d, Words: %d, Bytes: %d", lineCount, wordCount, byteCount)
	if err := h.checkToolOutputLimit(ctx, "count_stats", result); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	return result, taskengine.DataTypeString, nil
}

func (h *LocalFSTools) readFileRange(ctx context.Context, args map[string]any) (any, taskengine.DataType, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: path required for read_file_range")
	}
	start := 1
	if v, ok := argFloat(args, "start_line"); ok {
		if start = int(v); start < 1 {
			start = 1
		}
	}
	end := math.MaxInt
	if v, ok := argFloat(args, "end_line"); ok {
		end = int(v)
	}
	if end < start {
		end = start
	}

	absPath, err := h.checkPath(ctx, path)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	if err := h.checkDeniedSubstrings(ctx, absPath); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	info, statErr := os.Stat(absPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, taskengine.DataTypeAny, h.notFound("read_file_range", path, absPath)
		}
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: read_file_range: %w", statErr)
	}
	if info.IsDir() {
		return nil, taskengine.DataTypeAny, recoverablef("local_fs: read_file_range: %s is a directory; use list_dir", path)
	}

	outLimit, unlimitedOut := h.maxOutputBytesFromPolicy(ctx)
	readLimit, unlimitedRead := h.maxReadBytesFromPolicy(ctx)
	budget := int64(0)
	if !unlimitedOut && outLimit > 0 {
		budget = outLimit
	}

	// Over the read cap: stream the requested range without loading the whole file
	// (Rec 4). No read marker recorded — an over-cap file cannot be loaded for
	// mutation, so the read-before-write gate is moot for it.
	if !unlimitedRead && info.Size() > readLimit {
		f, err := os.Open(absPath)
		if err != nil {
			return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: read_file_range open: %w", err)
		}
		defer f.Close()
		out, lastLine, nextLine, sErr := streamRange(f, start, end, budget)
		if sErr != nil {
			return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: read_file_range stream: %w", sErr)
		}
		// Only a BUDGET-driven early stop is a truncation: reaching the requested
		// end_line (lastLine == end) returned exactly what was asked for, even
		// though the file continues past it.
		if nextLine != 0 && lastLine < end {
			out += fmt.Sprintf(
				"\n\nlocal_fs: read_file_range truncated — showed lines %d-%d; output capped at %d bytes. To read the next page call read_file with start_line: %d. %s",
				start, lastLine, budget, nextLine, severityRecoverable)
		}
		return out, taskengine.DataTypeString, nil
	}

	content, err := h.fileIO.ReadFile(ctx, absPath)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	totalLines := len(lines)
	if start > totalLines {
		h.recordRangeRead(ctx, absPath, content)
		return "", taskengine.DataTypeString, nil
	}
	e := end
	if e > totalLines {
		e = totalLines
	}
	out := strings.Join(lines[start-1:e], "\n")
	h.recordRangeRead(ctx, absPath, content)

	// Rec 4: truncate-and-continue on the output cap rather than erroring.
	if budget > 0 && int64(len(out)) > budget {
		head, lastLine, nextLine, _ := streamRange(bytes.NewReader([]byte(out)), 1, math.MaxInt, budget)
		absNext := start + nextLine - 1
		notice := fmt.Sprintf(
			"local_fs: read_file_range truncated — showed lines %d-%d; output capped at %d bytes. To read the next page call read_file with start_line: %d. %s",
			start, start+lastLine-1, budget, absNext, severityRecoverable)
		return head + "\n\n" + notice, taskengine.DataTypeString, nil
	}
	return out, taskengine.DataTypeString, nil
}

func (h *LocalFSTools) statFile(ctx context.Context, args map[string]any) (any, taskengine.DataType, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: path required for stat_file")
	}

	absPath, err := h.checkPath(ctx, path)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		// Rec 7: did-you-mean over sibling names on a missing path.
		if os.IsNotExist(err) {
			return nil, taskengine.DataTypeAny, h.notFound("stat_file", path, absPath)
		}
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to stat file: %w", err)
	}

	// binary is only meaningful (and only sniffed) for regular files; a
	// directory or other special file is never "binary" in the sense a model
	// asking whether it's safe to read_file cares about.
	binary := false
	if info.Mode().IsRegular() {
		if b, sniffErr := sniffBinaryFile(absPath); sniffErr == nil {
			binary = b
		}
	}

	result := map[string]any{
		"name":       info.Name(),
		"size":       info.Size(),
		"sizeHuman":  humanSize(info.Size()),
		"modTime":    info.ModTime().Format(time.RFC3339),
		"isDir":      info.IsDir(),
		"mode":       info.Mode().String(),
		"executable": isExecutable(info),
		"binary":     binary,
	}

	b, err := json.Marshal(result)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: marshal stat: %w", err)
	}
	out := string(b)
	if err := h.checkToolOutputLimit(ctx, "stat_file", out); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	return out, taskengine.DataTypeJSON, nil
}

func (h *LocalFSTools) Supports(ctx context.Context) ([]string, error) {
	return []string{h.name, "read_file", "write_file", "list_dir", "grep", "find_files", "sed", "count_stats", "read_file_range", "stat_file"}, nil
}

func (h *LocalFSTools) GetSchemasForSupportedTools(ctx context.Context) (map[string]*openapi3.T, error) {
	return map[string]*openapi3.T{}, nil
}

func (h *LocalFSTools) GetToolsForToolsByName(ctx context.Context, name string) ([]taskengine.Tool, error) {
	// If name is one of the sub-commands, return just that tool.
	// If name is "local_fs", return all of them.

	allTools := []taskengine.Tool{
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "read_file",
				Description: "Read a text file. With no start_line/end_line, reads the whole file and returns the raw text. Refuses with an error for files that sniff as binary (a NUL byte or a high fraction of invalid UTF-8 in the first ~512 bytes) instead of dumping raw bytes into your context — call stat_file first if unsure, or use shell tools for binaries. If you have already read this exact file version this session and it is unchanged on disk, you get a short stub instead of the full content (the prior read is still current). NEVER TRUNCATED SILENTLY: if the file is larger than the read/output cap, the result is a line-based HEAD followed by a notice naming the exact next step — 'call read_file with start_line: N' (a real number). Page forward by passing start_line (and optional end_line); this is the same as read_file_range. A truncated or ranged read does NOT satisfy the full-file read-before-write prerequisite (only a complete read does); a complete read gates write_file/sed on that path this session. Missing paths return a 'Did you mean:' suggestion of similar sibling names. Errors carry a severity marker: '(recoverable: adjust parameters and retry)' when a corrected call fixes it, '(fatal: <reason>)' only for a broken environment.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":       map[string]interface{}{"type": "string", "description": "Path to the file relative to the project root"},
						"start_line": map[string]interface{}{"type": "integer", "description": "Optional 1-based first line to read. Provide this (from a truncation notice) to page forward through a large file. When set, the read is treated as a ranged read."},
						"end_line":   map[string]interface{}{"type": "integer", "description": "Optional 1-based last line to read (inclusive; default: end of file). Only meaningful with a ranged read."},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "write_file",
				Description: "Overwrite a file with new content, or create it if it does not exist. Creates intermediate directories automatically. Returns compact JSON with {path, written, old_bytes, new_bytes, old_sha256, new_sha256}; full old/new file bodies are not returned to the model. Modifying an existing file requires a prior read_file call against the same current version in this session; read_file_range is not sufficient for full-file overwrite. Creating a brand-new file requires no prior read.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":    map[string]interface{}{"type": "string", "description": "Path to the file"},
						"content": map[string]interface{}{"type": "string", "description": "New content for the file"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "list_dir",
				Description: "List entries in a directory under the project root. Non-recursive: one level, names sorted. Set recursive true for a depth-limited tree (paths relative to project root, dirs end with /). Entry names carry ls -F-style hints so you can tell a directory from a text file from an executable binary without a follow-up call: directories end with '/'; a trailing '*' means the executable bit is set; files over 1 MiB get a compact size in parentheses, e.g. 'contenox* (48 MiB)'. Files with no suffix are ordinary, non-executable, non-huge files. Calling list_dir on something that is not a directory returns an error describing what the path actually is (kind, size, executable/binary flags) instead of just saying it isn't a directory. By default, high-noise directories (.git, node_modules, .venv, etc.) are silently omitted — override with _skip_dir_names policy key (comma-separated basenames; empty string disables filtering). Filter returned files by extension with _list_extensions (comma-separated, e.g. .go,.md,.json). A missing path returns a 'Did you mean:' suggestion of similar sibling names; errors carry a '(recoverable: ...)' or '(fatal: ...)' severity marker.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "Directory path relative to project root (default: .)"},
						"recursive": map[string]interface{}{
							"type":        "boolean",
							"description": "If true, list subdirectories up to max_depth (default depth 3; capped by tools policy _max_list_depth)",
						},
						"max_depth": map[string]interface{}{
							"type":        "integer",
							"description": "When recursive is true, maximum directory depth below the listed path (default 3)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "grep",
				Description: "Search a single file for a pattern. Default: literal substring match. Set regex true for RE2 regex. Optional start_line and end_line (1-based, inclusive) limit the search to a line range. Output: matching lines as 'N: text'.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "Path to the file relative to the project root"},
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "Substring to find, or regex pattern when regex is true",
						},
						"regex": map[string]interface{}{
							"type":        "boolean",
							"description": "If true, pattern is a Go RE2 regular expression matched per line",
						},
						"start_line": map[string]interface{}{
							"type":        "integer",
							"description": "First line to search (1-based; default 1)",
						},
						"end_line": map[string]interface{}{
							"type":        "integer",
							"description": "Last line to search inclusive (default: end of file)",
						},
					},
					"required": []string{"path", "pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "find_files",
				Description: "Find files by name pattern under the project root. Uses Go filepath.Match glob syntax: * matches any sequence of non-separator characters, ? matches one character, [range] matches a character class. Note: ** (double-star cross-directory wildcard) is NOT supported. Without a slash in the pattern, the pattern is matched against the file basename only (e.g. \"*.go\" finds all Go files anywhere in the tree). With a slash, the pattern is matched against the relative path. Returns JSON: {matches: [...], count: N, truncated: true|false}. Results are capped at 200 by default (policy: _max_find_results). High-noise directories (.git, node_modules, .venv, etc.) are skipped automatically.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "Glob pattern matched against the file name (e.g. \"*.go\") or relative path when the pattern contains a slash",
						},
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Root directory to search from (relative to project root, default: project root)",
						},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "sed",
				Description: "Replace all literal occurrences of pattern with replacement in a file (plain string replacement, not regex). Replaces every occurrence on every line. Returns compact JSON with {path, written, changed, replacements, old_bytes, new_bytes, old_sha256, new_sha256}; full old/new file bodies are not returned to the model. Requires a prior read_file or read_file_range call against the current file version in this session; editing a file you have not seen, or that changed since you saw it, is blocked. If the pattern is NOT found, the file is left UNCHANGED and you get the closest actual lines as a suggestion ('Closest lines: ...') so you can correct the pattern — a fuzzy match is never applied on your behalf. The suggestion carries a '(recoverable: adjust parameters and retry)' marker.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":        map[string]interface{}{"type": "string", "description": "Path to the file"},
						"pattern":     map[string]interface{}{"type": "string", "description": "String to replace"},
						"replacement": map[string]interface{}{"type": "string", "description": "Replacement string"},
					},
					"required": []string{"path", "pattern", "replacement"},
				},
			},
		},
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "count_stats",
				Description: "Count lines, words, and bytes in a file. Returns a plain string in the format \"Lines: N, Words: N, Bytes: N\". Useful for checking file size before deciding whether to read_file or read_file_range.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "Path to the file"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "read_file_range",
				Description: "Read a contiguous range of lines from a file (1-based, inclusive end_line optional). Works on files of any size (streamed when over the read cap). If the range's output exceeds the output cap it is truncated to a head with a notice naming the exact resume line ('call read_file with start_line: N') — never truncated silently. This satisfies the read-before-mutate prerequisite for targeted sed edits on the same current file version, but not for write_file full-file overwrites. Missing paths return a 'Did you mean:' suggestion. Call read_file (full) before write_file on an existing file.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path":       map[string]interface{}{"type": "string", "description": "Path to the file"},
						"start_line": map[string]interface{}{"type": "integer", "description": "Starting line number (1-indexed, default 1)"},
						"end_line":   map[string]interface{}{"type": "integer", "description": "Ending line number (inclusive, optional)"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "stat_file",
				Description: "Return metadata for a file or directory. Returns JSON with {name, size (bytes), sizeHuman (e.g. \"48 MiB\"), modTime (RFC3339), isDir (bool), mode (Go permission string, e.g. \"-rwxr-xr-x\"), executable (bool: any executable bit set), binary (bool: best-effort content sniff of the first ~512 bytes — NUL byte or high invalid-UTF-8 density; always false for directories)}. Reads at most the first ~512 bytes of file content for the binary check, never the whole file. Use this before read_file when unsure whether a path is a directory, a text file, or an executable/binary — a 50 MB binary reports as {isDir:false, executable:true, binary:true, sizeHuman:\"48 MiB\", ...}. A missing path returns a 'Did you mean:' suggestion of similar sibling names.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "Path to the file/directory"},
					},
					"required": []string{"path"},
				},
			},
		},
	}

	if name == h.name {
		return allTools, nil
	}

	for _, t := range allTools {
		if t.Function.Name == name {
			return []taskengine.Tool{t}, nil
		}
	}

	return nil, fmt.Errorf("unknown tools tool: %s", name)
}

var _ taskengine.ToolsRepo = (*LocalFSTools)(nil)

// sessionIDFromContext returns the active session ID set by the chat command,
// or "" when running outside a session (e.g. one-shot contenox run).
func sessionIDFromContext(ctx context.Context) string {
	v := ctx.Value(runtimetypes.SessionIDContextKey)
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func contentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:])
}

// rangeReadMarkerPath returns a separate DB key for partial/range reads.
//
// Full-file reads intentionally keep using the plain canonical absPath key for
// backward compatibility with the existing local_fs_reads table and tests.
// Range reads must not use the same key, otherwise read_file_range unlocks
// write_file full overwrites.
func rangeReadMarkerPath(absPath string) string {
	return "range:" + absPath
}

func fullHashMarkerPath(absPath, hash string) string {
	return "fullhash:" + absPath + ":" + hash
}

func rangeHashMarkerPath(absPath, hash string) string {
	return "rangehash:" + absPath + ":" + hash
}

// recordFullRead persists that this session has read the full absPath and the
// exact content version that was observed.
func (h *LocalFSTools) recordFullRead(ctx context.Context, absPath string, content []byte) {
	hash := contentHash(content)
	h.recordReadMarker(ctx, absPath)
	h.recordReadMarker(ctx, fullHashMarkerPath(absPath, hash))
}

// recordRangeRead persists that this session has read only a line range from
// absPath and the exact file version from which the range was taken.
// This is enough for targeted mutators such as sed, but not for write_file.
func (h *LocalFSTools) recordRangeRead(ctx context.Context, absPath string, content []byte) {
	hash := contentHash(content)
	h.recordReadMarker(ctx, rangeReadMarkerPath(absPath))
	h.recordReadMarker(ctx, rangeHashMarkerPath(absPath, hash))
}

func (h *LocalFSTools) recordReadMarker(ctx context.Context, markerPath string) {
	if h.db == nil {
		return
	}
	sessionID := sessionIDFromContext(ctx)
	if sessionID == "" {
		return
	}
	exec := h.db.WithoutTransaction()
	_, _ = exec.ExecContext(ctx,
		`INSERT INTO local_fs_reads (session_id, path, last_read_at) VALUES (?, ?, ?)
		 ON CONFLICT (session_id, path) DO UPDATE SET last_read_at = excluded.last_read_at`,
		sessionID, markerPath, time.Now().UTC(),
	)
}

func (h *LocalFSTools) readTrackingDisabled(ctx context.Context) bool {
	return h.db == nil || sessionIDFromContext(ctx) == ""
}

// hasPriorRead reports whether the current session has called read_file against
// absPath. Returns true (fail-open) when no DB is configured or no session ID is
// in scope, since the guard only applies when the tool can scope its check.
func (h *LocalFSTools) hasPriorRead(ctx context.Context, absPath string) bool {
	return h.hasReadMarker(ctx, absPath)
}

// hasPriorRangeRead reports whether the current session has called
// read_file_range against absPath.
func (h *LocalFSTools) hasPriorRangeRead(ctx context.Context, absPath string) bool {
	return h.hasReadMarker(ctx, rangeReadMarkerPath(absPath))
}

func (h *LocalFSTools) hasCurrentFullRead(ctx context.Context, absPath, currentHash string) bool {
	return h.hasReadMarker(ctx, fullHashMarkerPath(absPath, currentHash))
}

func (h *LocalFSTools) hasCurrentRangeRead(ctx context.Context, absPath, currentHash string) bool {
	return h.hasReadMarker(ctx, rangeHashMarkerPath(absPath, currentHash))
}

func (h *LocalFSTools) hasAnyPriorRead(ctx context.Context, absPath string) bool {
	if h.readTrackingDisabled(ctx) {
		return true
	}
	return h.hasReadMarker(ctx, absPath) || h.hasReadMarker(ctx, rangeReadMarkerPath(absPath))
}

func (h *LocalFSTools) hasReadMarker(ctx context.Context, markerPath string) bool {
	if h.db == nil {
		return true
	}
	sessionID := sessionIDFromContext(ctx)
	if sessionID == "" {
		return true
	}
	exec := h.db.WithoutTransaction()
	var dummy string
	err := exec.QueryRowContext(ctx,
		`SELECT path FROM local_fs_reads WHERE session_id = ? AND path = ?`,
		sessionID, markerPath,
	).Scan(&dummy)
	if err == nil {
		return true
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	// Any other DB error: fail open. A tracker outage shouldn't block the model.
	return true
}

// requireReadBeforeMutation enforces the read-before-write contract for an
// existing file. Returns (denialMessage, true) when the call should be denied
// with a soft tool-result message; ("", false) when the call may proceed.
// New files (not yet on disk) always pass through.
func (h *LocalFSTools) requireReadBeforeMutation(ctx context.Context, absPath string, requirement readRequirement) (string, bool) {
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return "", false
		}
		// Permission/IO error: let the actual mutation attempt surface it.
		return "", false
	}

	if h.readTrackingDisabled(ctx) {
		return "", false
	}

	currentBytes, err := h.fileIO.ReadFile(ctx, absPath)
	if err != nil {
		// Let the actual mutation attempt surface the I/O error.
		return "", false
	}
	currentHash := contentHash(currentBytes)

	switch requirement {
	case requireFullFileRead:
		if h.hasCurrentFullRead(ctx, absPath, currentHash) {
			return "", false
		}
		if h.hasCurrentRangeRead(ctx, absPath, currentHash) {
			return fmt.Sprintf(readBeforeWriteFullReadDenial, absPath, absPath), true
		}
		if h.hasAnyPriorRead(ctx, absPath) {
			return fmt.Sprintf(readBeforeWriteStaleReadDenial, absPath, absPath), true
		}
		return fmt.Sprintf(readBeforeWriteDenial, absPath, absPath), true

	case requireAnyFileRead:
		if h.hasCurrentFullRead(ctx, absPath, currentHash) || h.hasCurrentRangeRead(ctx, absPath, currentHash) {
			return "", false
		}
		if h.hasAnyPriorRead(ctx, absPath) {
			return fmt.Sprintf(readBeforeWriteStaleReadDenial, absPath, absPath), true
		}
		return fmt.Sprintf(readBeforeWriteDenial, absPath, absPath), true

	default:
		if h.hasCurrentFullRead(ctx, absPath, currentHash) {
			return "", false
		}
		if h.hasAnyPriorRead(ctx, absPath) {
			return fmt.Sprintf(readBeforeWriteStaleReadDenial, absPath, absPath), true
		}
		return fmt.Sprintf(readBeforeWriteDenial, absPath, absPath), true
	}
}

func escapeSQLiteLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func (h *LocalFSTools) invalidateReads(ctx context.Context, absPath string) {
	if h.db == nil {
		return
	}
	sessionID := sessionIDFromContext(ctx)
	if sessionID == "" {
		return
	}

	fullPrefix := escapeSQLiteLike("fullhash:"+absPath+":") + "%"
	rangePrefix := escapeSQLiteLike("rangehash:"+absPath+":") + "%"

	exec := h.db.WithoutTransaction()
	_, _ = exec.ExecContext(ctx,
		`DELETE FROM local_fs_reads
		  WHERE session_id = ?
		    AND (
		      path = ?
		      OR path = ?
		      OR path LIKE ? ESCAPE '\'
		      OR path LIKE ? ESCAPE '\'
		    )`,
		sessionID,
		absPath,
		rangeReadMarkerPath(absPath),
		fullPrefix,
		rangePrefix,
	)
}
