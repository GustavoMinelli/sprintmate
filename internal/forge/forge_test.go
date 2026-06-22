package forge

import (
	"context"
	"strings"
	"testing"
)

func TestGHCreatePR(t *testing.T) {
	var gotDir string
	var gotArgs []string
	fake := func(ctx context.Context, dir, name string, args ...string) (string, error) {
		gotDir, gotArgs = dir, append([]string{name}, args...)
		return "Creating pull request\nhttps://github.com/acme/repo/pull/42\n", nil
	}
	g := gh{run: fake}

	pr, err := g.CreatePR(context.Background(), "/work/dir", Params{
		Branch: "demo-1-fix", Base: "main", Title: "DEMO-1 Fix", Body: "see issue",
	})
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if pr.URL != "https://github.com/acme/repo/pull/42" {
		t.Errorf("PR URL = %q", pr.URL)
	}
	if gotDir != "/work/dir" {
		t.Errorf("dir = %q", gotDir)
	}
	cmd := strings.Join(gotArgs, " ")
	for _, want := range []string{"gh pr create", "--head demo-1-fix", "--base main", "--title DEMO-1 Fix"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
}

func TestGHCreatePRRequiresBranch(t *testing.T) {
	g := gh{run: func(ctx context.Context, dir, name string, args ...string) (string, error) {
		t.Fatal("runner should not be called without a head branch")
		return "", nil
	}}
	if _, err := g.CreatePR(context.Background(), "/d", Params{Title: "x"}); err == nil {
		t.Error("expected an error when head branch is empty")
	}
}

func TestFirstURL(t *testing.T) {
	if got := firstURL("blah\nhttps://x/pull/1\ntrailing"); got != "https://x/pull/1" {
		t.Errorf("firstURL = %q", got)
	}
	if got := firstURL("no url here"); got != "no url here" {
		t.Errorf("firstURL fallback = %q", got)
	}
}
