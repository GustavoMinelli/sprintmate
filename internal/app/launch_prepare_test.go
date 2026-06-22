package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/GustavoMinelli/sprintmate/internal/agents"
	"github.com/GustavoMinelli/sprintmate/internal/config"
	issuecontext "github.com/GustavoMinelli/sprintmate/internal/context"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
)

func TestPrepareAndLaunchHappyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the POSIX 'true' command")
	}
	agents.Register("test-true", func() agents.Agent {
		return agents.NewBuiltin("test-true", "true", nil)
	})

	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workdir = dir
	cfg.Git.CreateBranch = false // skip git
	cfg.Launch.Strategy = config.StrategyInplace

	plan, err := BuildPlan(cfg, jira.Issue{Key: "DEMO-1", Title: "Test", ProjectKey: "DEMO"}, "test-true")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := PrepareAndLaunch(context.Background(), cfg, plan); err != nil {
		t.Fatalf("PrepareAndLaunch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, issuecontext.Filename)); err != nil {
		t.Errorf("context file not written: %v", err)
	}
}

func TestPrepareAndLaunchUnknownAgent(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workdir = dir
	cfg.Git.CreateBranch = false

	plan, _ := BuildPlan(cfg, jira.Issue{Key: "DEMO-2", ProjectKey: "DEMO"}, "no-such-agent")
	err := PrepareAndLaunch(context.Background(), cfg, plan)
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Errorf("expected unknown-agent error, got %v", err)
	}
}

func TestPrepareAndLaunchNotInstalled(t *testing.T) {
	agents.Register("test-missing", func() agents.Agent {
		return agents.NewBuiltin("test-missing", "definitely-not-a-real-binary-zzz", nil)
	})
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workdir = dir
	cfg.Git.CreateBranch = false

	plan, _ := BuildPlan(cfg, jira.Issue{Key: "DEMO-3", ProjectKey: "DEMO"}, "test-missing")
	err := PrepareAndLaunch(context.Background(), cfg, plan)
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected not-installed error, got %v", err)
	}
}
