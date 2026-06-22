package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGreater(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v1.2.0", "v1.1.9", true},
		{"v1.0.0", "v1.0.0", false},
		{"1.0.1", "v1.0.0", true},       // leading "v" optional on either side
		{"v1.0.0", "v1.0.1", false},     // older is not greater
		{"v2.0.0", "v1.9.9", true},      // major dominates
		{"v1.2.3-rc1", "v1.2.3", false}, // prerelease metadata ignored → equal
		{"v1.3", "v1.2.9", true},        // missing patch defaults to 0
		{"v1.0.0", "garbage", false},    // unparseable → never greater
		{"garbage", "v1.0.0", false},
	}
	for _, c := range cases {
		if got := greater(c.a, c.b); got != c.want {
			t.Errorf("greater(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestCheckSkipsDevBuilds(t *testing.T) {
	// A dev build must short-circuit before any network call.
	for _, v := range []string{"", "dev", "unknown"} {
		if latest, newer := Check(t.Context(), v); newer || latest != "" {
			t.Errorf("Check(%q) = (%q, %v), want no update", v, latest, newer)
		}
	}
}

func TestCheckWith(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		current    string
		wantLatest string
		wantNewer  bool
	}{
		{"newer", 200, `{"tag_name":"v2.0.0"}`, "v1.0.0", "v2.0.0", true},
		{"same", 200, `{"tag_name":"v1.0.0"}`, "v1.0.0", "", false},
		{"older", 200, `{"tag_name":"v0.9.0"}`, "v1.0.0", "", false},
		{"server-error", 500, ``, "v1.0.0", "", false},
		{"garbage", 200, `not json`, "v1.0.0", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				w.Write([]byte(c.body))
			}))
			defer srv.Close()

			latest, newer := checkWith(context.Background(), srv.Client(), srv.URL, c.current)
			if latest != c.wantLatest || newer != c.wantNewer {
				t.Errorf("checkWith = (%q, %v), want (%q, %v)", latest, newer, c.wantLatest, c.wantNewer)
			}
		})
	}
}
