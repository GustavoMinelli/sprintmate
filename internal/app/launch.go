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

	// Git branch (best-effort: only when enabled and inside a repo). Only
	// expose {branch} to the agent when a branch was actually created/reused.
	branchForAgent := ""
	if cfg.Git.CreateBranch && git.Available() && git.IsRepo(prepCtx, plan.Dir) {
		if _, err := git.CreateOrReuseBranch(prepCtx, plan.Dir, plan.Branch); err != nil {
			return fmt.Errorf("preparing branch: %w", err)
		}
		branchForAgent = plan.Branch
	}

	// Best-effort Jira interactions; failures here never block the launch.
	if cfg.Jira.Host != "" && plan.Issue.Key != "" {
		client := jira.New(cfg.Jira.Host, cfg.Jira.Email, cfg.Jira.Token)
		// Refresh the genuinely most-recent comments for the context (do this
		// before posting our own note so it isn't fed back into the context).
		if cs, err := client.RecentComments(prepCtx, plan.Issue.Key, 5); err == nil && len(cs) > 0 {
			plan.Issue.Comments = cs
		}
		// Optionally post a launch note so the team can see work has started.
		if cfg.Jira.OnLaunch.Comment {
			_ = client.AddComment(prepCtx, plan.Issue.Key, launchComment(plan.AgentName, branchForAgent))
		}
	}

	// Issue context file.
	ctxPath, err := issuecontext.NewBuilder().Build(prepCtx, plan.Issue, plan.Dir)
	if err != nil {
		return err
	}

	// Agent command spec.
	spec := agent.Spec(agents.Params{
		IssueKey:    plan.Issue.Key,
		ContextPath: ctxPath,
		Branch:      branchForAgent,
		Dir:         plan.Dir,
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
