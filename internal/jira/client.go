package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrAuth is returned when Jira rejects the credentials (401/403).
var ErrAuth = errors.New("jira authentication failed: check host, email and API token")

// Client is a minimal Jira Cloud REST/Agile API client using Basic auth.
type Client struct {
	host  string
	email string
	token string
	http  *http.Client
}

// New builds a client. host is normalized (scheme added, trailing slash
// trimmed). A plain http:// host pointing at a remote server is upgraded to
// https:// so the Basic-auth credentials are never sent in cleartext; http to
// loopback (localhost/127.0.0.1) is left intact for local dev and testing.
func New(host, email, token string) *Client {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	switch {
	case host == "":
	case strings.HasPrefix(host, "http://"):
		if !isLoopbackHost(host) {
			host = "https://" + strings.TrimPrefix(host, "http://")
		}
	case !strings.Contains(host, "://"):
		host = "https://" + host
	}
	return &Client{
		host:  host,
		email: email,
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Host returns the normalized base URL.
func (c *Client) Host() string { return c.host }

// BrowseURL returns the human-facing URL for an issue key.
func (c *Client) BrowseURL(key string) string {
	return c.host + "/browse/" + key
}

// maxRetries bounds how many times do() retries a rate-limited or transient
// server error before giving up.
const maxRetries = 3

// do performs an authenticated request and decodes a JSON response into out.
// A 429 (rate limit) or transient 5xx is retried a bounded number of times,
// honoring any Retry-After header and the context deadline.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.host + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		bodyBytes = b
	}

	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, reader)
		if err != nil {
			return fmt.Errorf("building request: %w", err)
		}
		auth := base64.StdEncoding.EncodeToString([]byte(c.email + ":" + c.token))
		req.Header.Set("Authorization", "Basic "+auth)
		req.Header.Set("Accept", "application/json")
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("requesting %s: %w", path, err)
		}

		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return ErrAuth
		}

		// Retry rate limits and transient upstream errors with backoff.
		if attempt < maxRetries && retryable(resp.StatusCode) {
			if err := wait(ctx, backoff(resp.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}

		if readErr != nil {
			return fmt.Errorf("reading response from %s: %w", path, readErr)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("jira %s %s: %s: %s", method, path, resp.Status, apiMessage(data))
		}
		if out != nil && len(data) > 0 {
			if looksLikeHTML(resp.Header.Get("Content-Type"), data) {
				return fmt.Errorf("jira %s %s returned an HTML page instead of JSON (status %s from %s): "+
					"check that the host is your Atlassian site (e.g. https://your-domain.atlassian.net) and "+
					"that requests aren't being redirected to an SSO/login or proxy page", method, path, resp.Status, resp.Request.URL.Host)
			}
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("decoding response from %s: %w", path, err)
			}
		}
		return nil
	}
}

// retryable reports whether a status code is worth retrying.
func retryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// backoff computes the delay before the next attempt, preferring a Retry-After
// header (seconds) and otherwise using exponential backoff capped at 8s.
func backoff(retryAfter string, attempt int) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs >= 0 {
			d := time.Duration(secs) * time.Second
			if d > 30*time.Second {
				d = 30 * time.Second // never stall the TUI for too long
			}
			return d
		}
	}
	d := time.Duration(1<<attempt) * time.Second
	if d > 8*time.Second {
		d = 8 * time.Second
	}
	return d
}

// wait sleeps for d or returns early if the context is cancelled.
func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// looksLikeHTML reports whether a response body is an HTML/XML document rather
// than the JSON we asked for — the signature of a wrong host or an SSO/login or
// proxy page served with a 2xx status.
func looksLikeHTML(contentType string, data []byte) bool {
	if ct := strings.ToLower(contentType); strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		return true
	}
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '<'
}

// apiMessage extracts a readable message from a Jira error payload.
func apiMessage(data []byte) string {
	var e struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
	}
	if json.Unmarshal(data, &e) == nil {
		if len(e.ErrorMessages) > 0 {
			return strings.Join(e.ErrorMessages, "; ")
		}
		if len(e.Errors) > 0 {
			parts := make([]string, 0, len(e.Errors))
			for k, v := range e.Errors {
				parts = append(parts, k+": "+v)
			}
			return strings.Join(parts, "; ")
		}
	}
	s := strings.TrimSpace(string(data))
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200])
	}
	return s
}

// TestConnection validates credentials by fetching the current user.
func (c *Client) TestConnection(ctx context.Context) (Myself, error) {
	var m Myself
	err := c.do(ctx, http.MethodGet, "/rest/api/3/myself", nil, nil, &m)
	return m, err
}

