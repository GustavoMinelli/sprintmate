// Package jira talks to the Jira Cloud REST and Agile APIs: it authenticates,
// resolves a configurable issue source (board / columns / sprint) and returns
// normalized issues for the TUI and context builder.
package jira

// Issue is the normalized view of a Jira issue used across SprintMate.
type Issue struct {
	Key                string
	Title              string
	Description        string // rendered from ADF to markdown
	Status             string
	Column             string // board column the status maps to (board mode)
	Priority           string
	Assignee           string // display name of the current assignee ("" = unassigned)
	StoryPoints        float64
	Sprint             string
	Labels             []string
	Project            string
	ProjectKey         string
	URL                string
	Comments           []Comment
	AcceptanceCriteria string
}

// Comment is a single issue comment, body rendered to markdown.
type Comment struct {
	Author string
	Body   string
}

// Board is an Agile board.
type Board struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// Column is a board column mapped to one or more status IDs.
type Column struct {
	Name      string
	StatusIDs []string
}

// Sprint is an Agile sprint.
type Sprint struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// Myself is the authenticated user (GET /myself), used to validate the
// connection in the wizard.
type Myself struct {
	AccountID    string `json:"accountId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

// Source describes which issues to fetch. When JQL is non-empty it overrides
// Board/Columns/Sprint entirely.
type Source struct {
	Board    string   // board name or id
	Sprint   string   // active | future | all | <sprintId>
	Columns  []string // board column names to include; empty = all columns
	Assignee string   // currentUser | <accountId> | all
	JQL      string   // full override

	SprintFieldID      string // optional custom field id override
	StoryPointsFieldID string // optional custom field id override
}

// Result bundles the fetched issues with display metadata for the footer.
type Result struct {
	Issues      []Issue
	SprintLabel string
	BoardName   string
}
