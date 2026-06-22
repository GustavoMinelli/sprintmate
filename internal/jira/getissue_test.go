package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/field":
			json.NewEncoder(w).Encode([]map[string]any{}) // no custom fields
		case strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/DEMO-7"):
			json.NewEncoder(w).Encode(map[string]any{
				"key": "DEMO-7",
				"fields": map[string]any{
					"summary":  "Fix the thing",
					"status":   map[string]any{"id": "3", "name": "In Progress"},
					"priority": map[string]any{"name": "High"},
					"labels":   []string{"backend"},
				},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	issue, err := New(srv.URL, "e", "t").GetIssue(context.Background(), "DEMO-7")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Key != "DEMO-7" || issue.Title != "Fix the thing" {
		t.Errorf("unexpected issue %+v", issue)
	}
	if issue.Status != "In Progress" || issue.Priority != "High" {
		t.Errorf("status/priority = %q/%q", issue.Status, issue.Priority)
	}
	if issue.URL != srv.URL+"/browse/DEMO-7" {
		t.Errorf("URL = %q", issue.URL)
	}
}
