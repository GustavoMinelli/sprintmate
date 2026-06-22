package app

import (
	"context"
	"fmt"
	"time"

	"github.com/GustavoMinelli/sprintmate/internal/config"
	issuecontext "github.com/GustavoMinelli/sprintmate/internal/context"
	"github.com/GustavoMinelli/sprintmate/internal/git"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
)

// ExecContextFile is the execute-phase context written alongside the standard
// plan-phase .issue-context.md in an autonomous job's worktree.
const ExecContextFile = ".issue-context.exec.md"

// PrepareHeadless creates the isolated worktree and both phase context files for
// an autonomous run, without launching anything. Headless jobs always use a
// worktree so parallel agents never share files. It returns the worktree
// directory and branch.
func PrepareHeadless(ctx context.Context, cfg *config.Config, plan Plan) (dir, branch string, err error) {
	prepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if !git.Available() || !git.IsRepo(prepCtx, plan.Dir) {
		return "", "", fmt.Errorf("autonomous runs need the working directory to be a git repository")
	}

	base := cfg.WorktreeBasePath(plan.Dir)
	wt := git.WorktreePath(base, plan.Issue.Key)
	if err := git.WorktreeAdd(prepCtx, plan.Dir, wt, plan.Branch); err != nil {
		return "", "", fmt.Errorf("preparing worktree: %w", err)
	}

	// Best-effort: refresh the latest comments for richer context.
	if cfg.Jira.Host != "" && plan.Issue.Key != "" {
		client := jira.New(cfg.Jira.Host, cfg.Jira.Email, cfg.Jira.Token)
		if cs, e := client.RecentComments(prepCtx, plan.Issue.Key, 5); e == nil && len(cs) > 0 {
			plan.Issue.Comments = cs
		}
	}

	// Plan-phase context (.issue-context.md): "produce a plan only".
	planB := issuecontext.NewBuilder()
	planB.PlanFirst = cfg.PlanFirstEnabled()
	planB.Preamble = cfg.Context.Preamble
	planB.Intent = issuecontext.HeadlessPlan
	if _, err := planB.Build(prepCtx, plan.Issue, wt); err != nil {
		return "", "", err
	}

	// Execute-phase context (.issue-context.exec.md): "implement the approved plan".
	execB := issuecontext.NewBuilder()
	execB.Intent = issuecontext.HeadlessExecute
	if _, err := execB.BuildAs(prepCtx, plan.Issue, wt, ExecContextFile); err != nil {
		return "", "", err
	}

	return wt, plan.Branch, nil
}
