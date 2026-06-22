package app

import (
	"context"
	"fmt"

	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/forge"
	"github.com/GustavoMinelli/sprintmate/internal/git"
	"github.com/GustavoMinelli/sprintmate/internal/tracker"
)

// ShipPlan is the input to Ship: the worktree of a finished autonomous job.
type ShipPlan struct {
	Dir    string
	Branch string
	Key    string
	Title  string
}

// ShipResult reports what Ship did.
type ShipResult struct {
	PRURL string
	Steps []string // human-readable list of completed steps
}

// Ship performs the opt-in actions configured under cfg.Ship for a finished job:
// push the branch, open a PR via the forge, and write back to the tracker
// (comment + transition). Each step is gated by its config flag (all default
// off). It stops at the first hard failure, returning the steps done so far.
func Ship(ctx context.Context, cfg *config.Config, p ShipPlan) (ShipResult, error) {
	var res ShipResult
	add := func(s string) { res.Steps = append(res.Steps, s) }

	if cfg.Ship.PushBranch {
		if err := git.Push(ctx, p.Dir, p.Branch); err != nil {
			return res, fmt.Errorf("pushing %s: %w", p.Branch, err)
		}
		add("pushed " + p.Branch)
	}

	if cfg.Ship.CreatePR {
		f, ok := forge.Detect()
		if !ok {
			return res, fmt.Errorf("create PR: no forge CLI found (install the GitHub CLI `gh`)")
		}
		pr, err := f.CreatePR(ctx, p.Dir, forge.Params{
			Branch: p.Branch,
			Base:   cfg.Ship.Base,
			Title:  fmt.Sprintf("%s %s", p.Key, p.Title),
			Body:   fmt.Sprintf("Resolves %s.\n\n_Shipped via SprintMate._", p.Key),
		})
		if err != nil {
			return res, fmt.Errorf("creating PR: %w", err)
		}
		res.PRURL = pr.URL
		add("opened PR " + pr.URL)
	}

	if cfg.Jira.Host != "" && p.Key != "" && (cfg.Ship.Comment || cfg.Ship.Transition != "") {
		w := tracker.NewJira(cfg.Jira.Host, cfg.Jira.Email, cfg.Jira.Token)
		if cfg.Ship.Comment {
			text := "Shipped via SprintMate."
			if res.PRURL != "" {
				text = "Shipped via SprintMate: " + res.PRURL
			}
			if err := w.Comment(ctx, p.Key, text); err != nil {
				return res, fmt.Errorf("commenting on %s: %w", p.Key, err)
			}
			add("commented on " + p.Key)
		}
		if cfg.Ship.Transition != "" {
			if err := w.TransitionTo(ctx, p.Key, cfg.Ship.Transition); err != nil {
				return res, fmt.Errorf("transitioning %s: %w", p.Key, err)
			}
			add(fmt.Sprintf("moved %s → %s", p.Key, cfg.Ship.Transition))
		}
	}

	return res, nil
}
