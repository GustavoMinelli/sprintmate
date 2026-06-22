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

func TestTransitionsAndApply(t *testing.T) {
	var posted struct {
		Transition struct {
			ID string `json:"id"`
		} `json:"transition"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/transitions") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"transitions": []map[string]any{
					{"id": "11", "name": "Start progress", "to": map[string]string{"name": "In Progress"}},
					{"id": "21", "name": "Done", "to": map[string]string{"name": "Done"}},
				},
			})
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &posted)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	ts, err := c.Transitions(context.Background(), "DEMO-1")
	if err != nil {
		t.Fatalf("Transitions: %v", err)
	}
	if len(ts) != 2 || ts[0].To != "In Progress" || ts[0].ID != "11" {
		t.Fatalf("unexpected transitions: %+v", ts)
	}
	if err := c.ApplyTransition(context.Background(), "DEMO-1", "11"); err != nil {
		t.Fatalf("ApplyTransition: %v", err)
	}
	if posted.Transition.ID != "11" {
		t.Errorf("posted transition id = %q, want 11", posted.Transition.ID)
	}
}