// ListBoards returns all Agile boards visible to the user.
func (c *Client) ListBoards(ctx context.Context) ([]Board, error) {
	var boards []Board
	startAt := 0
	for {
		q := url.Values{}
		q.Set("startAt", fmt.Sprint(startAt))
		q.Set("maxResults", "50")
		var page struct {
			Values     []Board `json:"values"`
			IsLast     bool    `json:"isLast"`
			MaxResults int     `json:"maxResults"`
		}
		if err := c.do(ctx, http.MethodGet, "/rest/agile/1.0/board", q, nil, &page); err != nil {
			return nil, err
		}
		boards = append(boards, page.Values...)
		if page.IsLast || len(page.Values) == 0 {
			break
		}
		startAt += len(page.Values)
	}
	return boards, nil
}

// ResolveBoard finds a board by numeric id or by name (exact, then substring).
func (c *Client) ResolveBoard(ctx context.Context, nameOrID string) (Board, error) {
	nameOrID = strings.TrimSpace(nameOrID)
	boards, err := c.ListBoards(ctx)
	if err != nil {
		return Board{}, err
	}
	if isDigits(nameOrID) {
		for _, b := range boards {
			if fmt.Sprint(b.ID) == nameOrID {
				return b, nil
			}
		}
	}
	lower := strings.ToLower(nameOrID)
	for _, b := range boards {
		if strings.EqualFold(b.Name, nameOrID) {
			return b, nil
		}
	}
	for _, b := range boards {
		if strings.Contains(strings.ToLower(b.Name), lower) {
			return b, nil
		}
	}
	return Board{}, fmt.Errorf("board %q not found", nameOrID)
}

// BoardColumns returns the board's columns mapped to their status IDs.
func (c *Client) BoardColumns(ctx context.Context, boardID int) ([]Column, error) {
	var cfg struct {
		ColumnConfig struct {
			Columns []struct {
				Name     string `json:"name"`
				Statuses []struct {
					ID string `json:"id"`
				} `json:"statuses"`
			} `json:"columns"`
		} `json:"columnConfig"`
	}
	path := fmt.Sprintf("/rest/agile/1.0/board/%d/configuration", boardID)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &cfg); err != nil {
		return nil, err
	}
	cols := make([]Column, 0, len(cfg.ColumnConfig.Columns))
	for _, col := range cfg.ColumnConfig.Columns {
		ids := make([]string, 0, len(col.Statuses))
		for _, s := range col.Statuses {
			ids = append(ids, s.ID)
		}
		cols = append(cols, Column{Name: col.Name, StatusIDs: ids})
	}
	return cols, nil
}

// ListSprints returns the board's sprints. state may be "active", "future",
// "closed" or a comma-separated combination; empty means all.
func (c *Client) ListSprints(ctx context.Context, boardID int, state string) ([]Sprint, error) {
	var sprints []Sprint
	startAt := 0
	for {
		q := url.Values{}
		q.Set("startAt", fmt.Sprint(startAt))
		q.Set("maxResults", "50")
		if state != "" {
			q.Set("state", state)
		}
		var page struct {
			Values []Sprint `json:"values"`
			IsLast bool     `json:"isLast"`
		}
		path := fmt.Sprintf("/rest/agile/1.0/board/%d/sprint", boardID)
		if err := c.do(ctx, http.MethodGet, path, q, nil, &page); err != nil {
			return nil, err
		}
		sprints = append(sprints, page.Values...)
		if page.IsLast || len(page.Values) == 0 {
			break
		}
		startAt += len(page.Values)
	}
	return sprints, nil
}

// AddComment posts a plain-text comment to an issue. The Jira Cloud v3 API
// requires the body in Atlassian Document Format, so the text is wrapped in a
// minimal ADF document.
func (c *Client) AddComment(ctx context.Context, key, text string) error {
	body := map[string]any{"body": textToADF(text)}
	path := "/rest/api/3/issue/" + url.PathEscape(key) + "/comment"
	return c.do(ctx, http.MethodPost, path, nil, body, nil)
}

// textToADF builds a minimal ADF document from plain text: each non-empty line
// becomes its own paragraph.
func textToADF(text string) map[string]any {
	var paras []map[string]any
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		paras = append(paras, map[string]any{
			"type":    "paragraph",
			"content": []map[string]any{{"type": "text", "text": line}},
		})
	}
	if len(paras) == 0 {
		paras = []map[string]any{{"type": "paragraph"}}
	}
	return map[string]any{"type": "doc", "version": 1, "content": paras}
}

// isLoopbackHost reports whether an http:// URL points at the local machine,
// where cleartext is acceptable (local dev / tests).
func isLoopbackHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "::1" || strings.HasPrefix(h, "127.")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
