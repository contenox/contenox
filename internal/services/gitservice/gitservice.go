// Package gitservice runs git operations against one served workspace root,
// in-process through go-git and contained so a path can never escape the
// workspace. It is the human-facing counterpart of the model's native-git
// toolset: plain typed results and errors, no severity markers, no tool
// dispatch — the HTTP surface (and later the toolset) adapt over it.
package gitservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ErrNotRepo reports a workspace that is not inside a git repository.
var ErrNotRepo = errors.New("gitservice: workspace is not in a git repository")

// Service runs git operations against one workspace root.
type Service struct {
	root string // absolute, clean
}

// New returns a Service for root. It never fails: the repository is located and
// opened per call, so a workspace outside version control reports ErrNotRepo on
// the first operation rather than refusing to construct.
func New(root string) *Service {
	return &Service{root: filepath.Clean(root)}
}

// Root returns the served workspace root.
func (s *Service) Root() string { return s.root }

// CommitRef is a commit reduced to what a status line shows.
type CommitRef struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

// StatusEntry is one path with the single-letter git status code that applies
// to it — 'M', 'A', 'D', 'R', '?', the codes `git status --short` prints.
type StatusEntry struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

// Status is the repository's working-tree state, filtered to the workspace.
type Status struct {
	Branch    string        `json:"branch"`
	Head      *CommitRef    `json:"head,omitempty"`
	Clean     bool          `json:"clean"`
	Files     []StatusEntry `json:"files"`
	Staged    []StatusEntry `json:"staged"`
	Unstaged  []StatusEntry `json:"unstaged"`
	Untracked []string      `json:"untracked"`
}

// FileDiff is one file's HEAD-vs-working-tree difference, shaped for a diff
// editor: the old and new text, plus whether either side is binary or the file
// is untracked (no HEAD side).
type FileDiff struct {
	Path   string `json:"path"`
	Old    string `json:"old"`
	New    string `json:"new"`
	Binary bool   `json:"binary,omitempty"`
	IsNew  bool   `json:"isNew,omitempty"`
}

