package git

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Corrigir login social":     "corrigir-login-social",
		"Ajustar layout  mobile!!":  "ajustar-layout-mobile",
		"Criar endpoint de eventos": "criar-endpoint-de-eventos",
		"Configuração da Sessão":    "configuracao-da-sessao",
		"  spaced  ":                "spaced",
		"###":                       "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugifyLength(t *testing.T) {
	long := "this is a really long title that should be truncated somewhere reasonable for a branch"
	if got := Slugify(long); len(got) > maxSlugLen {
		t.Errorf("slug too long (%d): %q", len(got), got)
	}
}

func TestBranchName(t *testing.T) {
	got := BranchName("{key}-{slug}", "DEMO-123", "Corrigir login social")
	if got != "demo-123-corrigir-login-social" {
		t.Errorf("BranchName = %q", got)
	}
}

// An empty slug (punctuation/CJK-only title) must never yield a ref git rejects
// such as a trailing "-" or "/"; it falls back to the key.
func TestBranchNameEmptySlug(t *testing.T) {
	cases := []struct{ pattern, key, title, want string }{
		{"{key}-{slug}", "DEMO-1", "###", "demo-1"},
		{"feature/{slug}", "DEMO-2", "###", "feature/demo-2"},
		{"feature/{slug}", "DEMO-3", "Real title", "feature/real-title"},
		{"{slug}", "DEMO-4", "", "demo-4"},
	}
	for _, c := range cases {
		got := BranchName(c.pattern, c.key, c.title)
		if got != c.want {
			t.Errorf("BranchName(%q,%q,%q) = %q, want %q", c.pattern, c.key, c.title, got, c.want)
		}
		if got == "" || got[len(got)-1] == '/' || got[len(got)-1] == '-' {
			t.Errorf("BranchName produced an invalid ref %q", got)
		}
	}
}

func TestBranchOpsInTempRepo(t *testing.T) {
	if !Available() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	ctx := context.Background()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "t@t.com")
	mustGit(t, dir, "config", "user.name", "Test")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "init")

	if !IsRepo(ctx, dir) {
		t.Fatal("expected dir to be a repo")
	}

	created, err := CreateOrReuseBranch(ctx, dir, "demo-1-feature")
	if err != nil || !created {
		t.Fatalf("create branch: created=%v err=%v", created, err)
	}
	if b, _ := CurrentBranch(ctx, dir); b != "demo-1-feature" {
		t.Errorf("current branch = %q", b)
	}

	// reuse path: switch away then back
	mustGit(t, dir, "checkout", "-")
	created, err = CreateOrReuseBranch(ctx, dir, "demo-1-feature")
	if err != nil || created {
		t.Fatalf("reuse branch: created=%v err=%v", created, err)
	}

	commits, err := RecentCommits(ctx, dir, 5)
	if err != nil || len(commits) != 1 {
		t.Fatalf("recent commits: %v err=%v", commits, err)
	}
}

func TestWorktreeLifecycleInTempRepo(t *testing.T) {
	if !Available() {
		t.Skip("git not available")
	}
	ctx := context.Background()
	repo := t.TempDir()
	mustGit(t, repo, "init")
	mustGit(t, repo, "config", "user.email", "t@t.com")
	mustGit(t, repo, "config", "user.name", "Test")
	mustGit(t, repo, "commit", "--allow-empty", "-m", "init")

	wt := filepath.Join(t.TempDir(), "base", "demo-1")

	// First add creates the branch (-b path) and the worktree directory.
	if err := WorktreeAdd(ctx, repo, wt, "demo-1-feature"); err != nil {
		t.Fatalf("WorktreeAdd (create): %v", err)
	}
	if !IsRepo(ctx, wt) {
		t.Fatal("worktree dir is not a repo")
	}
	if b, _ := CurrentBranch(ctx, wt); b != "demo-1-feature" {
		t.Errorf("worktree branch = %q, want demo-1-feature", b)
	}

	// Second add at the same dir is an idempotent reuse (no error).
	if err := WorktreeAdd(ctx, repo, wt, "demo-1-feature"); err != nil {
		t.Fatalf("WorktreeAdd (reuse): %v", err)
	}

	list, err := WorktreeList(ctx, repo)
	if err != nil {
		t.Fatalf("WorktreeList: %v", err)
	}
	// git reports real paths; resolve the temp dir's /var → /private/var symlink.
	want := wt
	if r, err := filepath.EvalSymlinks(wt); err == nil {
		want = r
	}
	var found *Worktree
	for i := range list {
		if filepath.Clean(list[i].Path) == filepath.Clean(want) {
			found = &list[i]
		}
	}
	if found == nil {
		t.Fatalf("worktree %q not listed: %+v", wt, list)
	}
	if found.Branch != "demo-1-feature" {
		t.Errorf("listed branch = %q", found.Branch)
	}

	// Remove, then re-add: the branch now exists, exercising the checkout path.
	if err := WorktreeRemove(ctx, repo, wt, true); err != nil {
		t.Fatalf("WorktreeRemove: %v", err)
	}
	if err := WorktreeAdd(ctx, repo, wt, "demo-1-feature"); err != nil {
		t.Fatalf("WorktreeAdd (existing branch): %v", err)
	}
	if b, _ := CurrentBranch(ctx, wt); b != "demo-1-feature" {
		t.Errorf("re-added worktree branch = %q", b)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
