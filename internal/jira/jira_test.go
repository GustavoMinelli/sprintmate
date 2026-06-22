package jira

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderADF(t *testing.T) {
	doc := `{"type":"doc","content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"Hello "},
			{"type":"text","text":"world","marks":[{"type":"strong"}]}
		]},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"two"}]}]}
		]}
	]}`
	got := renderADF(json.RawMessage(doc))
	if !strings.Contains(got, "Hello **world**") {
		t.Errorf("missing bold text:\n%s", got)
	}
	if !strings.Contains(got, "- one") || !strings.Contains(got, "- two") {
		t.Errorf("missing list items:\n%s", got)
	}
}

func TestRenderADFPlainString(t *testing.T) {
	if got := renderADF(json.RawMessage(`"just text"`)); got != "just text" {
		t.Errorf("plain string fallback failed: %q", got)
	}
	if got := renderADF(nil); got != "" {
		t.Errorf("nil should render empty, got %q", got)
	}
}

func TestBuildJQL(t *testing.T) {
	cases := []struct {
		assignee string
		statuses []string
		want     string
	}{
		{"currentUser", []string{"1", "2"}, "assignee = currentUser() AND status in (1, 2) ORDER BY priority DESC"},
		{"all", nil, "ORDER BY priority DESC"},
		{"5557", nil, `assignee = "5557" ORDER BY priority DESC`},
	}
	for _, c := range cases {
		if got := buildJQL(c.assignee, c.statuses); got != c.want {
			t.Errorf("buildJQL(%q,%v) = %q, want %q", c.assignee, c.statuses, got, c.want)
		}
	}
}

func TestSelectStatusIDs(t *testing.T) {
	cols := []Column{
		{Name: "To Do", StatusIDs: []string{"1"}},
		{Name: "In Progress", StatusIDs: []string{"2", "3"}},
		{Name: "Done", StatusIDs: []string{"4"}},
	}
	got := selectStatusIDs(cols, []string{"To Do", "In Progress"})
	if strings.Join(got, ",") != "1,2,3" {
		t.Errorf("selected = %v", got)
	}
	if all := selectStatusIDs(cols, nil); len(all) != 4 {
		t.Errorf("empty selection should include all, got %v", all)
	}
}

func TestExtractAcceptanceCriteria(t *testing.T) {
	md := "Some intro\n\n## Acceptance Criteria\n- a\n- b\n\n## Notes\nignore"
	got := extractAcceptanceCriteria(md)
	if !strings.Contains(got, "- a") || strings.Contains(got, "ignore") {
		t.Errorf("bad extraction: %q", got)
	}
	if got := extractAcceptanceCriteria("no headings here"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestDecodeSprintPicksActive(t *testing.T) {
	raw := json.RawMessage(`[{"name":"Sprint 41","state":"closed"},{"name":"Sprint 42","state":"active"}]`)
	if got := decodeSprint(raw); got != "Sprint 42" {
		t.Errorf("decodeSprint = %q", got)
	}
}

func TestClientAuthAndEndpoints(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/rest/api/3/myself":
			json.NewEncoder(w).Encode(Myself{AccountID: "abc", DisplayName: "Dev"})
		case r.URL.Path == "/rest/agile/1.0/board":
			json.NewEncoder(w).Encode(map[string]any{
				"values": []Board{{ID: 1, Name: "Sprint Board"}}, "isLast": true,
			})
		case strings.HasSuffix(r.URL.Path, "/configuration"):
			w.Write([]byte(`{"columnConfig":{"columns":[{"name":"To Do","statuses":[{"id":"10"}]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "voce@empresa.com", "tok")
	ctx := context.Background()

	me, err := c.TestConnection(ctx)
	if err != nil || me.AccountID != "abc" {
		t.Fatalf("TestConnection: %v, %+v", err, me)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got %q", gotAuth)
	}

	b, err := c.ResolveBoard(ctx, "Sprint Board")
	if err != nil || b.ID != 1 {
		t.Fatalf("ResolveBoard: %v, %+v", err, b)
	}

	cols, err := c.BoardColumns(ctx, 1)
	if err != nil || len(cols) != 1 || cols[0].StatusIDs[0] != "10" {
		t.Fatalf("BoardColumns: %v, %+v", err, cols)
	}
}

func TestClientAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := New(srv.URL, "x", "y")
	if _, err := c.TestConnection(context.Background()); err != ErrAuth {
		t.Errorf("expected ErrAuth, got %v", err)
	}
}

func TestClientHTMLResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<!DOCTYPE html><html><body>Sign in</body></html>"))
	}))
	defer srv.Close()
	c := New(srv.URL, "x", "y")
	_, err := c.TestConnection(context.Background())
	if err == nil {
		t.Fatal("expected an error for an HTML response")
	}
	if !strings.Contains(err.Error(), "HTML page") {
		t.Errorf("error should explain the HTML page, got %q", err)
	}
	if strings.Contains(err.Error(), "invalid character") {
		t.Errorf("error should not surface the raw JSON-decode failure, got %q", err)
	}
}

func TestAddComment(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	if err := c.AddComment(context.Background(), "DEMO-1", "Started via SprintMate"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/rest/api/3/issue/DEMO-1/comment" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"type":"doc"`) || !strings.Contains(gotBody, "Started via SprintMate") {
		t.Errorf("body should be an ADF doc containing the text, got: %s", gotBody)
	}
}

func TestNewNormalizesHost(t *testing.T) {
	c := New("empresa.atlassian.net/", "e", "t")
	if c.Host() != "https://empresa.atlassian.net" {
		t.Errorf("host = %q", c.Host())
	}
}
