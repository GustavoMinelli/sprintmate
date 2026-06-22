// Package forge abstracts the git host (GitHub, GitLab, …) for the one action
// SprintMate needs after an agent finishes: opening a pull request. It is kept
// separate from the issue tracker (internal/tracker) because the forge and the
// tracker are often different systems. The first implementation shells out to
// the GitHub CLI (`gh`); an API-based forge can replace it without touching
// callers.
package forge

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PR is a created pull request.
type PR struct {
	URL string
}

// Params describes the pull request to open.
type Params struct {
	Branch string // head branch (required)
	Base   string // base branch; empty means the repo default
	Title  string
	Body   string
}

// Forge opens pull requests for a working directory's repository.
type Forge interface {
	CreatePR(ctx context.Context, dir string, p Params) (PR, error)
}

// runner runs a command in dir and returns its combined output. It is a seam for
// tests to inject a fake instead of executing a real binary.
type runner func(ctx context.Context, dir, name string, args ...string) (string, error)

// Detect returns a Forge if a supported CLI is available. Today that is the
// GitHub CLI; when it's missing the caller skips PR creation and informs the user.
func Detect() (Forge, bool) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, false
	}
	return gh{run: execRun}, true
}

type gh struct{ run runner }

func (g gh) CreatePR(ctx context.Context, dir string, p Params) (PR, error) {
	if strings.TrimSpace(p.Branch) == "" {
		return PR{}, fmt.Errorf("create PR: head branch is required")
	}
	args := []string{"pr", "create", "--head", p.Branch, "--title", p.Title, "--body", p.Body}
	if strings.TrimSpace(p.Base) != "" {
		args = append(args, "--base", p.Base)
	}
	out, err := g.run(ctx, dir, "gh", args...)
	if err != nil {
		return PR{}, err
	}
	return PR{URL: firstURL(out)}, nil
}

func execRun(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// firstURL returns the first https URL in s (gh prints the PR URL on success),
// falling back to the trimmed output.
func firstURL(s string) string {
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "https://") {
			return strings.TrimSpace(f)
		}
	}
	return strings.TrimSpace(s)
}
