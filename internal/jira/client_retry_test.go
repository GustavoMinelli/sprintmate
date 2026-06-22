package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A 429 with Retry-After must be retried, not surfaced as a fatal error.
func TestDoRetriesOn429(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(Myself{AccountID: "ok"})
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	me, err := c.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if me.AccountID != "ok" || calls != 2 {
		t.Errorf("accountID=%q calls=%d (want ok / 2)", me.AccountID, calls)
	}
}

// A persistent 429 eventually gives up after the bounded retries.
func TestDoGivesUpAfterMaxRetries(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New(srv.URL, "e", "t")
	if _, err := c.TestConnection(context.Background()); err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if calls != maxRetries+1 {
		t.Errorf("attempts = %d, want %d", calls, maxRetries+1)
	}
}

func TestApiMessageExtraction(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"errorMessages", `{"errorMessages":["Bad JQL"]}`, "Bad JQL"},
		{"errorsMap", `{"errors":{"jql":"invalid"}}`, "jql: invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := c.body
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(body))
			}))
			defer srv.Close()
			cl := New(srv.URL, "e", "t")
			_, err := cl.TestConnection(context.Background())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want to contain %q", err, c.want)
			}
		})
	}
}

func TestApiMessageTruncatesLongBody(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := apiMessage([]byte(long))
	if len([]rune(got)) != 200 {
		t.Errorf("expected truncation to 200 runes, got %d", len([]rune(got)))
	}
}

func TestNewUpgradesRemoteHTTPToHTTPS(t *testing.T) {
	cases := map[string]string{
		"http://jira.example.com":   "https://jira.example.com",
		"jira.example.com":          "https://jira.example.com",
		"https://jira.example.com/": "https://jira.example.com",
		"http://localhost:8080":     "http://localhost:8080", // loopback left intact
		"http://127.0.0.1:9000":     "http://127.0.0.1:9000",
	}
	for in, want := range cases {
		if got := New(in, "e", "t").Host(); got != want {
			t.Errorf("New(%q).Host() = %q, want %q", in, got, want)
		}
	}
}
