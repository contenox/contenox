package localtools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/runtime/localtools"
	"github.com/contenox/runtime/runtime/taskengine"
)

func TestUnit_LocalFSTools_Exec(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "contenox-fs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	h := localtools.NewLocalFSTools(tempDir, nil)
	ctx := context.Background()
	now := time.Now()

	t.Run("writeFile", func(t *testing.T) {
		args := map[string]any{
			"path":    "test.txt",
			"content": "hello world\nline 2\nline 3",
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "write_file"}
		res, dataType, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		fw, ok := res.(localtools.FsWriteResult)
		if !ok || dataType != taskengine.DataTypeJSON {
			t.Errorf("unexpected result: %v (%T), %v", res, res, dataType)
		}
		if !fw.Written || fw.NewText != "hello world\nline 2\nline 3" {
			t.Errorf("unexpected FsWriteResult: %+v", fw)
		}
	})

	t.Run("readFile", func(t *testing.T) {
		args := map[string]any{"path": "test.txt"}
		toolsCall := &taskengine.ToolsCall{ToolName: "read_file"}
		res, dataType, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		content := res.(string)
		if !strings.Contains(content, "hello world") || dataType != taskengine.DataTypeString {
			t.Errorf("unexpected content: %q", content)
		}
	})

	t.Run("listDir", func(t *testing.T) {
		args := map[string]any{"path": "."}
		toolsCall := &taskengine.ToolsCall{ToolName: "list_dir"}
		res, dataType, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		if dataType != taskengine.DataTypeString {
			t.Errorf("unexpected data type: %v", dataType)
		}
		files := res.(string)
		if files == "." {
			t.Errorf("list_dir output should not literally be just the requested path string %q", files)
		}
		if !strings.Contains(files, "test.txt") {
			t.Errorf("unexpected files: %q", files)
		}
	})

	t.Run("grep", func(t *testing.T) {
		args := map[string]any{
			"path":    "test.txt",
			"pattern": "line 2",
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "grep"}
		res, dataType, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		match := res.(string)
		if !strings.Contains(match, "2: line 2") || dataType != taskengine.DataTypeString {
			t.Errorf("unexpected match: %q", match)
		}
	})

	t.Run("sed", func(t *testing.T) {
		args := map[string]any{
			"path":        "test.txt",
			"pattern":     "line 3",
			"replacement": "modified line 3",
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "sed"}
		res, _, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		sed, ok := res.(localtools.FsSedResult)
		if !ok || !sed.Written || !sed.Changed || sed.Replacements != 1 {
			t.Errorf("unexpected result: %v", res)
		}

		// Verify change
		argsRead := map[string]any{"path": "test.txt"}
		readCall := &taskengine.ToolsCall{ToolName: "read_file"}
		resRead, _, _ := h.Exec(ctx, now, argsRead, false, readCall)
		if !strings.Contains(resRead.(string), "modified line 3") {
			t.Errorf("sed failed to modify content: %q", resRead)
		}
	})

	t.Run("SecurityRestriction", func(t *testing.T) {
		args := map[string]any{"path": "/etc/passwd"}
		toolsCall := &taskengine.ToolsCall{ToolName: "read_file"}
		_, _, err := h.Exec(ctx, now, args, false, toolsCall)
		if err == nil {
			t.Error("expected error for path outside allowed dir, got nil")
		} else if !strings.Contains(err.Error(), "escapes allowed directory") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("UnknownArgsRejected", func(t *testing.T) {
		args := map[string]any{"path": "test.txt", "unexpected": true}
		toolsCall := &taskengine.ToolsCall{ToolName: "read_file"}
		_, _, err := h.Exec(ctx, now, args, false, toolsCall)
		if err == nil {
			t.Fatal("expected unknown argument error")
		}
		if !strings.Contains(err.Error(), "unexpected") {
			t.Fatalf("expected error to name unknown argument, got %v", err)
		}
	})

	t.Run("MkdirAllVerification", func(t *testing.T) {
		args := map[string]any{
			"path":    "subdir/another/file.txt",
			"content": "nested content",
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "write_file"}
		_, _, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(filepath.Join(tempDir, "subdir/another/file.txt")); os.IsNotExist(err) {
			t.Error("failed to create nested directories and file")
		}
	})

	t.Run("countStats", func(t *testing.T) {
		args := map[string]any{"path": "test.txt"}
		toolsCall := &taskengine.ToolsCall{ToolName: "count_stats"}
		res, dataType, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		stats := res.(string)
		// test.txt has: "hello world\nline 2\nmodified line 3" (modified in sed test)
		// Lines: 3, Words: 6, Bytes: ?
		if !strings.Contains(stats, "Lines: 3") || dataType != taskengine.DataTypeString {
			t.Errorf("unexpected stats: %q", stats)
		}
	})

	t.Run("readFileRange", func(t *testing.T) {
		args := map[string]any{
			"path":       "test.txt",
			"start_line": float64(2),
			"end_line":   float64(2),
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "read_file_range"}
		res, dataType, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		rangeContent := res.(string)
		if rangeContent != "line 2" || dataType != taskengine.DataTypeString {
			t.Errorf("unexpected range content: %q", rangeContent)
		}
	})

	t.Run("maxReadBytesRejectsLargeFile", func(t *testing.T) {
		bigPath := filepath.Join(tempDir, "big.bin")
		f, err := os.Create(bigPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(make([]byte, 2*1024*1024)); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()

		args := map[string]any{"path": "big.bin"}
		toolsCall := &taskengine.ToolsCall{ToolName: "read_file"}
		_, _, err = h.Exec(ctx, now, args, false, toolsCall)
		if err == nil {
			t.Fatal("expected error for file over default max read size")
		}
		if !strings.Contains(err.Error(), "max") {
			t.Fatalf("expected max size hint: %v", err)
		}
	})

	t.Run("maxReadBytesUnlimited", func(t *testing.T) {
		ctxUnlimited := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_max_read_bytes":   "-1",
			"_max_output_bytes": "-1",
		})
		args := map[string]any{"path": "big.bin"}
		toolsCall := &taskengine.ToolsCall{ToolName: "read_file"}
		_, _, err := h.Exec(ctxUnlimited, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("maxOutputBytesRejectsOversizedResult", func(t *testing.T) {
		ctxSmallOut := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_max_read_bytes":   "-1",
			"_max_output_bytes": "64",
		})
		args := map[string]any{"path": "big.bin"}
		toolsCall := &taskengine.ToolsCall{ToolName: "read_file"}
		_, _, err := h.Exec(ctxSmallOut, now, args, false, toolsCall)
		if err == nil {
			t.Fatal("expected error when tool output exceeds _max_output_bytes")
		}
		if !strings.Contains(err.Error(), "read_file output") || !strings.Contains(err.Error(), "max") {
			t.Fatalf("expected output limit hint: %v", err)
		}
	})

	t.Run("maxOutputBytesUnlimited", func(t *testing.T) {
		ctxBoth := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_max_read_bytes":   "-1",
			"_max_output_bytes": "-1",
		})
		args := map[string]any{"path": "big.bin"}
		toolsCall := &taskengine.ToolsCall{ToolName: "read_file"}
		_, _, err := h.Exec(ctxBoth, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("deniedPathSubstrings", func(t *testing.T) {
		ctxDeny := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_denied_path_substrings": "node_modules,secret",
		})
		args := map[string]any{"path": "pkg/node_modules/foo.txt"}
		_ = os.MkdirAll(filepath.Join(tempDir, "pkg/node_modules"), 0755)
		if err := os.WriteFile(filepath.Join(tempDir, "pkg/node_modules/foo.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "read_file"}
		_, _, err := h.Exec(ctxDeny, now, args, false, toolsCall)
		if err == nil {
			t.Fatal("expected denied path error")
		}
		if !strings.Contains(err.Error(), "denied") {
			t.Fatalf("expected denied: %v", err)
		}
	})

	t.Run("allowedDirFromPolicy", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "policy.txt"), []byte("policy ok"), 0644); err != nil {
			t.Fatal(err)
		}
		ctxPolicy := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_allowed_dir": root,
		})
		h2 := localtools.NewLocalFSTools("", nil)
		res, dataType, err := h2.Exec(ctxPolicy, now, map[string]any{"path": "policy.txt"}, false, &taskengine.ToolsCall{ToolName: "read_file"})
		if err != nil {
			t.Fatal(err)
		}
		if dataType != taskengine.DataTypeString || res.(string) != "policy ok" {
			t.Fatalf("unexpected read result: %v (%v)", res, dataType)
		}
	})

	t.Run("relativeAllowedDirFromPolicyUsesCwdResolver", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "workspace"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "workspace", "policy.txt"), []byte("workspace policy ok"), 0644); err != nil {
			t.Fatal(err)
		}
		ctxPolicy := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_allowed_dir": "workspace",
		})
		h2 := localtools.NewLocalFSToolsWith("", nil, nil, localtools.LocalFSToolsName, func(context.Context) string {
			return root
		})
		res, dataType, err := h2.Exec(ctxPolicy, now, map[string]any{"path": "policy.txt"}, false, &taskengine.ToolsCall{ToolName: "read_file"})
		if err != nil {
			t.Fatal(err)
		}
		if dataType != taskengine.DataTypeString || res.(string) != "workspace policy ok" {
			t.Fatalf("unexpected read result: %v (%v)", res, dataType)
		}
	})

	t.Run("statFile", func(t *testing.T) {
		args := map[string]any{"path": "test.txt"}
		toolsCall := &taskengine.ToolsCall{ToolName: "stat_file"}
		res, dataType, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		if dataType != taskengine.DataTypeJSON {
			t.Errorf("unexpected data type: %v", dataType)
		}
		statStr := res.(string)
		if !strings.Contains(statStr, "\"name\":\"test.txt\"") {
			t.Errorf("unexpected stat output: %q", statStr)
		}
	})

	t.Run("grepLineRange", func(t *testing.T) {
		args := map[string]any{
			"path":       "test.txt",
			"pattern":    "line",
			"start_line": float64(2),
			"end_line":   float64(2),
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "grep"}
		res, _, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.(string), "2: line 2") {
			t.Fatalf("expected line 2 only: %q", res)
		}
	})

	t.Run("grepRegex", func(t *testing.T) {
		args := map[string]any{
			"path":    "test.txt",
			"pattern": `^line \d$`,
			"regex":   true,
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "grep"}
		res, _, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		s := res.(string)
		if !strings.Contains(s, "2: line 2") || strings.Contains(s, "modified") {
			t.Fatalf("unexpected regex grep: %q", s)
		}
	})

	t.Run("grepInvalidRegex", func(t *testing.T) {
		args := map[string]any{
			"path":    "test.txt",
			"pattern": "(",
			"regex":   true,
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "grep"}
		_, _, err := h.Exec(ctx, now, args, false, toolsCall)
		if err == nil {
			t.Fatal("expected invalid regex error")
		}
	})

	t.Run("grepMaxMatches", func(t *testing.T) {
		ctxLim := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_max_grep_matches": "1",
		})
		args := map[string]any{
			"path":    "test.txt",
			"pattern": "e",
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "grep"}
		_, _, err := h.Exec(ctxLim, now, args, false, toolsCall)
		if err == nil {
			t.Fatal("expected max grep matches error")
		}
		if !strings.Contains(err.Error(), "_max_grep_matches") {
			t.Fatalf("expected policy hint: %v", err)
		}
	})

	t.Run("listDirRecursive", func(t *testing.T) {
		_ = os.MkdirAll(filepath.Join(tempDir, "walktree/sub"), 0755)
		if err := os.WriteFile(filepath.Join(tempDir, "walktree/sub/leaf.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		args := map[string]any{
			"path":      "walktree",
			"recursive": true,
			"max_depth": float64(3),
		}
		toolsCall := &taskengine.ToolsCall{ToolName: "list_dir"}
		res, _, err := h.Exec(ctx, now, args, false, toolsCall)
		if err != nil {
			t.Fatal(err)
		}
		s := res.(string)
		if !strings.Contains(s, "walktree/sub/") || !strings.Contains(s, "walktree/sub/leaf.txt") {
			t.Fatalf("expected nested paths: %q", s)
		}
	})

	t.Run("listDirMustBeDirectory", func(t *testing.T) {
		args := map[string]any{"path": "test.txt"}
		toolsCall := &taskengine.ToolsCall{ToolName: "list_dir"}
		_, _, err := h.Exec(ctx, now, args, false, toolsCall)
		if err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("expected not-a-directory: %v", err)
		}
	})

	// --- list_dir noise-filtering tests ---

	// TestFailure: before the fix, list_dir(".") on a project root would return
	// .git/ and node_modules/ entries, flooding the model's context window with
	// thousands of irrelevant paths. The default skip set must prevent this.
	t.Run("listDirSkipsNoiseDirsDefault", func(t *testing.T) {
		root := t.TempDir()
		// Simulate the real-project layout that caused the context flood.
		_ = os.MkdirAll(filepath.Join(root, ".git", "objects"), 0755)
		_ = os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0644)
		_ = os.MkdirAll(filepath.Join(root, "node_modules", "react"), 0755)
		_ = os.WriteFile(filepath.Join(root, "node_modules", "react", "index.js"), []byte("//react\n"), 0644)
		_ = os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0644)

		h2 := localtools.NewLocalFSTools(root, nil)
		res, _, err := h2.Exec(ctx, now, map[string]any{"path": "."}, false, &taskengine.ToolsCall{ToolName: "list_dir"})
		if err != nil {
			t.Fatal(err)
		}
		listing := res.(string)
		if strings.Contains(listing, ".git") {
			t.Errorf("default listing must not contain .git, got:\n%s", listing)
		}
		if strings.Contains(listing, "node_modules") {
			t.Errorf("default listing must not contain node_modules, got:\n%s", listing)
		}
		if !strings.Contains(listing, "main.go") {
			t.Errorf("default listing must contain main.go, got:\n%s", listing)
		}
	})

	t.Run("listDirSkipDirNamesCustom", func(t *testing.T) {
		root := t.TempDir()
		_ = os.MkdirAll(filepath.Join(root, "build"), 0755)
		_ = os.MkdirAll(filepath.Join(root, "src"), 0755)
		_ = os.WriteFile(filepath.Join(root, "src", "app.go"), []byte("package main\n"), 0644)

		ctxCustom := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_skip_dir_names": "build",
		})
		h2 := localtools.NewLocalFSTools(root, nil)
		res, _, err := h2.Exec(ctxCustom, now, map[string]any{"path": "."}, false, &taskengine.ToolsCall{ToolName: "list_dir"})
		if err != nil {
			t.Fatal(err)
		}
		listing := res.(string)
		if strings.Contains(listing, "build") {
			t.Errorf("build/ must be skipped by custom policy, got:\n%s", listing)
		}
		if !strings.Contains(listing, "src/") {
			t.Errorf("src/ must appear when not in skip list, got:\n%s", listing)
		}
	})

	t.Run("listDirSkipDirNamesDisabled", func(t *testing.T) {
		root := t.TempDir()
		_ = os.MkdirAll(filepath.Join(root, ".git"), 0755)
		_ = os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0644)

		// Empty string disables the default filter entirely.
		ctxDisabled := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_skip_dir_names": "",
		})
		h2 := localtools.NewLocalFSTools(root, nil)
		res, _, err := h2.Exec(ctxDisabled, now, map[string]any{"path": "."}, false, &taskengine.ToolsCall{ToolName: "list_dir"})
		if err != nil {
			t.Fatal(err)
		}
		listing := res.(string)
		if !strings.Contains(listing, ".git/") {
			t.Errorf("disabled skip filter must show .git/, got:\n%s", listing)
		}
	})

	t.Run("listDirExtensionFilter", func(t *testing.T) {
		root := t.TempDir()
		_ = os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0644)
		_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("# readme\n"), 0644)
		_ = os.WriteFile(filepath.Join(root, "go.sum"), []byte("hash\n"), 0644)
		_ = os.WriteFile(filepath.Join(root, "logo.png"), []byte("\x89PNG\r\n"), 0644)

		ctxExt := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_list_extensions": ".go,.md",
			"_skip_dir_names":  "", // show everything dir-wise; we only filter files
		})
		h2 := localtools.NewLocalFSTools(root, nil)
		res, _, err := h2.Exec(ctxExt, now, map[string]any{"path": "."}, false, &taskengine.ToolsCall{ToolName: "list_dir"})
		if err != nil {
			t.Fatal(err)
		}
		listing := res.(string)
		if !strings.Contains(listing, "main.go") {
			t.Errorf("main.go must appear with .go filter, got:\n%s", listing)
		}
		if !strings.Contains(listing, "README.md") {
			t.Errorf("README.md must appear with .md filter, got:\n%s", listing)
		}
		if strings.Contains(listing, "go.sum") {
			t.Errorf("go.sum must be hidden by extension filter, got:\n%s", listing)
		}
		if strings.Contains(listing, "logo.png") {
			t.Errorf("logo.png must be hidden by extension filter, got:\n%s", listing)
		}
	})

	t.Run("listDirExtensionFilterRecursive", func(t *testing.T) {
		root := t.TempDir()
		_ = os.MkdirAll(filepath.Join(root, "pkg", "sub"), 0755)
		_ = os.WriteFile(filepath.Join(root, "pkg", "sub", "util.go"), []byte("package sub\n"), 0644)
		_ = os.WriteFile(filepath.Join(root, "pkg", "sub", "util_test.go"), []byte("package sub\n"), 0644)
		_ = os.WriteFile(filepath.Join(root, "pkg", "sub", "data.json"), []byte("{}"), 0644)

		ctxExt := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_list_extensions": ".go",
			"_skip_dir_names":  "",
		})
		h2 := localtools.NewLocalFSTools(root, nil)
		res, _, err := h2.Exec(ctxExt, now, map[string]any{
			"path":      "pkg",
			"recursive": true,
			"max_depth": float64(3),
		}, false, &taskengine.ToolsCall{ToolName: "list_dir"})
		if err != nil {
			t.Fatal(err)
		}
		listing := res.(string)
		if !strings.Contains(listing, "util.go") {
			t.Errorf("util.go must appear in recursive .go filter, got:\n%s", listing)
		}
		if !strings.Contains(listing, "util_test.go") {
			t.Errorf("util_test.go must appear in recursive .go filter, got:\n%s", listing)
		}
		if strings.Contains(listing, "data.json") {
			t.Errorf("data.json must be hidden by .go extension filter, got:\n%s", listing)
		}
	})
}

