package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// FetchIssues in board mode with an active sprint must resolve the board, its
// columns and the active sprint, then pull issues from the sprint-scoped path.
func TestFetchIssuesBoardActiveSprint(t *testing.T) {
	var issuePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			json.NewEncoder(w).Encode(map[string]any{
				"values": []Board{{ID: 1, Name: "Sprint Board"}}, "isLast": true,
			})
		case strings.HasSuffix(r.URL.Path, "/configuration"):
			w.Write([]byte(`{"columnConfig":{"columns":[{"name":"To Do","statuses":[{"id":"10"}]}]}}`))
		case strings.HasSuffix(r.URL.Path, "/sprint"):
			json.NewEncoder(w).Encode(map[string]any{
				"values": []Sprint{{ID: 99, Name: "Sprint 99", State: "active"}}, "isLast": true,
			})
		case strings.HasSuffix(r.URL.Path, "/issue"):
			issuePath = r.URL.Path
			w.Write([]byte(`{"total":1,"issues":[
				{"key":"DEMO-1","fields":{
					"summary":"Fix login",
					"status":{"id":"10","name":"To Do"},
					"priority":{"name":"High"},
					"project":{"key":"DEMO","name":"Demo"},
					"labels":["bug"]
				}}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	src := Source{
		Board: "Sprint Board", Sprint: "active", Columns: []string{"To Do"},
		Assignee: "currentUser", SprintFieldID: "cf_sprint", StoryPointsFieldID: "cf_pts",
	}
	res, err := c.FetchIssues(context.Background(), src)
	if err != nil {
		t.Fatalf("FetchIssues: %v", err)
	}
	if res.SprintLabel != "Sprint 99" || res.BoardName != "Sprint Board" {
		t.Errorf("label/board = %q / %q", res.SprintLabel, res.BoardName)
	}
	if !strings.Contains(issuePath, "/sprint/99/issue") {
		t.Errorf("expected sprint-scoped path, got %q", issuePath)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
	is := res.Issues[0]
	if is.Key != "DEMO-1" || is.Title != "Fix login" || is.Column != "To Do" || is.Priority != "High" || is.Project != "Demo" {
		t.Errorf("issue not normalized: %+v", is)
	}
}

// FetchIssues with a JQL override must hit the search endpoint and label "Custom JQL".
func TestFetchIssuesJQLOverride(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/search/jql" {
			hit = true
			w.Write([]byte(`{"issues":[{"key":"X-1","fields":{"summary":"hi"}}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	res, err := c.FetchIssues(context.Background(), Source{
		JQL: "project = X", SprintFieldID: "cf_sprint", StoryPointsFieldID: "cf_pts",
	})
	if err != nil {
		t.Fatalf("FetchIssues: %v", err)
	}
	if !hit || res.SprintLabel != "Custom JQL" || len(res.Issues) != 1 {
		t.Errorf("jql override: hit=%v label=%q issues=%d", hit, res.SprintLabel, len(res.Issues))
	}
}

func TestIssueParserColumnFallback(t *testing.T) {
	p := issueParser{
		client:       New("https://x.test", "e", "t"),
		columnByStat: map[string]string{"10": "To Do"},
	}
	// status id present in the column map → uses the mapped column name.
	mapped := p.one(rawIssue{Key: "A-1", Fields: map[string]json.RawMessage{
		"summary": json.RawMessage(`"t"`),
		"status":  json.RawMessage(`{"id":"10","name":"Backlog"}`),
	}})
	if mapped.Column != "To Do" {
		t.Errorf("mapped column = %q, want To Do", mapped.Column)
	}
	// status id absent → falls back to the status name.
	fallback := p.one(rawIssue{Key: "A-2", Fields: map[string]json.RawMessage{
		"status": json.RawMessage(`{"id":"99","name":"Review"}`),
	}})
	if fallback.Column != "Review" {
		t.Errorf("fallback column = %q, want Review", fallback.Column)
	}
	// empty fields map must not panic.
	_ = p.one(rawIssue{Key: "A-3", Fields: map[string]json.RawMessage{}})
}

// decodeComments keeps only the newest maxComments, preserving their order.
func TestDecodeCommentsKeepsNewest(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"comments":[`)
	for i := 1; i <= 7; i++ {
		if i > 1 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"author":{"displayName":"u%d"},"body":"c%d"}`, i, i)
	}
	sb.WriteString(`]}`)

	got := decodeComments(json.RawMessage(sb.String()))
	if len(got) != maxComments {
		t.Fatalf("kept %d comments, want %d", len(got), maxComments)
	}
	// 7 comments oldest-first → newest 5 are c3..c7 in order.
	if got[0].Body != "c3" || got[4].Body != "c7" {
		t.Errorf("kept wrong window: %+v", got)
	}
}

// resolveSprint covers the non-error branches.
func TestResolveSprintBranches(t *testing.T) {
	c := New("https://x.test", "e", "t") // numeric/empty branches make no network call
	if id, label, usePath, err := c.resolveSprint(context.Background(), 1, "42"); err != nil || id != 42 || label != "Sprint 42" || !usePath {
		t.Errorf("numeric: %d %q %v %v", id, label, usePath, err)
	}
	for _, spec := range []string{"", "all"} {
		if id, label, usePath, err := c.resolveSprint(context.Background(), 1, spec); err != nil || id != 0 || label != "" || usePath {
			t.Errorf("%q: %d %q %v %v", spec, id, label, usePath, err)
		}
	}
}
