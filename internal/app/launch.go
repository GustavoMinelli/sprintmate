// Package app holds the launch orchestration that ties the pieces together:
// resolve the project directory, create/reuse the git branch, build the issue
// context file, and launch the chosen agent via the terminal package.
//
// It deliberately does not depend on the TUI: the TUI selects an issue+agent
// and exits, then the entrypoint calls PrepareAndLaunch.
package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/GustavoMinelli/sprintmate/internal/agents"
	"github.com/GustavoMinelli/sprintmate/internal/config"
	issuecontext "github.com/GustavoMinelli/sprintmate/internal/context"
	"github.com/GustavoMinelli/sprintmate/internal/git"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
	"github.com/GustavoMinelli/sprintmate/internal/terminal"
	"github.com/GustavoMinelli/sprintmate/internal/tracker"
)

// Plan is the resolved set of actions for launching an issue, computed before
// any side effects so it can be previewed.
type Plan struct {
	Issue     jira.Issue
	AgentName string
	Dir       string
	Branch    string
	// Strategy is the launch strategy to use. It defaults to the configured
	// value but callers may pin it to a concrete strategy (e.g. the dashboard
	// resolves "auto" up front so a windowed launch can't silently degrade to
	// an in-place handoff that would corrupt the running TUI).
	Strategy string
}

// BuildPlan resolves the working directory and branch name for an issue without
// performing any side effects.
func BuildPlan(cfg *config.Config, issue jira.Issue, agentName string) (Plan, error) {
	dir, ok := cfg.WorkdirPath()
	if !ok {
		return Plan{}, fmt.Errorf("no working directory configured (set it in settings)")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return Plan{}, fmt.Errorf("the working directory %q does not exist", dir)
	}
	branch := git.BranchName(cfg.Git.BranchPattern, issue.Key, issue.Title)
	return Plan{Issue: issue, AgentName: agentName, Dir: dir, Branch: branch, Strategy: cfg.Launch.Strategy}, nil
}

// PrepareAndLaunch executes the plan: branch, context, agent launch.
func PrepareAndLaunch(ctx context.Context, cfg *config.Config, plan Plan) error {
	agent, ok := agents.Get(plan.AgentName)
	if !ok {
		return fmt.Errorf("unknown agent %q", plan.AgentName)
	}
	if !agent.IsInstalled() {
		return fmt.Errorf("agent %q is not installed (command not found on PATH)", plan.AgentName)
	}

	// Bound the preparation steps (git, network, file write) so a hung git
	// prompt or slow API can't block forever. The interactive agent launch
	// itself runs without a deadline.
	prepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Git branch/worktree (best-effort: only when enabled and inside a repo).
	// Only expose {branch} to the agent when a branch was actually prepared.
	// The agent runs in agentDir, which is the worktree when worktrees are on.
	branchForAgent := ""
	agentDir := plan.Dir
	inRepo := git.Available() && git.IsRepo(prepCtx, plan.Dir)
	switch {
	case cfg.Git.UseWorktrees && inRepo:
		// Each issue gets an isolated worktree so parallel agents never touch the
		// same files; the main checkout is left on its current branch.
		base := cfg.WorktreeBasePath(plan.Dir)
		wt := git.WorktreePath(base, plan.Issue.Key)
		if err := git.WorktreeAdd(prepCtx, plan.Dir, wt, plan.Branch); err != nil {
			return fmt.Errorf("preparing worktree: %w", err)
		}
		agentDir = wt
		branchForAgent = plan.Branch
	case cfg.Git.CreateBranch && inRepo:
		if _, err := git.CreateOrReuseBranch(prepCtx, plan.Dir, plan.Branch); err != nil {
			return fmt.Errorf("preparing branch: %w", err)
		}
		branchForAgent = plan.Branch
	}

	// Best-effort tracker interactions; failures here never block the launch.
	if cfg.Jira.Host != "" && plan.Issue.Key != "" {
		// Refresh the genuinely most-recent comments for the context (a read; do
		// this before posting our own note so it isn't fed back into the context).
		client := jira.New(cfg.Jira.Host, cfg.Jira.Email, cfg.Jira.Token)
		if cs, err := client.RecentComments(prepCtx, plan.Issue.Key, 5); err == nil && len(cs) > 0 {
			plan.Issue.Comments = cs
		}
		// Outward write-backs go through the source-agnostic tracker (opt-in).
		w := tracker.NewJira(cfg.Jira.Host, cfg.Jira.Email, cfg.Jira.Token)
		if cfg.Jira.OnLaunch.Comment {
			_ = w.Comment(prepCtx, plan.Issue.Key, launchComment(plan.AgentName, branchForAgent))
		}
		if cfg.Jira.OnLaunch.Transition != "" {
			_ = w.TransitionTo(prepCtx, plan.Issue.Key, cfg.Jira.OnLaunch.Transition)
		}
	}

	// Issue context file, written into the directory the agent will run in (the
	// worktree when enabled). The planning preamble (telling the agent to plan
	// before editing) is on by default and configurable under config.context.
	builder := issuecontext.NewBuilder()
	builder.PlanFirst = cfg.PlanFirstEnabled()
	builder.Preamble = cfg.Context.Preamble
	ctxPath, err := builder.Build(prepCtx, plan.Issue, agentDir)
	if err != nil {
		return err
	}

	// Agent command spec.
	spec := agent.Spec(agents.Params{
		IssueKey:    plan.Issue.Key,
		ContextPath: ctxPath,
		Branch:      branchForAgent,
		Dir:         agentDir,
	}, cfg.AgentConfig(plan.AgentName))

	strategy := plan.Strategy
	if strategy == "" {
		strategy = cfg.Launch.Strategy
	}
	return terminal.Launch(spec, strategy)
}

// launchComment is the note posted to the issue when jira.on_launch.comment is
// enabled, so teammates can see the work has started.
func launchComment(agent, branch string) string {
	if branch != "" {
		return fmt.Sprintf("Started via SprintMate with the %s agent on branch `%s`.", agent, branch)
	}
	return fmt.Sprintf("Started via SprintMate with the %s agent.", agent)
}