// BlameLine is one line's authorship at HEAD.
type BlameLine struct {
	Line   int    `json:"line"`
	Hash   string `json:"hash"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

// Blame is a file's per-line authorship at HEAD.
type Blame struct {
	Lines []BlameLine `json:"lines"`
}

// Commit is the outcome of committing the staging area.
type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	Branch  string `json:"branch"`
	Files   int    `json:"files"`
}

// Status returns the workspace's git status. Paths are relative to the
// workspace root.
func (s *Service) Status(ctx context.Context) (*Status, error) {
	repo, repoRoot, err := s.openRepo()
	if err != nil {
		return nil, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("gitservice: status: %w", err)
	}
	st, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("gitservice: status: %w", err)
	}

	out := &Status{
		Branch:    currentBranch(repo),
		Clean:     true,
		Files:     []StatusEntry{},
		Staged:    []StatusEntry{},
		Unstaged:  []StatusEntry{},
		Untracked: []string{},
	}
	if commit, ok, err := headCommit(repo); err == nil && ok {
		out.Head = &CommitRef{Hash: shortHash(commit.Hash), Subject: subjectOf(commit.Message)}
	}

	for _, path := range sortedStatusPaths(st) {
		wsRel, inWs := s.workspaceRelOf(repoRoot, path)
		if !inWs {
			continue
		}
		fs := st[path]
		if fs.Staging == git.Untracked && fs.Worktree == git.Untracked {
			out.Untracked = append(out.Untracked, wsRel)
			out.Files = append(out.Files, StatusEntry{Path: wsRel, Code: "?"})
			out.Clean = false
			continue
		}
		if fs.Staging != git.Unmodified && fs.Staging != git.Untracked {
			out.Staged = append(out.Staged, StatusEntry{Path: wsRel, Code: string(fs.Staging)})
			out.Clean = false
		}
		if fs.Worktree != git.Unmodified && fs.Worktree != git.Untracked {
			out.Unstaged = append(out.Unstaged, StatusEntry{Path: wsRel, Code: string(fs.Worktree)})
			out.Clean = false
		}
		code := fs.Worktree
		if code == git.Unmodified || code == git.Untracked {
			code = fs.Staging
		}
		if code != git.Unmodified && code != git.Untracked {
			out.Files = append(out.Files, StatusEntry{Path: wsRel, Code: string(code)})
		}
	}
	return out, nil
}

// Diff returns the HEAD-vs-working-tree content of one workspace file.
func (s *Service) Diff(ctx context.Context, path string) (*FileDiff, error) {
	repo, repoRoot, err := s.openRepo()
	if err != nil {
		return nil, err
	}
	rel, err := s.repoRelOf(repoRoot, path)
	if err != nil {
		return nil, err
	}
	if rel == "" || rel == "." {
		return nil, errors.New("gitservice: diff requires a file path, not the repository root")
	}

	oldContent := ""
	isNew := true
	if commit, ok, err := headCommit(repo); err != nil {
		return nil, err
	} else if ok {
		if f, ferr := commit.File(rel); ferr == nil {
			if c, cerr := f.Contents(); cerr == nil {
				oldContent = c
				isNew = false
			}
		}
	}

	newContent := ""
	if data, rerr := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel))); rerr == nil {
		newContent = string(data)
	}
	return &FileDiff{
		Path:   path,
		Old:    oldContent,
		New:    newContent,
		Binary: isBinary([]byte(oldContent)) || isBinary([]byte(newContent)),
		IsNew:  isNew,
	}, nil
}

// Blame returns per-line authorship of one tracked workspace file at HEAD.
func (s *Service) Blame(ctx context.Context, path string) (*Blame, error) {
	repo, repoRoot, err := s.openRepo()
	if err != nil {
		return nil, err
	}
	rel, err := s.repoRelOf(repoRoot, path)
	if err != nil {
		return nil, err
	}
	if rel == "" || rel == "." {
		return nil, errors.New("gitservice: blame requires a file path, not the repository root")
	}
	commit, ok, err := headCommit(repo)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("gitservice: the repository has no commits yet")
	}
	result, err := git.Blame(commit, rel)
	if err != nil {
		return nil, fmt.Errorf("gitservice: cannot blame %s: %w", rel, err)
	}
	lines := make([]BlameLine, 0, len(result.Lines))
	for i, line := range result.Lines {
		lines = append(lines, BlameLine{
			Line:   i + 1,
			Hash:   shortHash(line.Hash),
			Author: line.AuthorName,
			Text:   line.Text,
		})
	}
	return &Blame{Lines: lines}, nil
}

// Stage stages the given workspace paths for the next commit. An empty path
// entry means "stage everything".
func (s *Service) Stage(ctx context.Context, paths []string) error {
	repo, repoRoot, err := s.openRepo()
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("gitservice: stage: %w", err)
	}
	rels, err := s.repoRelPaths(repoRoot, paths)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		opts := &git.AddOptions{Path: rel}
		if rel == "" {
			opts = &git.AddOptions{All: true}
		}
		if err := wt.AddWithOptions(opts); err != nil {
			return fmt.Errorf("gitservice: cannot stage %s: %w", displayRel(rel), err)
		}
	}
	return nil
}

// Unstage moves the given paths out of the staging area, leaving the file
// contents alone.
func (s *Service) Unstage(ctx context.Context, paths []string) error {
	repo, repoRoot, err := s.openRepo()
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("gitservice: unstage: %w", err)
	}
	rels, err := s.repoRelPaths(repoRoot, paths)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		if rel == "" {
			return errors.New("gitservice: name the files to unstage — unstaging the whole repository at once is refused")
		}
	}
	if err := wt.Restore(&git.RestoreOptions{Files: rels, Staged: true, Worktree: false}); err != nil {
		return fmt.Errorf("gitservice: cannot unstage: %w", err)
	}
	return nil
}

// Restore discards uncommitted changes to the given paths, back to HEAD.
func (s *Service) Restore(ctx context.Context, paths []string) error {
	repo, repoRoot, err := s.openRepo()
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("gitservice: restore: %w", err)
	}
	rels, err := s.repoRelPaths(repoRoot, paths)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		if rel == "" {
			return errors.New("gitservice: name the files to restore — restoring the whole repository at once is refused")
		}
	}
	if err := wt.Restore(&git.RestoreOptions{Files: rels, Staged: true, Worktree: true}); err != nil {
		return fmt.Errorf("gitservice: cannot restore: %w", err)
	}
	return nil
}

// Commit commits the staging area with the given message. The author comes from
// the repository's own git config.
func (s *Service) Commit(ctx context.Context, message string) (*Commit, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, errors.New("gitservice: commit message is required")
	}
	repo, _, err := s.openRepo()
	if err != nil {
		return nil, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("gitservice: commit: %w", err)
	}
	st, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("gitservice: commit: %w", err)
	}
	staged := 0
	for _, path := range sortedStatusPaths(st) {
		if fs := st[path]; fs.Staging != git.Unmodified && fs.Staging != git.Untracked {
			staged++
		}
	}
	if staged == 0 {
		return nil, errors.New("gitservice: nothing is staged to commit")
	}
	hash, err := wt.Commit(message, &git.CommitOptions{})
	if err != nil {
		if errors.Is(err, git.ErrMissingAuthor) {
			return nil, errors.New("gitservice: this repository has no commit identity — set user.name and user.email in its git config first")
		}
		return nil, fmt.Errorf("gitservice: commit: %w", err)
	}
	return &Commit{
		Hash:    shortHash(hash),
		Subject: subjectOf(message),
		Branch:  currentBranch(repo),
		Files:   staged,
	}, nil
}

// openRepo locates and opens the repository containing the workspace root.
func (s *Service) openRepo() (*git.Repository, string, error) {
	repoRoot, found := findRepoRoot(s.root)
	if !found {
		return nil, "", ErrNotRepo
	}
	repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		return nil, "", fmt.Errorf("gitservice: open repository at %s: %w", repoRoot, err)
	}
	return repo, repoRoot, nil
}

// repoRelOf resolves a workspace path (relative to the workspace root, or an
// absolute path inside it) to a repo-relative slash path.
func (s *Service) repoRelOf(repoRoot, raw string) (string, error) {
	abs, err := s.workspaceAbs(raw)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil {
		return "", fmt.Errorf("gitservice: path %q is outside the repository", raw)
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("gitservice: path %q is outside the repository", raw)
	}
	return rel, nil
}

// repoRelPaths resolves a list of workspace paths to repo-relative paths.
func (s *Service) repoRelPaths(repoRoot string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New("gitservice: paths is required")
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel, err := s.repoRelOf(repoRoot, p)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, nil
}

// workspaceAbs resolves a client path to an absolute path contained by the
// workspace root.
func (s *Service) workspaceAbs(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("gitservice: path is required")
	}
	p := raw
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.root, p)
	}
	p = filepath.Clean(p)
	if p != s.root && !strings.HasPrefix(p, s.root+string(filepath.Separator)) {
		return "", fmt.Errorf("gitservice: path %q escapes the workspace", raw)
	}
	return p, nil
}

// workspaceRelOf maps a repo-relative path to a workspace-relative one,
// reporting false for a path outside the workspace.
func (s *Service) workspaceRelOf(repoRoot, repoRel string) (string, bool) {
	wsRel, err := filepath.Rel(repoRoot, s.root)
	if err != nil {
		return "", false
	}
	if wsRel == "." {
		return repoRel, true
	}
	wsRel = filepath.ToSlash(wsRel)
	if repoRel == wsRel {
		return "", true
	}
	prefix := wsRel + "/"
	if strings.HasPrefix(repoRel, prefix) {
		return strings.TrimPrefix(repoRel, prefix), true
	}
	return "", false
}

func findRepoRoot(start string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func headCommit(repo *git.Repository) (*object.Commit, bool, error) {
	ref, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, false, err
	}
	return commit, true, nil
}

func shortHash(h plumbing.Hash) string {
	s := h.String()
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func subjectOf(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return strings.TrimSpace(msg)
}

func currentBranch(repo *git.Repository) string {
	ref, err := repo.Head()
	if err != nil {
		return "(no commits yet)"
	}
	if ref.Name().IsBranch() {
		return ref.Name().Short()
	}
	return "(detached at " + shortHash(ref.Hash()) + ")"
}

func sortedStatusPaths(st git.Status) []string {
	paths := make([]string, 0, len(st))
	for p := range st {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func displayRel(rel string) string {
	if rel == "" {
		return "everything"
	}
	return rel
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}
