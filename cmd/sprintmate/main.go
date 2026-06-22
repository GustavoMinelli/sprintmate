// Command sprintmate connects your Jira board to AI coding agents.
package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/GustavoMinelli/sprintmate/internal/app"
	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
	"github.com/GustavoMinelli/sprintmate/internal/tui"

	// Register the built-in agents (extension point: add more here).
	_ "github.com/GustavoMinelli/sprintmate/internal/agents/claude"
	_ "github.com/GustavoMinelli/sprintmate/internal/agents/codex"
)

// issueKeyRe matches a Jira issue key like DEMO-123 (case-insensitive so a
// lowercased argument still works).
var issueKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]+-[0-9]+$`)

// Set via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	startInWizard := false

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("sprintmate %s (commit %s, built %s)\n", version, commit, date)
			return
		case "help", "--help", "-h":
			usage()
			return
		case "config":
			startInWizard = true
		default:
			// `sprintmate ABC-123` launches one issue directly, skipping the dashboard.
			if issueKeyRe.MatchString(os.Args[1]) {
				quickLaunch(strings.ToUpper(os.Args[1]))
				return
			}
			fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
			usage()
			os.Exit(2)
		}
	}

	// Load config; tolerate a missing/incomplete file (the wizard handles it).
	cfg, err := config.Load()
	if err != nil {
		cfg = config.LoadRaw() // best-effort pre-fill (may be nil)
		startInWizard = true
	}

	newCfg, result, err := tui.Run(cfg, startInWizard, version)
	if err != nil {
		fatal(err)
	}
	if result == nil || !result.Launch {
		return
	}
	launchPlan(newCfg, result.Issue, result.Agent)
}

// quickLaunch fetches a single issue by key and launches it directly, bypassing
// the dashboard. It requires a complete config (Jira credentials + workdir).
func quickLaunch(key string) {
	cfg, err := config.Load()
	if err != nil {
		fatal(fmt.Errorf("`sprintmate %s` needs a complete config — run `sprintmate config` first (%w)", key, err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	issue, err := jira.New(cfg.Jira.Host, cfg.Jira.Email, cfg.Jira.Token).GetIssue(ctx, key)
	if err != nil {
		fatal(fmt.Errorf("fetching %s: %w", key, err))
	}
	launchPlan(cfg, issue, cfg.Agent.Default)
}

// launchPlan builds and runs the launch plan for an issue+agent, shared by the
// dashboard result path and quick-launch.
func launchPlan(cfg *config.Config, issue jira.Issue, agent string) {
	plan, err := app.BuildPlan(cfg, issue, agent)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("→ %s · %s · branch %s · %s\n", plan.Issue.Key, plan.AgentName, plan.Branch, plan.Dir)
	if err := app.PrepareAndLaunch(context.Background(), cfg, plan); err != nil {
		fatal(err)
	}
}

func usage() {
	fmt.Print(`SprintMate — connect Jira to your AI coding agents.

Usage:
  sprintmate            Open the dashboard (or the setup wizard on first run)
  sprintmate ABC-123    Launch a specific issue directly, skipping the dashboard
  sprintmate config     Open the setup wizard / settings
  sprintmate version    Print version
  sprintmate help       Show this help

Config: run the wizard, or edit the YAML in your user config dir.
Docs:   https://github.com/GustavoMinelli/sprintmate
`)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
