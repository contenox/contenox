package localtools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	libdb "github.com/contenox/agent/libdbexec"
	"github.com/contenox/agent/runtime/runtimetypes"
	"github.com/contenox/agent/runtime/taskengine"
	"github.com/getkin/kin-openapi/openapi3"
)

const LocalFSToolsName = "local_fs"

// readBeforeWriteDenial is the LLM-facing message returned when the model tries
// to mutate an existing file it has not read in this session. The model treats
// it as a normal tool result and is expected to call read_file then retry.
const readBeforeWriteDenial = "local_fs: cannot modify existing file %s without reading it first. Call local_fs.read_file(%q) to confirm the current contents, then retry."

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

// Exec handles filesystem tool execution.
func (h *LocalFSTools) Exec(ctx context.Context, startTime time.Time, input any, debug bool, toolsCall *taskengine.ToolsCall) (any, taskengine.DataType, error) {
	if toolsCall == nil {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: tools required")
	}

	args, ok := input.(map[string]any)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: input must be a map")
	}

	toolName := toolsCall.ToolName
	if toolName == "" {
		toolName = toolsCall.Name
	}

	switch toolName {
	case "read_file":
		return h.readFile(ctx, args)
	case "write_file":
		return h.writeFile(ctx, args)
	case "list_dir":
		return h.listDir(ctx, args)
	case "grep":
		return h.grep(ctx, args)
	case "sed":
		return h.sed(ctx, args)
	case "count_stats":
		return h.countStats(ctx, args)
	case "read_file_range":
		return h.readFileRange(ctx, args)
	case "stat_file":
		return h.statFile(ctx, args)
	default:
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: unknown tool %s", toolName)
	}
}

// checkPath verifies if a path is within the allowed directory.
// It resolves symlinks so that a symlink inside the sandbox pointing outside it
// (e.g. ln -s /etc /allowed/link) is caught before any I/O is performed.
func (h *LocalFSTools) checkPath(ctx context.Context, path string) (string, error) {
	base := h.allowedDir
	if base == "" && h.cwdResolver != nil {
		if r := h.cwdResolver(ctx); r != "" {
			base = filepath.Clean(r)
		}
	}
	if base == "" {
		return "", errors.New("local_fs: no allowed directory configured")
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("local_fs: invalid allowed dir: %w", err)
	}

	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Join(absBase, path)
	}
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("local_fs: invalid path: %w", err)
	}

	// Resolve symlinks to find the true on-disk destination.
	// We only skip on NotExist so write_file to new files still works.
	realPath, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		absPath = realPath
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("local_fs: path resolution error: %w", err)
	}

	// Use the strict prefix check: ".." alone or "../" prefix.
	// strings.HasPrefix(rel, "..") would falsely trigger for "..hidden".
	sep := string(filepath.Separator)
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+sep) {
		return "", fmt.Errorf("local_fs: path %s escapes allowed directory %s", path, base)
	}

	return absPath, nil
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
	base, err := filepath.Abs(h.allowedDir)
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
	if err := h.precheckFullRead(ctx, absPath); err != nil {
		return nil, taskengine.DataTypeAny, err
	}

	content, err := h.fileIO.ReadFile(ctx, absPath)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read file: %w", err)
	}

	out := string(content)
	if err := h.checkToolOutputLimit(ctx, "read_file", out); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	h.recordRead(ctx, absPath)
	return out, taskengine.DataTypeString, nil
}

