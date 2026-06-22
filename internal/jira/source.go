package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// standardFields are always requested from the search endpoints.
var standardFields = []string{
	"summary", "status", "priority", "labels", "project", "description", "comment",
}

// maxComments caps how many (most recent) comments we keep per issue.
const maxComments = 5

// FetchIssues resolves the configured Source and returns normalized issues.
//
// When Source.JQL is set it is used verbatim against /rest/api/3/search/jql.
// Otherwise the board is resolved and issues are pulled from the Agile board
// (scoped natively to the board), filtered by the selected columns' statuses
// and the chosen sprint.
func (c *Client) FetchIssues(ctx context.Context, src Source) (Result, error) {
	sprintFieldID, storyPointsID := c.resolveFieldIDs(ctx, src)
	fields := append(append([]string{}, standardFields...), nonEmpty(sprintFieldID, storyPointsID)...)

	parser := issueParser{
		client:        c,
		sprintFieldID: sprintFieldID,
		storyPoints:   storyPointsID,
		columnByStat:  map[string]string{},
	}

	// JQL override mode.
	if strings.TrimSpace(src.JQL) != "" {
		raws, err := c.searchJQL(ctx, src.JQL, fields)
		if err != nil {
			return Result{}, err
		}
		return Result{Issues: parser.all(raws), SprintLabel: "Custom JQL"}, nil
	}

	// Board mode.
	board, err := c.ResolveBoard(ctx, src.Board)
	if err != nil {
		return Result{}, err
	}
	cols, err := c.BoardColumns(ctx, board.ID)
	if err != nil {
		return Result{}, err
	}
	statusIDs := selectStatusIDs(cols, src.Columns)
	for _, col := range cols {
		for _, id := range col.StatusIDs {
			parser.columnByStat[id] = col.Name
		}
	}

	sprintID, sprintLabel, useSprintPath, err := c.resolveSprint(ctx, board.ID, src.Sprint)
	if err != nil {
		return Result{}, err
	}

	jql := buildJQL(src.Assignee, statusIDs)
	path := fmt.Sprintf("/rest/agile/1.0/board/%d/issue", board.ID)
	if useSprintPath {
		path = fmt.Sprintf("/rest/agile/1.0/board/%d/sprint/%d/issue", board.ID, sprintID)
	}
	raws, err := c.agileIssues(ctx, path, jql, fields)
	if err != nil {
		return Result{}, err
	}
	if sprintLabel == "" {
		sprintLabel = "All sprints"
	}
	return Result{Issues: parser.all(raws), SprintLabel: sprintLabel, BoardName: board.Name}, nil
}

// resolveSprint determines the sprint to query. It returns the sprint id, a
// human label, whether the sprint-scoped board path should be used, and an
// error. A failed ListSprints call propagates as an error (so the user can
// retry); only a successful-but-empty result falls back to "all".
func (c *Client) resolveSprint(ctx context.Context, boardID int, spec string) (id int, label string, usePath bool, err error) {
	spec = strings.TrimSpace(spec)
	switch {
	case spec == "" || strings.EqualFold(spec, "all"):
		return 0, "", false, nil
	case isDigits(spec):
		var n int
		_, _ = fmt.Sscan(spec, &n) // spec is all digits, so this can't fail
		return n, fmt.Sprintf("Sprint %d", n), true, nil
	default: // active | future
		sprints, e := c.ListSprints(ctx, boardID, strings.ToLower(spec))
		if e != nil {
			return 0, "", false, fmt.Errorf("listing %s sprints: %w", spec, e)
		}
		if len(sprints) == 0 {
			return 0, "", false, nil // no such sprint: fall back to all issues on the board
		}
		s := sprints[0]
		return s.ID, s.Name, true, nil
	}
}

