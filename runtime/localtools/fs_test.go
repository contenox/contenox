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
		if res != "ok" {
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