func TestUnit_LocalFSTools_InjectedNamePlumbsThrough(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalFSToolsWith(t.TempDir(), nil, nil, "scoped_fs", nil)

	names, err := h.Supports(ctx)
	if err != nil {
		t.Fatalf("Supports: %v", err)
	}
	if len(names) == 0 || names[0] != "scoped_fs" {
		t.Fatalf("Supports must lead with the injected name, got %v", names)
	}

	tools, err := h.GetToolsForToolsByName(ctx, "scoped_fs")
	if err != nil {
		t.Fatalf("GetToolsForToolsByName(scoped_fs): %v", err)
	}
	want := map[string]bool{
		"read_file": true, "write_file": true, "list_dir": true, "grep": true,
		"find_files": true, "search_repo": true,
		"sed": true, "count_stats": true, "read_file_range": true, "stat_file": true,
	}
	if len(tools) != len(want) {
		t.Fatalf("expected %d tools, got %d", len(want), len(tools))
	}
	for _, tl := range tools {
		if !want[tl.Function.Name] {
			t.Fatalf("unexpected tool %q advertised under scoped_fs", tl.Function.Name)
		}
	}

	if _, err := h.GetToolsForToolsByName(ctx, "local_fs"); err == nil {
		t.Fatalf("a renamed instance must not answer to the old name local_fs")
	}
}