// buildJQL assembles the assignee + status clauses for board queries.
func buildJQL(assignee string, statusIDs []string) string {
	var parts []string
	switch strings.TrimSpace(assignee) {
	case "", "currentUser", "currentUser()":
		parts = append(parts, "assignee = currentUser()")
	case "all":
		// no assignee filter
	default:
		parts = append(parts, fmt.Sprintf("assignee = %q", assignee))
	}
	if len(statusIDs) > 0 {
		parts = append(parts, "status in ("+strings.Join(statusIDs, ", ")+")")
	}
	jql := strings.Join(parts, " AND ")
	if jql != "" {
		jql += " "
	}
	return jql + "ORDER BY priority DESC"
}

// selectStatusIDs returns the union of status IDs for the named columns. When
// names is empty all columns are included.
func selectStatusIDs(cols []Column, names []string) []string {
	want := map[string]bool{}
	for _, n := range names {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}
	seen := map[string]bool{}
	var ids []string
	for _, col := range cols {
		if len(want) > 0 && !want[strings.ToLower(col.Name)] {
			continue
		}
		for _, id := range col.StatusIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// agileIssues pages through an Agile board issue endpoint (startAt pagination).
func (c *Client) agileIssues(ctx context.Context, path, jql string, fields []string) ([]rawIssue, error) {
	var out []rawIssue
	startAt := 0
	for {
		q := url.Values{}
		q.Set("jql", jql)
		q.Set("fields", strings.Join(fields, ","))
		q.Set("startAt", fmt.Sprint(startAt))
		q.Set("maxResults", "100")
		var page struct {
			Issues     []rawIssue `json:"issues"`
			Total      int        `json:"total"`
			MaxResults int        `json:"maxResults"`
		}
		if err := c.do(ctx, http.MethodGet, path, q, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Issues...)
		startAt += len(page.Issues)
		if len(page.Issues) == 0 || startAt >= page.Total {
			break
		}
	}
	return out, nil
}

// searchJQL pages through /rest/api/3/search/jql (token pagination).
func (c *Client) searchJQL(ctx context.Context, jql string, fields []string) ([]rawIssue, error) {
	var out []rawIssue
	token := ""
	for {
		body := map[string]any{
			"jql":        jql,
			"fields":     fields,
			"maxResults": 100,
		}
		if token != "" {
			body["nextPageToken"] = token
		}
		var page struct {
			Issues        []rawIssue `json:"issues"`
			NextPageToken string     `json:"nextPageToken"`
		}
		if err := c.do(ctx, http.MethodPost, "/rest/api/3/search/jql", nil, body, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Issues...)
		// Stop only when there is no next page. Don't break on an empty page
		// while a token is still offered. Guard against a stuck (unchanged)
		// token to avoid an unbounded loop.
		if page.NextPageToken == "" || page.NextPageToken == token {
			break
		}
		token = page.NextPageToken
	}
	return out, nil
}

// rawIssue is the on-the-wire issue shape; fields are decoded lazily.
type rawIssue struct {
	Key    string                     `json:"key"`
	Fields map[string]json.RawMessage `json:"fields"`
}

// issueParser turns rawIssues into normalized Issues.
type issueParser struct {
	client        *Client
	sprintFieldID string
	storyPoints   string
	columnByStat  map[string]string
}

func (p issueParser) all(raws []rawIssue) []Issue {
	issues := make([]Issue, 0, len(raws))
	for _, r := range raws {
		issues = append(issues, p.one(r))
	}
	return issues
}

func (p issueParser) one(r rawIssue) Issue {
	iss := Issue{Key: r.Key, URL: p.client.BrowseURL(r.Key)}
	f := r.Fields

	iss.Title = decodeString(f["summary"])
	iss.Labels = decodeStrings(f["labels"])

	var status struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	decode(f["status"], &status)
	iss.Status = status.Name
	if col, ok := p.columnByStat[status.ID]; ok {
		iss.Column = col
	} else {
		iss.Column = status.Name
	}

	var priority struct {
		Name string `json:"name"`
	}
	decode(f["priority"], &priority)
	iss.Priority = priority.Name

	var project struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	decode(f["project"], &project)
	iss.Project = project.Name
	iss.ProjectKey = project.Key

	iss.Description = renderADF(f["description"])
	iss.AcceptanceCriteria = extractAcceptanceCriteria(iss.Description)
	iss.Comments = decodeComments(f["comment"])

	if p.sprintFieldID != "" {
		iss.Sprint = decodeSprint(f[p.sprintFieldID])
	}
	if p.storyPoints != "" {
		var pts float64
		if decode(f[p.storyPoints], &pts) {
			iss.StoryPoints = pts
		}
	}
	return iss
}

// --- decode helpers -------------------------------------------------------

func decode(raw json.RawMessage, out any) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func decodeString(raw json.RawMessage) string {
	var s string
	decode(raw, &s)
	return s
}

func decodeStrings(raw json.RawMessage) []string {
	var s []string
	decode(raw, &s)
	return s
}

func decodeComments(raw json.RawMessage) []Comment {
	var wrap struct {
		Comments []struct {
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Body json.RawMessage `json:"body"`
		} `json:"comments"`
	}
	if !decode(raw, &wrap) {
		return nil
	}
	// The inline `comment` field is the first (oldest-first) page Jira embeds in
	// the issue; taking the tail yields the newest of what's present. This is
	// only fully accurate when the issue isn't truncated — the launch flow
	// refreshes the selected issue via RecentComments for guaranteed recency.
	all := wrap.Comments
	if len(all) > maxComments {
		all = all[len(all)-maxComments:]
	}
	out := make([]Comment, 0, len(all))
	for _, c := range all {
		out = append(out, Comment{Author: c.Author.DisplayName, Body: renderADF(c.Body)})
	}
	return out
}

// RecentComments fetches the most recent comments for an issue, ordered newest
// first by Jira and returned oldest-first for natural reading. Used at launch
// time to guarantee the agent context has the genuinely latest comments.
func (c *Client) RecentComments(ctx context.Context, key string, max int) ([]Comment, error) {
	q := url.Values{}
	q.Set("orderBy", "-created")
	q.Set("maxResults", fmt.Sprint(max))
	var wrap struct {
		Comments []struct {
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Body json.RawMessage `json:"body"`
		} `json:"comments"`
	}
	if err := c.do(ctx, http.MethodGet, "/rest/api/3/issue/"+url.PathEscape(key)+"/comment", q, nil, &wrap); err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(wrap.Comments))
	for _, cm := range wrap.Comments {
		out = append(out, Comment{Author: cm.Author.DisplayName, Body: renderADF(cm.Body)})
	}
	// orderBy=-created gives newest-first; reverse to chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// decodeSprint reads the sprint custom field (array of sprint objects) and
// returns the active sprint name, falling back to the last entry.
func decodeSprint(raw json.RawMessage) string {
	var sprints []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	if !decode(raw, &sprints) || len(sprints) == 0 {
		return ""
	}
	for _, s := range sprints {
		if strings.EqualFold(s.State, "active") {
			return s.Name
		}
	}
	return sprints[len(sprints)-1].Name
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// extractAcceptanceCriteria pulls the content under an "Acceptance Criteria"
// (or "Critérios de aceite") heading from the rendered description.
func extractAcceptanceCriteria(md string) string {
	lines := strings.Split(md, "\n")
	start := -1
	for i, l := range lines {
		t := strings.ToLower(strings.TrimLeft(l, "# "))
		if strings.HasPrefix(l, "#") && (strings.Contains(t, "acceptance criteria") ||
			strings.Contains(t, "critérios de aceite") || strings.Contains(t, "criterios de aceite")) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	var out []string
	for _, l := range lines[start:] {
		if strings.HasPrefix(l, "#") {
			break // next heading ends the section
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
