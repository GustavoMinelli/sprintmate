// Command sprintmate connects your Jira board to AI coding agents.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GustavoMinelli/sprintmate/internal/app"
	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/tui"

	// Register the built-in agents (extension point: add more here).
	_ "github.com/GustavoMinelli/sprintmate/internal/agents/claude"
	_ "github.com/GustavoMinelli/sprintmate/internal/agents/codex"
)

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

	plan, err := app.BuildPlan(newCfg, result.Issue, result.Agent)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("→ %s · %s · branch %s · %s\n", plan.Issue.Key, plan.AgentName, plan.Branch, plan.Dir)
	if err := app.PrepareAndLaunch(context.Background(), newCfg, plan); err != nil {
		fatal(err)
	}
}

func usage() {
	fmt.Print(`SprintMate — connect Jira to your AI coding agents.

Usage:
  sprintmate            Open the dashboard (or the setup wizard on first run)
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
