package git

import "context"

// Diff returns the working-tree changes in dir relative to HEAD (staged and
// unstaged) — what an autonomous agent left behind for review.
func Diff(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "diff", "HEAD")
}

// Push pushes branch to origin, setting upstream. Used by the ship action.
func Push(ctx context.Context, dir, branch string) error {
	_, err := run(ctx, dir, "push", "-u", "origin", branch)
	return err
}
