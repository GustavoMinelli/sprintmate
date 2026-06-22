package jira

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomFieldsDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/field" {
			http.NotFound(w, r)
			return
		}
		// Mixed case + the story-point alias must still match.
		w.Write([]byte(`[
			{"id":"customfield_1","name":"SPRINT"},
			{"id":"customfield_2","name":"Story point estimate"},
			{"id":"customfield_3","name":"Unrelated"}
		]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	sprint, points, err := c.customFields(context.Background())
	if err != nil {
		t.Fatalf("customFields: %v", err)
	}
	if sprint != "customfield_1" || points != "customfield_2" {
		t.Errorf("discovered sprint=%q points=%q", sprint, points)
	}
}

func TestResolveFieldIDsOverridesSkipDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("discovery should not be called when both ids are overridden (hit %s)", r.URL.Path)
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	sprint, points := c.resolveFieldIDs(context.Background(), Source{
		SprintFieldID: "cf_a", StoryPointsFieldID: "cf_b",
	})
	if sprint != "cf_a" || points != "cf_b" {
		t.Errorf("overrides not honored: %q %q", sprint, points)
	}
}

func TestResolveFieldIDsDiscoveryErrorPreservesOverrides(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // discovery fails
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	// One override present; discovery fails → the override survives, no error.
	sprint, points := c.resolveFieldIDs(context.Background(), Source{SprintFieldID: "cf_a"})
	if sprint != "cf_a" || points != "" {
		t.Errorf("best-effort discovery failed badly: %q %q", sprint, points)
	}
}
