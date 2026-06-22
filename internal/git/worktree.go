package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Worktree is a linked working tree of a repository, as reported by
// `git worktree list`.
type Worktree struct {
	Path   string // absolute path to the worktree
	Branch string // short branch name, or "" when detached
	Head   string // commit hash currently checked out
}

// WorktreeAdd creates a linked worktree at dir for branch, isolating an issue's
// agent in its own directory so parallel agents never touch the same files.
//
// It is idempotent: if a worktree is already registered at dir it returns nil
// (reuse). The branch is created from HEAD when it does not yet exist, or
// checked out into the new worktree when it does.
func WorktreeAdd(ctx context.Context, repoDir, dir, branch string) error {
	if hasWorktreeAt(ctx, repoDir, dir) {
		return nil // reuse the existing worktree
	}
	// git worktree add creates the leaf directory but not arbitrary parents.
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("creating worktree base: %w", err)
	}
	args := []string{"worktree", "add"}
	if branchExists(ctx, repoDir, branch) {
		args = append(args, dir, branch)
	} else {
		args = append(args, "-b", branch, dir)
	}
	_, err := run(ctx, repoDir, args...)
	return err
}

// WorktreeList returns the repository's linked worktrees (including the main
// one), parsed from `git worktree list --porcelain`.
func WorktreeList(ctx context.Context, repoDir string) ([]Worktree, error) {
	out, err := run(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []Worktree
	var cur Worktree
	flush := func() {
		if cur.Path != "" {
			list = append(list, cur)
		}
		cur = Worktree{}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "":
			flush()
		}
	}
	flush()
	return list, nil
}

// WorktreeRemove removes the worktree at dir. force drops it even if it has
// uncommitted changes (used when the user discards an autonomous run).
func WorktreeRemove(ctx context.Context, repoDir, dir string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, dir)
	_, err := run(ctx, repoDir, args...)
	return err
}

// WorktreePath returns the per-issue worktree directory under base, named after
// a filesystem-safe slug of the issue key.
func WorktreePath(base, key string) string {
	return filepath.Join(base, Slugify(key))
}

func hasWorktreeAt(ctx context.Context, repoDir, dir string) bool {
	list, err := WorktreeList(ctx, repoDir)
	if err != nil {
		return false
	}
	target := resolvePath(dir)
	for _, w := range list {
		if resolvePath(w.Path) == target {
			return true
		}
	}
	return false
}

// resolvePath canonicalizes p, following symlinks when the path exists (git
// reports real paths, so a /var → /private/var symlink on macOS must not make a
// worktree look new). Falls back to a lexical clean for not-yet-existing paths.
func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}
