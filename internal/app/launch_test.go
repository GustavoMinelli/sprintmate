package app

import (
	"testing"

	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
)

func TestBuildPlan(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workdir = dir

	plan, err := BuildPlan(cfg, jira.Issue{Key: "DEMO-123", Title: "Corrigir login", ProjectKey: "DEMO"}, "claude")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Dir != dir {
		t.Errorf("dir = %q, want %q", plan.Dir, dir)
	}
	if plan.Branch != "demo-123-corrigir-login" {
		t.Errorf("branch = %q", plan.Branch)
	}
}

func TestBuildPlanNoWorkdir(t *testing.T) {
	cfg := config.Default()
	if _, err := BuildPlan(cfg, jira.Issue{Key: "ZZZ-1"}, "claude"); err == nil {
		t.Error("expected error when no workdir is configured")
	}
}

func TestBuildPlanMissingDir(t *testing.T) {
	cfg := config.Default()
	cfg.Workdir = "/nonexistent/path/xyz"
	if _, err := BuildPlan(cfg, jira.Issue{Key: "DEMO-1", ProjectKey: "DEMO"}, "claude"); err == nil {
		t.Error("expected error for missing dir")
	}
}
