package jira

import (
	"context"
	"net/http"
	"strings"
)

// field is one entry of GET /rest/api/3/field.
type field struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// customFields discovers the custom field IDs for Sprint and Story Points by
// matching field names. Either return value may be empty if not found.
func (c *Client) customFields(ctx context.Context) (sprintID, storyPointsID string, err error) {
	var fields []field
	if err = c.do(ctx, http.MethodGet, "/rest/api/3/field", nil, nil, &fields); err != nil {
		return "", "", err
	}
	for _, f := range fields {
		name := strings.ToLower(strings.TrimSpace(f.Name))
		switch {
		case sprintID == "" && name == "sprint":
			sprintID = f.ID
		case storyPointsID == "" && (name == "story points" || name == "story point estimate"):
			storyPointsID = f.ID
		}
	}
	return sprintID, storyPointsID, nil
}

// resolveFieldIDs returns the field IDs to request, preferring explicit
// overrides from the Source and falling back to discovery.
func (c *Client) resolveFieldIDs(ctx context.Context, src Source) (sprintID, storyPointsID string) {
	sprintID, storyPointsID = src.SprintFieldID, src.StoryPointsFieldID
	if sprintID != "" && storyPointsID != "" {
		return sprintID, storyPointsID
	}
	ds, dp, err := c.customFields(ctx)
	if err != nil {
		return sprintID, storyPointsID // discovery is best-effort
	}
	if sprintID == "" {
		sprintID = ds
	}
	if storyPointsID == "" {
		storyPointsID = dp
	}
	return sprintID, storyPointsID
}
