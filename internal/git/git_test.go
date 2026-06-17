package git

import (
	"context"
	"os/exec"
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

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
