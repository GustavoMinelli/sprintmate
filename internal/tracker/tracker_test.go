package tracker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// jiraWriter.TransitionTo must resolve a target status name to the matching
// transition id and POST it.
func TestJiraWriterTransitionTo(t *testing.T) {
	var appliedID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/transitions") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"transitions": []map[string]any{
					{"id": "11", "name": "Start progress", "to": map[string]string{"name": "In Progress"}},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/transitions") && r.Method == http.MethodPost:
			var body struct {
				Transition struct {
					ID string `json:"id"`
				} `json:"transition"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			appliedID = body.Transition.ID
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	w := NewJira(srv.URL, "e", "t")

	// match by destination status name
	if err := w.TransitionTo(context.Background(), "DEMO-1", "in progress"); err != nil {
		t.Fatalf("TransitionTo: %v", err)
	}
	if appliedID != "11" {
		t.Errorf("applied id = %q, want 11", appliedID)
	}

	// empty target is a no-op (must not error)
	if err := w.TransitionTo(context.Background(), "DEMO-1", ""); err != nil {
		t.Errorf("empty target should be a no-op, got %v", err)
	}

	// unknown target reports the available options
	if err := w.TransitionTo(context.Background(), "DEMO-1", "Released"); err == nil ||
		!strings.Contains(err.Error(), "In Progress") {
		t.Errorf("expected an error listing options, got %v", err)
	}
}

func TestJiraWriterCaps(t *testing.T) {
	if c := NewJira("h", "e", "t").Caps(); !c.Comment || !c.Transition {
		t.Errorf("jira caps = %+v, want both true", c)
	}
}
