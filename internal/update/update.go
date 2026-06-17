// Package update checks GitHub Releases for a newer SprintMate so the dashboard
// can show a small "vX available" notice. It never returns an error: a slow or
// failed check (offline, rate-limited, junk payload) is reported as "no update",
// so a network hiccup can't crash or block the TUI — it just shows nothing.
package update

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// repo is the GitHub "owner/name" whose releases we check.
const repo = "gustavodiasminelli/sprintmate"

// Check returns the latest released version and true when it is newer than
// current. It returns ("", false) on any error, when current is a dev build, or
// when already up to date — the caller should render nothing in those cases.
func Check(ctx context.Context, current string) (latest string, newer bool) {
	if isDev(current) {
		return "", false
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", false
	}
	if greater(rel.TagName, current) {
		return rel.TagName, true
	}
	return "", false
}

// isDev reports whether v is a non-release build we should not nag about.
func isDev(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || v == "dev" || v == "unknown"
}

// greater reports whether semantic version a is strictly newer than b. Both may
// carry a leading "v" and a "-prerelease"/"+build" suffix, which are ignored. A
// version that can't be parsed is treated as not-greater, so we never nag on junk.
func greater(a, b string) bool {
	av, ok1 := parse(a)
	bv, ok2 := parse(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if av[i] != bv[i] {
			return av[i] > bv[i]
		}
	}
	return false
}

// parse turns "v1.2.3", "1.2", "v1.2.3-rc1" etc. into [major, minor, patch].
// Missing components default to 0; it reports false when a component is not a
// non-negative integer or there are more than three of them.
func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i] // drop prerelease / build metadata
	}
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
