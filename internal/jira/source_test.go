package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// searchJQL must keep paginating while a nextPageToken is present, even across
// an empty intermediate page (regression for the early-break bug).
func TestSearchJQLPaginatesAcrossEmptyPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		calls++
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		token, _ := req["nextPageToken"].(string)
		switch token {
		case "":
			w.Write([]byte(`{"issues":[{"key":"A","fields":{}}],"nextPageToken":"t2"}`))
		case "t2": // empty page but more to come
			w.Write([]byte(`{"issues":[],"nextPageToken":"t3"}`))
		case "t3":
			w.Write([]byte(`{"issues":[{"key":"B","fields":{}}]}`))
		default:
			t.Errorf("unexpected token %q", token)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	raws, err := c.searchJQL(context.Background(), "assignee = currentUser()", []string{"summary"})
	if err != nil {
		t.Fatalf("searchJQL: %v", err)
	}
	if len(raws) != 2 || raws[0].Key != "A" || raws[1].Key != "B" {
		t.Fatalf("expected [A B], got %+v", raws)
	}
	if calls != 3 {
		t.Errorf("expected 3 pages, got %d", calls)
	}
}

// RecentComments fetches ordered newest-first and returns chronological order.
func TestRecentCommentsChronological(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/DEMO-1/comment" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("orderBy"); got != "-created" {
			t.Errorf("orderBy = %q, want -created", got)
		}
		// Jira returns newest-first under orderBy=-created.
		w.Write([]byte(`{"comments":[
			{"author":{"displayName":"C"},"body":"newest"},
			{"author":{"displayName":"B"},"body":"mid"},
			{"author":{"displayName":"A"},"body":"oldest"}
		]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	cs, err := c.RecentComments(context.Background(), "DEMO-1", 5)
	if err != nil {
		t.Fatalf("RecentComments: %v", err)
	}
	if len(cs) != 3 || cs[0].Body != "oldest" || cs[2].Body != "newest" {
		t.Fatalf("expected chronological [oldest..newest], got %+v", cs)
	}
}

// resolveSprint propagates ListSprints errors instead of silently changing the
// result set.
func TestResolveSprintPropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(srv.URL, "e", "t")
	if _, _, _, err := c.resolveSprint(context.Background(), 1, "active"); err == nil {
		t.Error("expected error from failing ListSprints to propagate")
	}
}