func writeSearchRepoFixture(t *testing.T, root, rel, content string) {
	t.Helper()

	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func execSearchRepoForTest(t *testing.T, ctx context.Context, h taskengine.ToolsRepo, args map[string]any) (string, taskengine.DataType, error) {
	t.Helper()

	res, dataType, err := h.Exec(ctx, time.Now(), args, false, &taskengine.ToolsCall{ToolName: "search_repo"})
	if err != nil {
		return "", dataType, err
	}

	s, ok := res.(string)
	if !ok {
		t.Fatalf("expected search_repo string result, got %T: %v", res, res)
	}
	return s, dataType, nil
}

func TestUnit_LocalFSTools_SearchRepo(t *testing.T) {
	ctx := context.Background()

	t.Run("patternRequired", func(t *testing.T) {
		root := t.TempDir()
		h := localtools.NewLocalFSTools(root, nil)

		_, _, err := h.Exec(ctx, time.Now(), map[string]any{"path": "."}, false, &taskengine.ToolsCall{ToolName: "search_repo"})
		if err == nil {
			t.Fatal("expected pattern required error")
		}
		if !strings.Contains(err.Error(), "pattern required") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("literalSearchAcrossFiles", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "cmd/main.go", "package main\n// needle in main\n")
		writeSearchRepoFixture(t, root, "docs/readme.md", "# docs\nneedle in docs\n")
		writeSearchRepoFixture(t, root, "other.txt", "nothing here\n")

		h := localtools.NewLocalFSTools(root, nil)
		out, dataType, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err != nil {
			t.Fatal(err)
		}
		if dataType != taskengine.DataTypeString {
			t.Fatalf("unexpected data type: %v", dataType)
		}
		if !strings.Contains(out, "cmd/main.go:2: // needle in main") {
			t.Fatalf("expected cmd/main.go match, got:\n%s", out)
		}
		if !strings.Contains(out, "docs/readme.md:2: needle in docs") {
			t.Fatalf("expected docs/readme.md match, got:\n%s", out)
		}
		if strings.Contains(out, "other.txt") {
			t.Fatalf("did not expect other.txt match, got:\n%s", out)
		}
	})

	t.Run("noMatches", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "main.go", "package main\n")

		h := localtools.NewLocalFSTools(root, nil)
		out, dataType, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err != nil {
			t.Fatal(err)
		}
		if dataType != taskengine.DataTypeString {
			t.Fatalf("unexpected data type: %v", dataType)
		}
		if out != "No matches found." {
			t.Fatalf("unexpected no-match output: %q", out)
		}
	})

	t.Run("regexSearch", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "users.txt", "user-x\nuser-42\nadmin-99\n")

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": `^user-[0-9]+$`,
			"regex":   true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "users.txt:2: user-42") {
			t.Fatalf("expected regex match for user-42, got:\n%s", out)
		}
		if strings.Contains(out, "user-x") || strings.Contains(out, "admin-99") {
			t.Fatalf("unexpected regex match, got:\n%s", out)
		}
	})

	t.Run("invalidRegex", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "main.go", "package main\n")

		h := localtools.NewLocalFSTools(root, nil)
		_, _, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": "(",
			"regex":   true,
		})
		if err == nil {
			t.Fatal("expected invalid regex error")
		}
		if !strings.Contains(err.Error(), "invalid regex") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("globFiltersByBasename", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "src/app.go", "package src\n// needle go\n")
		writeSearchRepoFixture(t, root, "src/app.txt", "needle txt\n")
		writeSearchRepoFixture(t, root, "README.md", "needle md\n")

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
			"glob":    "*.go",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "src/app.go") {
			t.Fatalf("expected .go match, got:\n%s", out)
		}
		if strings.Contains(out, "src/app.txt") || strings.Contains(out, "README.md") {
			t.Fatalf("glob should only include basename-matching .go files, got:\n%s", out)
		}
	})

	t.Run("defaultSkipsNoiseDirs", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, ".git/HEAD", "needle from git\n")
		writeSearchRepoFixture(t, root, "node_modules/pkg/index.js", "needle from node_modules\n")
		writeSearchRepoFixture(t, root, ".venv/lib/site.py", "needle from venv\n")
		writeSearchRepoFixture(t, root, "src/app.go", "package src\n// needle from src\n")

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "src/app.go") {
			t.Fatalf("expected src match, got:\n%s", out)
		}
		if strings.Contains(out, ".git") || strings.Contains(out, "node_modules") || strings.Contains(out, ".venv") {
			t.Fatalf("default search_repo must skip noise dirs, got:\n%s", out)
		}
	})

	t.Run("skipDirsCanBeDisabled", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, ".git/HEAD", "needle from git\n")

		ctxDisabled := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_skip_dir_names": "",
		})

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctxDisabled, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, ".git/HEAD") {
			t.Fatalf("expected disabled skip filter to include .git/HEAD, got:\n%s", out)
		}
	})

	t.Run("binaryNULFilesAreSkipped", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "plain.txt", "needle in text\n")

		if err := os.WriteFile(filepath.Join(root, "blob.bin"), []byte("needle\x00hidden\n"), 0644); err != nil {
			t.Fatal(err)
		}

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "plain.txt") {
			t.Fatalf("expected plain text match, got:\n%s", out)
		}
		if strings.Contains(out, "blob.bin") {
			t.Fatalf("binary file with NUL byte must be skipped, got:\n%s", out)
		}
	})

	t.Run("longMatchingLinesAreTruncated", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "long.txt", "needle "+strings.Repeat("a", 600)+"\n")

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "long.txt:1: needle ") {
			t.Fatalf("expected long line match, got:\n%s", out)
		}
		if !strings.Contains(out, "…") {
			t.Fatalf("expected long line truncation ellipsis, got:\n%s", out)
		}
		if strings.Contains(out, strings.Repeat("a", 510)) {
			t.Fatalf("long line was not truncated enough, got:\n%s", out)
		}
	})

	t.Run("maxMatchesTruncatesInsteadOfErroring", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "many.txt", "needle 1\nneedle 2\nneedle 3\n")

		ctxLimit := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_max_grep_matches": "2",
		})

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctxLimit, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "many.txt:1: needle 1") || !strings.Contains(out, "many.txt:2: needle 2") {
			t.Fatalf("expected first two matches, got:\n%s", out)
		}
		if strings.Contains(out, "many.txt:3: needle 3") {
			t.Fatalf("expected third match to be truncated, got:\n%s", out)
		}
		if !strings.Contains(out, "Truncated at 2 matches") {
			t.Fatalf("expected truncation notice, got:\n%s", out)
		}
	})

	t.Run("maxOutputBytesRejectsOversizedSearchResult", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "many.txt", strings.Repeat("needle line with padding\n", 20))

		ctxSmallOut := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_max_output_bytes": "80",
		})

		h := localtools.NewLocalFSTools(root, nil)
		_, _, err := execSearchRepoForTest(t, ctxSmallOut, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err == nil {
			t.Fatal("expected _max_output_bytes error")
		}
		if !strings.Contains(err.Error(), "search_repo output") || !strings.Contains(err.Error(), "max") {
			t.Fatalf("expected search_repo output limit hint, got: %v", err)
		}
	})
}

