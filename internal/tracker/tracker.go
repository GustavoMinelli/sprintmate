// Package tracker is the source-agnostic write-back surface for issue trackers.
//
// SprintMate is not Jira-only: the roadmap adds GitHub Issues, Linear and Azure
// Boards as sources. So every outward write (comment, status transition) goes
// through this small interface instead of calling a provider client directly.
// Jira is the first implementation; new providers supply their own constructor
// returning a Writer.
package tracker

import (
	"context"
	"fmt"
	"strings"

	"github.com/GustavoMinelli/sprintmate/internal/jira"
)

// Caps reports which write-back actions a provider supports, so the UI can hide
// the ones it can't do.
type Caps struct {
	Comment    bool
	Transition bool
}

// Writer is the set of best-effort write-backs SprintMate performs on an issue.
type Writer interface {
	// Comment posts a plain-text comment on the issue.
	Comment(ctx context.Context, key, text string) error
	// TransitionTo moves the issue to the workflow state matching target, by the
	// transition name or its destination status (case-insensitive). It is a no-op
	// when target is empty, and errors when no matching transition is available.
	TransitionTo(ctx context.Context, key, target string) error
	// Caps reports the supported actions.
	Caps() Caps
}

// NewJira returns a Writer backed by Jira Cloud.
func NewJira(host, email, token string) Writer {
	return jiraWriter{c: jira.New(host, email, token)}
}

type jiraWriter struct{ c *jira.Client }

func (w jiraWriter) Caps() Caps { return Caps{Comment: true, Transition: true} }

func (w jiraWriter) Comment(ctx context.Context, key, text string) error {
	return w.c.AddComment(ctx, key, text)
}

func (w jiraWriter) TransitionTo(ctx context.Context, key, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	ts, err := w.c.Transitions(ctx, key)
	if err != nil {
		return err
	}
	for _, t := range ts {
		if strings.EqualFold(t.Name, target) || strings.EqualFold(t.To, target) {
			return w.c.ApplyTransition(ctx, key, t.ID)
		}
	}
	available := make([]string, 0, len(ts))
	for _, t := range ts {
		available = append(available, t.To)
	}
	return fmt.Errorf("no transition to %q available on %s (current options: %s)",
		target, key, strings.Join(available, ", "))
}
