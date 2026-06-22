package jira

import (
	"context"
	"net/http"
	"net/url"
)

// Transition is a workflow transition currently available on an issue.
type Transition struct {
	ID   string
	Name string // the transition's own name, e.g. "Start progress"
	To   string // the destination status name, e.g. "In Progress"
}

// Transitions lists the workflow transitions available on an issue right now
// (Jira only returns those valid from the issue's current status).
func (c *Client) Transitions(ctx context.Context, key string) ([]Transition, error) {
	var wrap struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	path := "/rest/api/3/issue/" + url.PathEscape(key) + "/transitions"
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &wrap); err != nil {
		return nil, err
	}
	out := make([]Transition, 0, len(wrap.Transitions))
	for _, t := range wrap.Transitions {
		out = append(out, Transition{ID: t.ID, Name: t.Name, To: t.To.Name})
	}
	return out, nil
}

// ApplyTransition moves an issue through the transition with the given id.
func (c *Client) ApplyTransition(ctx context.Context, key, transitionID string) error {
	body := map[string]any{"transition": map[string]string{"id": transitionID}}
	path := "/rest/api/3/issue/" + url.PathEscape(key) + "/transitions"
	return c.do(ctx, http.MethodPost, path, nil, body, nil)
}
