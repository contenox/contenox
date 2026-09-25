package agentdecl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func preseedStateAt(t *testing.T, dir string) map[string]preseedRecord {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, GeneratedDirName, PreseedStateFilename))
	require.NoError(t, err)
	state := map[string]preseedRecord{}
	require.NoError(t, json.Unmarshal(raw, &state))
	return state
}

func writePreseedStateAt(t *testing.T, dir string, state map[string]preseedRecord) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, GeneratedDirName), 0o755))
	raw, err := json.MarshalIndent(state, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, GeneratedDirName, PreseedStateFilename), raw, 0o644))
}

// gitRoot is the checkout holding this test, which every pathspec below is read
// from: git resolves a pathspec against the working directory, and the tests run
// from the package directory.
func gitRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skipf("no git checkout to read history from: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// gitAt runs git against that checkout, failing the test when the history it
// needs is not there to read.
func gitAt(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", gitRoot(t)}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git %s unavailable in this checkout: %v", strings.Join(args, " "), err)
	}
	return out
}

// shippedHistory is every content the given declaration's source file ever held
// in this checkout, as sha256, newest first. It is the independent reading of
// the same history the generator reads, so a stale table fails here.
func shippedHistory(t *testing.T, gitPath string) []string {
	t.Helper()
	commits := strings.Fields(string(gitAt(t, "log", "--all", "--format=%H", "--", gitPath)))
	var hashes []string
	for _, commit := range commits {
		blob := gitAt(t, "show", commit+":"+gitPath)
		sum := sha256.Sum256(blob)
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	return hashes
}

// gitPathFor maps a shipped declaration to its path from the repository root,
// which is what every git pathspec here has to be.
func gitPathFor(t *testing.T, source string) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	rel, err := filepath.Rel(gitRoot(t), dir)
	require.NoError(t, err)
	return filepath.ToSlash(filepath.Join(rel, filepath.FromSlash(source)))
}

// A copy of any version this tree ever shipped is the release's to carry
// forward. Without the table it reads as an operator's file and stays behind
// forever, which is the whole reason the table is generated and committed.
func TestUnit_PreseedBlessedHashesCoverShippedHistory(t *testing.T) {
	sources, err := PreseedSources()
	require.NoError(t, err)
	require.NotEmpty(t, sources)

	blessed := make([]string, 0, len(sources))
	for _, source := range sources {
		blessed = append(blessed, source.Rel)
		history := shippedHistory(t, gitPathFor(t, source.Source))
		require.NotEmpty(t, history, source.Source)
		recorded := map[string]bool{}
		for _, hash := range blessedPreseedHashes[source.Rel] {
			recorded[hash] = true
		}
		for _, hash := range history {
			require.True(t, recorded[hash],
				"%s: history hash %s is missing from the table; regenerate with `go run ./tools/preseed-hashes`", source.Rel, hash)
		}
		shipped, err := os.ReadFile(filepath.FromSlash(source.Source))
		require.NoError(t, err)
		require.Contains(t, blessedPreseedHashes[source.Rel], hashBytes(shipped),
			"%s: what this tree ships now must be blessed, so an install carrying it is recognised", source.Rel)
	}

	table := make([]string, 0, len(blessedPreseedHashes))
	for rel := range blessedPreseedHashes {
		table = append(table, rel)
	}
	sort.Strings(table)
	sort.Strings(blessed)
	require.Equal(t, blessed, table, "the table names a different set of declarations than seeding ships")
}

func TestUnit_Preseed_RefreshesAnOlderShippedVersion(t *testing.T) {
	rel := filepath.Join(NativeSourceDir, "acp", "coding", AgentFilename)
	current, err := os.ReadFile("preseed/agents/acp/coding/agent.md")
	require.NoError(t, err)
	older, err := os.ReadFile("testdata/coding-agent-legacy.md")
	require.NoError(t, err)
	require.NotEqual(t, current, older)
	require.Contains(t, blessedPreseedHashes[filepath.ToSlash(rel)], hashBytes(older))

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, rel), older, 0o644))

	result, err := Preseed(context.Background(), rootOf(t, dir))
	require.NoError(t, err)
	require.Contains(t, result.Updated, filepath.Join(dir, rel))
	require.NotContains(t, result.Unrecorded, filepath.Join(dir, rel))

	onDisk, err := os.ReadFile(filepath.Join(dir, rel))
	require.NoError(t, err)
	require.Equal(t, current, onDisk)
	require.Equal(t, hashBytes(current), preseedStateAt(t, dir)[filepath.ToSlash(rel)].SHA256)
}

