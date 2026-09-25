package agentdecl

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"

	"github.com/contenox/contenox/internal/services/vfs"
)

//go:embed preseed/reviewer.md
var preseedReviewer string

//go:embed preseed/researcher.md
var preseedResearcher string

//go:embed preseed/README.md
var preseedReadme string

//go:embed preseed/run.md
var preseedRun string

//go:embed preseed/chat.md
var preseedChat string

// preseedTrees is the worked example of the directory convention: a router and
// the branches it chooses between.
//
//go:embed preseed/agents
var preseedTrees embed.FS

// Preseeded are the files that establish the authoring convention.
var Preseeded = []struct {
	// RelPath is relative to the contenox directory.
	RelPath string
	// Source is the file in this package that ships the content.
	Source  string
	Content func() string
}{
	{ConfigFilename, "agents.toml", func() string { return string(shippedConfig) }},
	{path.Join(NativeSourceDir, "README.md"), "preseed/README.md", func() string { return preseedReadme }},
	{path.Join(NativeSourceDir, "reviewer.md"), "preseed/reviewer.md", func() string { return preseedReviewer }},
	{path.Join(NativeSourceDir, "researcher.md"), "preseed/researcher.md", func() string { return preseedResearcher }},
	{path.Join(NativeSourceDir, "run.md"), "preseed/run.md", func() string { return preseedRun }},
	{path.Join(NativeSourceDir, "chat.md"), "preseed/chat.md", func() string { return preseedChat }},
}

// PreseedSource is one shipped declaration: where seeding writes it under a
// contenox directory, and the file in this package that holds the content.
type PreseedSource struct {
	Rel    string
	Source string
}

// PreseedSources reports every shipped declaration and its source file. The
// blessed-hash table is generated from those files' history, so the mapping has
// to be the one seeding reads.
func PreseedSources() ([]PreseedSource, error) {
	files, err := preseedFiles()
	if err != nil {
		return nil, err
	}
	sources := make([]PreseedSource, 0, len(files))
	for _, f := range files {
		sources = append(sources, PreseedSource{Rel: f.rel, Source: f.source})
	}
	return sources, nil
}

// PreseedStateFilename records the shipped bytes each seeded declaration came
// from. It sits beside the compiled chains: a copy whose bytes still match what
// this file recorded writing is ours to refresh, and anything else is the
// operator's.
const PreseedStateFilename = ".preseed-state.json"

// PreseedResult is what one seeding pass did with the shipped declarations.
type PreseedResult struct {
	// Created are paths written because the directory held nothing there.
	Created []string
	// Updated are paths replaced with shipped content. Only a copy recognised as
	// ours is refreshed — the bytes this mechanism recorded writing, or a version
	// a release shipped, which is what keeps an operator's edit from being
	// overwritten by a later release.
	Updated []string
	// Edited are paths this mechanism wrote and the operator then changed. Left
	// alone and still recorded, so one restored to its shipped bytes is
	// refreshable again.
	Edited []string
	// Unrecorded are paths that differ from every shipped version and carry no
	// record: a declaration the operator authored. Reported the first time they
	// are seen, then kept without further notice, because nothing about them
	// changes until the operator edits them again.
	Unrecorded []string
}

type preseedRecord struct {
	SHA256 string `json:"sha256,omitempty"`
	// Operator marks a file that is not a shipped copy: seeding wrote it once, or
	// never wrote it at all, and reported it. Recorded so the report is made once
	// rather than on every pass.
	Operator bool `json:"operator,omitempty"`
}

type preseedFile struct {
	rel     string
	source  string
	content []byte
}

