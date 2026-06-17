// Package git wraps the git CLI for the small set of operations SprintMate
// needs: inspecting the current branch and recent commits, and creating or
// reusing a feature branch for an issue.
package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Available reports whether the git binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(ctx context.Context, dir string) bool {
	out, err := run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// CurrentBranch returns the checked-out branch name.
func CurrentBranch(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}

// RecentCommits returns up to n recent commits as "<short-hash> <subject>".
func RecentCommits(ctx context.Context, dir string, n int) ([]string, error) {
	out, err := run(ctx, dir, "log", fmt.Sprintf("-n%d", n), "--pretty=format:%h %s")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// CreateOrReuseBranch checks out branch, creating it if it does not exist.
// It returns whether the branch was newly created.
func CreateOrReuseBranch(ctx context.Context, dir, branch string) (created bool, err error) {
	if branchExists(ctx, dir, branch) {
		_, err = run(ctx, dir, "checkout", branch)
		return false, err
	}
	if _, err = run(ctx, dir, "checkout", "-b", branch); err != nil {
		return false, err
	}
	return true, nil
}

func branchExists(ctx context.Context, dir, branch string) bool {
	_, err := run(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	// Never block on an interactive credential prompt; fail fast instead.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
