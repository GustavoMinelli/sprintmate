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

// New builds a client. host is normalized (scheme added, trailing slash trimmed).
func New(host, email, token string) *Client {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host != "" && !strings.Contains(host, "://") {
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

// do performs an authenticated request and decodes a JSON response into out.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.host + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(c.email + ":" + c.token))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("requesting %s: %w", path, err)
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrAuth
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