// A shipped declaration the operator never edited is the release's to refresh:
// without the record it is indistinguishable from a hand-authored file, which
// is why the record exists.
func TestUnit_Preseed_RefreshesAnUneditedShippedCopy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rel := filepath.Join(NativeSourceDir, "acp", "coding", AgentFilename)
	previous := []byte("---\nname: coding\ndescription: An older shipped copy\n---\n\nOld prompt.\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, rel), previous, 0o644))
	writePreseedStateAt(t, dir, map[string]preseedRecord{
		filepath.ToSlash(rel): {SHA256: hashBytes(previous)},
	})

	result, err := Preseed(context.Background(), rootOf(t, dir))
	require.NoError(t, err)
	require.Contains(t, result.Updated, filepath.Join(dir, rel))

	shipped, err := os.ReadFile(filepath.Join(dir, rel))
	require.NoError(t, err)
	require.NotEqual(t, previous, shipped)
	require.Contains(t, string(shipped), "name:")
	require.NotContains(t, string(shipped), "Old prompt.")

	state := preseedStateAt(t, dir)
	require.Equal(t, hashBytes(shipped), state[filepath.ToSlash(rel)].SHA256)
}

// The edit is the whole point: a copy the operator changed is never a release's
// to replace, and it is not reported as unusable either.
func TestUnit_Preseed_KeepsAnEditedShippedCopy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root := rootOf(t, dir)
	_, err := Preseed(context.Background(), root)
	require.NoError(t, err)

	rel := filepath.Join(NativeSourceDir, "acp", "general", AgentFilename)
	mine := []byte("---\nname: general\ndescription: Mine\n---\n\nMy prompt.\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, rel), mine, 0o644))

	result, err := Preseed(context.Background(), root)
	require.NoError(t, err)
	require.Contains(t, result.Edited, filepath.Join(dir, rel))
	require.NotContains(t, result.Updated, filepath.Join(dir, rel))

	onDisk, err := os.ReadFile(filepath.Join(dir, rel))
	require.NoError(t, err)
	require.Equal(t, mine, onDisk)
}

// A declaration that matches no shipped version is the operator's. It is worth
// saying so once — a release may have moved on without them — and worth not
// saying again on every pass afterwards.
func TestUnit_Preseed_ReportsAnOperatorsFileOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rel := filepath.Join(NativeSourceDir, "reviewer.md")
	authored := []byte("---\nname: reviewer\ndescription: Handwritten\n---\n\nMine.\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, rel), authored, 0o644))

	result, err := Preseed(context.Background(), rootOf(t, dir))
	require.NoError(t, err)
	require.Contains(t, result.Unrecorded, filepath.Join(dir, rel))
	require.NotContains(t, result.Updated, filepath.Join(dir, rel))

	onDisk, err := os.ReadFile(filepath.Join(dir, rel))
	require.NoError(t, err)
	require.Equal(t, authored, onDisk)

	again, err := Preseed(context.Background(), rootOf(t, dir))
	require.NoError(t, err)
	require.Empty(t, again.Unrecorded)
	require.Empty(t, again.Edited)
	require.Empty(t, again.Updated)

	stillThere, err := os.ReadFile(filepath.Join(dir, rel))
	require.NoError(t, err)
	require.Equal(t, authored, stillThere)
}

func TestUnit_Preseed_RecordsEveryShippedDeclaration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root := rootOf(t, dir)
	result, err := Preseed(context.Background(), root)
	require.NoError(t, err)
	require.NotEmpty(t, result.Created)

	files, err := preseedFiles()
	require.NoError(t, err)
	state := preseedStateAt(t, dir)
	require.Len(t, state, len(files))
	for _, f := range files {
		require.Equal(t, hashBytes(f.content), state[f.rel].SHA256, f.rel)
	}
}

// Nothing to say on a pass that finds everything current: no report, and no
// rewrite of the record either.
func TestUnit_Preseed_SecondPassIsSilent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root := rootOf(t, dir)
	_, err := Preseed(context.Background(), root)
	require.NoError(t, err)

	statePath := filepath.Join(dir, GeneratedDirName, PreseedStateFilename)
	before, err := os.Stat(statePath)
	require.NoError(t, err)

	result, err := Preseed(context.Background(), root)
	require.NoError(t, err)
	require.Empty(t, result.Created)
	require.Empty(t, result.Updated)
	require.Empty(t, result.Edited)
	require.Empty(t, result.Unrecorded)

	after, err := os.Stat(statePath)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime())
}