func TestUnit_LocalFSTools_SearchRepoGuardrails(t *testing.T) {
	ctx := context.Background()

	t.Run("honorsDeniedPathSubstringsDuringWalk", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "public/app.txt", "needle public\n")
		writeSearchRepoFixture(t, root, "secret/keys.txt", "needle TOPSECRET\n")

		ctxDeny := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_denied_path_substrings": "secret",
			"_skip_dir_names":         "",
		})

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctxDeny, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "public/app.txt") {
			t.Fatalf("expected public match to remain, got:\n%s", out)
		}
		if strings.Contains(out, "secret/keys.txt") || strings.Contains(out, "TOPSECRET") {
			t.Fatalf("search_repo must not return denied-path matches, got:\n%s", out)
		}
	})

	t.Run("doesNotFollowSymlinkToFileOutsideAllowedDir", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()

		writeSearchRepoFixture(t, root, "public/app.txt", "needle public\n")

		outsideSecret := filepath.Join(outside, "secret.txt")
		if err := os.WriteFile(outsideSecret, []byte("external needle secret\n"), 0644); err != nil {
			t.Fatal(err)
		}

		linkPath := filepath.Join(root, "linked_secret.txt")
		if err := os.Symlink(outsideSecret, linkPath); err != nil {
			t.Skipf("symlink unavailable on this platform: %v", err)
		}

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "public/app.txt") {
			t.Fatalf("expected public match to remain, got:\n%s", out)
		}
		if strings.Contains(out, "linked_secret.txt") || strings.Contains(out, "external needle secret") {
			t.Fatalf("search_repo must not follow symlinked files outside allowed dir, got:\n%s", out)
		}
	})

	t.Run("respectsMaxReadBytesForCandidateFiles", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "oversized.txt", "needle "+strings.Repeat("x", 1024)+"\n")

		ctxSmallRead := taskengine.WithToolsArgs(ctx, localtools.LocalFSToolsName, map[string]string{
			"_max_read_bytes":   "64",
			"_max_output_bytes": "-1",
			"_skip_dir_names":   "",
		})

		h := localtools.NewLocalFSTools(root, nil)
		_, _, err := execSearchRepoForTest(t, ctxSmallRead, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
		})
		if err == nil {
			t.Fatal("expected search_repo to reject an oversized candidate file before reading it")
		}
		if !strings.Contains(err.Error(), "max") {
			t.Fatalf("expected max read size hint, got: %v", err)
		}
	})

	t.Run("globCanMatchRelativePaths", func(t *testing.T) {
		root := t.TempDir()
		writeSearchRepoFixture(t, root, "pkg/app.go", "package pkg\n// needle pkg\n")
		writeSearchRepoFixture(t, root, "cmd/app.go", "package main\n// needle cmd\n")
		writeSearchRepoFixture(t, root, "pkg/readme.md", "needle md\n")

		h := localtools.NewLocalFSTools(root, nil)
		out, _, err := execSearchRepoForTest(t, ctx, h, map[string]any{
			"path":    ".",
			"pattern": "needle",
			"glob":    "pkg/*.go",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "pkg/app.go") {
			t.Fatalf("expected path-shaped glob to include pkg/app.go, got:\n%s", out)
		}
		if strings.Contains(out, "cmd/app.go") || strings.Contains(out, "pkg/readme.md") {
			t.Fatalf("path-shaped glob must only include pkg/*.go, got:\n%s", out)
		}
	})
}
