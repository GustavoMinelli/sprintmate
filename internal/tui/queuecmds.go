package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/GustavoMinelli/sprintmate/internal/agents"
	"github.com/GustavoMinelli/sprintmate/internal/app"
	"github.com/GustavoMinelli/sprintmate/internal/config"
	issuecontext "github.com/GustavoMinelli/sprintmate/internal/context"
	"github.com/GustavoMinelli/sprintmate/internal/git"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
	"github.com/GustavoMinelli/sprintmate/internal/notify"
	"github.com/GustavoMinelli/sprintmate/internal/queue"
)

// prepareJobCmd builds the launch plan and creates the worktree + context files
// for a headless run off the update goroutine, returning a job ready to enqueue.
func prepareJobCmd(ctx context.Context, cfg *config.Config, issue jira.Issue, agent string) tea.Cmd {
	cfgCopy := *cfg
	return func() tea.Msg {
		plan, err := app.BuildPlan(&cfgCopy, issue, agent)
		if err != nil {
			return jobPreparedMsg{key: issue.Key, err: err}
		}
		dir, branch, err := app.PrepareHeadless(ctx, &cfgCopy, plan)
		if err != nil {
			return jobPreparedMsg{key: issue.Key, err: err}
		}
		return jobPreparedMsg{key: issue.Key, job: &queue.Job{
			Key:      issue.Key,
			Title:    issue.Title,
			Agent:    agent,
			Dir:      dir,
			Branch:   branch,
			PlanPath: queue.PlanPath(dir),
			LogPath:  queue.LogPath(dir),
		}}
	}
}

// runPhaseCmd runs exactly one phase of one job and reports a phaseDoneMsg. It
// recovers from a panic so a misbehaving agent never takes down the TUI. The
// job is passed by value so the goroutine never touches engine state.
func runPhaseCmd(ctx context.Context, cfg *config.Config, job queue.Job, phase queue.Phase) tea.Cmd {
	cfgCopy := *cfg
	return func() (m tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				m = phaseDoneMsg{id: job.ID, phase: phase, err: fmt.Errorf("panic in phase: %v", r)}
			}
		}()
		agent, ok := agents.Get(job.Agent)
		if !ok {
			return phaseDoneMsg{id: job.ID, phase: phase, err: fmt.Errorf("unknown agent %q", job.Agent)}
		}
		mode := agents.HeadlessPlan
		ctxFile := issuecontext.Filename
		if phase == queue.ExecPhase {
			mode = agents.HeadlessExecute
			ctxFile = app.ExecContextFile
		}
		spec, ok := agent.HeadlessSpec(agents.Params{
			IssueKey:    job.Key,
			Branch:      job.Branch,
			Dir:         job.Dir,
			ContextPath: filepath.Join(job.Dir, ctxFile),
		}, cfgCopy.AgentConfig(job.Agent), mode)
		if !ok {
			return phaseDoneMsg{id: job.ID, phase: phase, err: fmt.Errorf("agent %q has no headless mode", job.Agent)}
		}
		jcopy := job
		err := queue.RunPhase(ctx, &jcopy, phase, spec)
		return phaseDoneMsg{id: job.ID, phase: phase, err: err}
	}
}

// loadPlanCmd reads a job's captured plan file.
func loadPlanCmd(id int, planPath string) tea.Cmd {
	return func() tea.Msg {
		b, err := os.ReadFile(planPath)
		if errors.Is(err, os.ErrNotExist) {
			return planLoadedMsg{id: id, text: "(plan not ready yet)"}
		}
		if err != nil {
			return planLoadedMsg{id: id, err: err}
		}
		text := string(b)
		if len(text) == 0 {
			text = "(the agent produced no plan output)"
		}
		return planLoadedMsg{id: id, text: text}
	}
}

// loadDiffCmd runs git diff in a job's worktree (when it has executed).
func loadDiffCmd(id int, dir string, executed bool) tea.Cmd {
	return func() tea.Msg {
		if !executed {
			return diffLoadedMsg{id: id, diff: "(no changes yet — not executed)"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, err := git.Diff(ctx, dir)
		if err != nil {
			return diffLoadedMsg{id: id, err: err}
		}
		return diffLoadedMsg{id: id, diff: out}
	}
}

// shipCmd runs the ship pipeline for a finished job. parent is the monitor's
// program-scoped context so quitting cancels an in-flight ship too.
func shipCmd(parent context.Context, cfg *config.Config, job queue.Job) tea.Cmd {
	cfgCopy := *cfg
	plan := app.ShipPlan{Dir: job.Dir, Branch: job.Branch, Key: job.Key, Title: job.Title}
	id := job.ID
	return func() (m tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				m = shipDoneMsg{id: id, err: fmt.Errorf("ship panic: %v", r)}
			}
		}()
		ctx, cancel := context.WithTimeout(parent, 90*time.Second)
		defer cancel()
		res, err := app.Ship(ctx, &cfgCopy, plan)
		return shipDoneMsg{id: id, prURL: res.PRURL, steps: res.Steps, err: err}
	}
}

// notifyCmd fires a best-effort completion notification.
func notifyCmd(cfg *config.Config, outcome, key string) tea.Cmd {
	o := notify.Options{
		Title:   "SprintMate",
		Body:    fmt.Sprintf("%s %s", key, outcome),
		Bell:    cfg.Notify.Bell,
		OS:      cfg.Notify.OS,
		Webhook: cfg.Notify.WebhookURL,
	}
	return func() tea.Msg {
		notify.Send(context.Background(), o)
		return nil
	}
}
