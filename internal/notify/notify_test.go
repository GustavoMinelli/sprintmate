package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendPostsWebhook(t *testing.T) {
	got := make(chan map[string]string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(body, &m)
		got <- m
	}))
	defer srv.Close()

	Send(context.Background(), Options{Title: "Done", Body: "DEMO-1 finished", Webhook: srv.URL})

	select {
	case m := <-got:
		if m["title"] != "Done" || m["body"] != "DEMO-1 finished" {
			t.Errorf("webhook payload = %v", m)
		}
	default:
		t.Error("webhook was not called")
	}
}

func TestSendNoChannelsIsNoop(t *testing.T) {
	// Should not panic or block when nothing is enabled.
	Send(context.Background(), Options{Title: "x", Body: "y"})
}