// Preseed writes the authoring convention into the root, leaving the operator's
// own files alone, and refreshes a shipped copy a later release changed. The
// defaults file is written verbatim so its comments survive: they are what makes
// the knobs tunable.
func Preseed(ctx context.Context, contenoxDir Root) (PreseedResult, error) {
	var result PreseedResult
	if contenoxDir.FS == nil {
		return result, nil
	}
	files, err := preseedFiles()
	if err != nil {
		return result, err
	}
	state := readPreseedState(ctx, contenoxDir.FS)
	next := make(map[string]preseedRecord, len(files))
	for _, f := range files {
		shipped := hashBytes(f.content)
		display := contenoxDir.Path(f.rel)
		onDisk, readErr := contenoxDir.FS.ReadFile(ctx, f.rel)
		record, recorded := state[f.rel]
		hash := hashBytes(onDisk)
		switch {
		case errors.Is(readErr, fs.ErrNotExist):
			if err := writePreseedFile(ctx, contenoxDir, f.rel, f.content); err != nil {
				return result, err
			}
			result.Created = append(result.Created, display)
		case readErr != nil:
			return result, fmt.Errorf("agentdecl: read %s: %w", display, readErr)
		case hash == shipped:
		case blessedPreseedHash(f.rel, hash):
			if err := writePreseedFile(ctx, contenoxDir, f.rel, f.content); err != nil {
				return result, err
			}
			result.Updated = append(result.Updated, display)
		case recorded && record.SHA256 == hash:
			if err := writePreseedFile(ctx, contenoxDir, f.rel, f.content); err != nil {
				return result, err
			}
			result.Updated = append(result.Updated, display)
		case recorded && !record.Operator:
			result.Edited = append(result.Edited, display)
			next[f.rel] = record
			continue
		case recorded:
			next[f.rel] = record
			continue
		default:
			result.Unrecorded = append(result.Unrecorded, display)
			next[f.rel] = preseedRecord{Operator: true}
			continue
		}
		next[f.rel] = preseedRecord{SHA256: shipped}
	}
	if err := writePreseedState(ctx, contenoxDir.FS, state, next); err != nil {
		return result, err
	}
	return result, nil
}

// preseedFiles is every shipped declaration, flat entries and tree example
// alike, in a stable order so a pass over them reports the same way twice.
func preseedFiles() ([]preseedFile, error) {
	files := make([]preseedFile, 0, len(Preseeded))
	for _, f := range Preseeded {
		files = append(files, preseedFile{rel: f.RelPath, source: f.Source, content: []byte(f.Content())})
	}
	err := fs.WalkDir(preseedTrees, "preseed/agents", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		rel, relErr := filepath.Rel("preseed/agents", p)
		if relErr != nil {
			return relErr
		}
		data, rErr := preseedTrees.ReadFile(p)
		if rErr != nil {
			return rErr
		}
		files = append(files, preseedFile{
			rel:     path.Join(NativeSourceDir, filepath.ToSlash(rel)),
			source:  path.Join("preseed", "agents", filepath.ToSlash(rel)),
			content: data,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	return files, nil
}

func writePreseedFile(ctx context.Context, contenoxDir Root, rel string, content []byte) error {
	if err := contenoxDir.FS.MkdirAll(ctx, path.Dir(rel)); err != nil {
		return fmt.Errorf("agentdecl: create %s: %w", contenoxDir.Path(path.Dir(rel)), err)
	}
	if err := contenoxDir.FS.WriteFile(ctx, rel, content); err != nil {
		return fmt.Errorf("agentdecl: write %s: %w", contenoxDir.Path(rel), err)
	}
	return nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// blessedPreseedHash reports whether data is a version this tree shipped for
// that declaration. Seeding refreshes such a copy: the bytes came from a release,
// so the file is ours to carry forward, however old the install is.
func blessedPreseedHash(rel, hash string) bool {
	for _, blessed := range blessedPreseedHashes[rel] {
		if blessed == hash {
			return true
		}
	}
	return false
}

// preseedStatePath is where the record lives: generated output, like the chains
// beside it, because it describes what this tool wrote rather than what the
// operator declared.
func preseedStatePath() string {
	return path.Join(GeneratedDirName, PreseedStateFilename)
}

func readPreseedState(ctx context.Context, fsys vfs.Files) map[string]preseedRecord {
	state := map[string]preseedRecord{}
	raw, err := fsys.ReadFile(ctx, preseedStatePath())
	if err != nil {
		return state
	}
	_ = json.Unmarshal(raw, &state)
	return state
}

// writePreseedState writes the record only when it changed, so a pass that did
// nothing leaves the file — and its modification time — alone.
func writePreseedState(ctx context.Context, fsys vfs.Files, previous, next map[string]preseedRecord) error {
	before, err := json.MarshalIndent(previous, "", "  ")
	if err != nil {
		return fmt.Errorf("agentdecl: marshal preseed state: %w", err)
	}
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("agentdecl: marshal preseed state: %w", err)
	}
	if string(before) == string(raw) {
		return nil
	}
	if err := fsys.MkdirAll(ctx, GeneratedDirName); err != nil {
		return fmt.Errorf("agentdecl: create %s: %w", GeneratedDirName, err)
	}
	if err := fsys.WriteFile(ctx, preseedStatePath(), raw); err != nil {
		return fmt.Errorf("agentdecl: write preseed state: %w", err)
	}
	return nil
}