type FsWriteResult struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
	Written bool   `json:"written"`
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

	if denial, deny := h.requireReadBeforeMutation(ctx, absPath); deny {
		return denial, taskengine.DataTypeString, nil
	}

	oldBytes, readErr := h.fileIO.ReadFile(ctx, absPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read existing file before write: %w", readErr)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to create directories: %w", err)
	}

	if err := h.fileIO.WriteFile(ctx, absPath, []byte(content)); err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to write file: %w", err)
	}

	return FsWriteResult{
		Path:    absPath,
		OldText: string(oldBytes),
		NewText: content,
		Written: true,
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
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: stat: %w", err)
	}
	if !st.IsDir() {
		return nil, taskengine.DataTypeAny, errors.New("local_fs: list_dir path must be a directory")
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

	var results []string
	if !recursive {
		entries, err := os.ReadDir(absRoot)
		if err != nil {
			return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read directory: %w", err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			suffix := ""
			if entry.IsDir() {
				suffix = "/"
			}
			results = append(results, entry.Name()+suffix)
		}
	} else {
		if err := h.walkListDir(ctx, listRootArg, absRoot, "", 1, reqDepth, &results); err != nil {
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
func (h *LocalFSTools) walkListDir(ctx context.Context, listRootArg string, curAbs string, relFromListRoot string, depth, maxDepth int, out *[]string) error {
	entries, err := os.ReadDir(curAbs)
	if err != nil {
		return fmt.Errorf("local_fs: failed to read directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
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
			if err := h.walkListDir(ctx, listRootArg, childAbs, rel, depth+1, maxDepth, out); err != nil {
				return err
			}
		} else {
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

	if denial, deny := h.requireReadBeforeMutation(ctx, absPath); deny {
		return denial, taskengine.DataTypeString, nil
	}

	content, err := h.fileIO.ReadFile(ctx, absPath)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read file: %w", err)
	}

	newContent := strings.ReplaceAll(string(content), pattern, replacement)

	if err := h.fileIO.WriteFile(ctx, absPath, []byte(newContent)); err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to write file: %w", err)
	}

	return "ok", taskengine.DataTypeString, nil
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
	startLine, ok := args["start_line"].(float64)
	if !ok {
		startLine = 1
	}
	endLine, ok := args["end_line"].(float64)

	absPath, err := h.checkPath(ctx, path)
	if err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	if err := h.checkDeniedSubstrings(ctx, absPath); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	// Line-range reads still load the full file internally; enforce size to avoid multi-GB reads.
	if err := h.checkFileSizeLimit(ctx, absPath); err != nil {
		return nil, taskengine.DataTypeAny, err
	}

	content, err := h.fileIO.ReadFile(ctx, absPath)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	totalLines := len(lines)

	s := int(startLine)
	if s < 1 {
		s = 1
	}
	if s > totalLines {
		return "", taskengine.DataTypeString, nil
	}

	e := totalLines
	if ok {
		e = int(endLine)
	}
	if e < s {
		e = s
	}
	if e > totalLines {
		e = totalLines
	}

	resultLines := lines[s-1 : e]
	out := strings.Join(resultLines, "\n")
	if err := h.checkToolOutputLimit(ctx, "read_file_range", out); err != nil {
		return nil, taskengine.DataTypeAny, err
	}
	h.recordRead(ctx, absPath)
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
		return nil, taskengine.DataTypeAny, fmt.Errorf("local_fs: failed to stat file: %w", err)
	}

	result := map[string]any{
		"name":    info.Name(),
		"size":    info.Size(),
		"modTime": info.ModTime().Format(time.RFC3339),
		"isDir":   info.IsDir(),
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
	return []string{h.name, "read_file", "write_file", "list_dir", "grep", "sed", "count_stats", "read_file_range", "stat_file"}, nil
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
				Description: "Read the full content of a text file under the project root. For large files use read_file_range instead. Calling this is also a prerequisite for write_file or sed against an existing file — the path you read here unlocks subsequent mutations of that same path in this session.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string", "description": "Path to the file relative to the project root"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "write_file",
				Description: "Write content to a file. Overwrites existing content. Creates directories if needed. Modifying an existing file requires that you have first called read_file or read_file_range against the same path in this session — this guards against overwriting files you have not actually seen. Creating a brand-new file (path does not yet exist) needs no prior read.",
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
				Description: "List entries in a directory under the project root. Non-recursive: one level, names sorted. Set recursive true for a depth-limited tree (paths relative to project root, dirs end with /).",
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
				Name:        "sed",
				Description: "Replace occurrences of a pattern with a replacement in a file. Requires that you have first called read_file or read_file_range against the same path in this session — modifying a file you have not seen is blocked.",
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
				Description: "Count lines, words, and bytes in a file (like wc)",
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
				Description: "Read a contiguous range of lines from a file (1-based, inclusive end_line optional). Like read_file, calling this satisfies the read-before-mutate prerequisite for write_file and sed against the same path in this session.",
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
				Description: "Get file metadata",
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

// recordRead persists that this session has read absPath. Errors are silent —
// the read itself already succeeded; failing the tool call because of a tracker
// glitch would be worse than letting the next write proceed unguarded.
func (h *LocalFSTools) recordRead(ctx context.Context, absPath string) {
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
		sessionID, absPath, time.Now().UTC(),
	)
}

// hasPriorRead reports whether the current session has called read_file or
// read_file_range against absPath. Returns true (fail-open) when no DB is
// configured or no session ID is in scope, since the guard only applies when
// the tool can scope its check.
func (h *LocalFSTools) hasPriorRead(ctx context.Context, absPath string) bool {
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
		sessionID, absPath,
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
func (h *LocalFSTools) requireReadBeforeMutation(ctx context.Context, absPath string) (string, bool) {
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return "", false
		}
		// Permission/IO error: let the actual write attempt surface it.
		return "", false
	}
	if h.hasPriorRead(ctx, absPath) {
		return "", false
	}
	return fmt.Sprintf(readBeforeWriteDenial, absPath, absPath), true
}
