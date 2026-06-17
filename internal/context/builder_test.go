package issuecontext

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GustavoMinelli/sprintmate/internal/jira"
)

func TestOneLineTextRuneSafe(t *testing.T) {
	// Each "é" is 2 bytes; a byte-index cut at 280 would split a rune.
	out := oneLineText(strings.Repeat("é", 400))
	if !utf8.ValidString(out) {
		t.Error("truncation produced invalid UTF-8 (split a rune)")
	}
	if utf8.RuneCountInString(strings.TrimSuffix(out, "…")) != 280 {
		t.Errorf("expected 280 runes + ellipsis, got %d", utf8.RuneCountInString(out))
	}
}

func TestBuildWritesContextFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Demo\nLaravel + React"), 0o644)
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs", "arch.md"), []byte("arch"), 0o644)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	issue := jira.Issue{
		Key:                "DEMO-123",
		Title:              "Corrigir login social",
		Description:        "Descrição detalhada",
		Status:             "In Progress",
		Priority:           "High",
		Sprint:             "Sprint 42",
		StoryPoints:        3,
		Labels:             []string{"backend"},
		AcceptanceCriteria: "- login funciona",
		Comments:           []jira.Comment{{Author: "Ana", Body: "verificar oauth"}},
		URL:                "https://x/browse/DEMO-123",
	}

	path, err := NewBuilder().Build(context.Background(), issue, dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if filepath.Base(path) != Filename {
		t.Errorf("unexpected path %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out := string(data)
	for _, want := range []string{
		"# DEMO-123", "Corrigir login social", "Descrição detalhada",
		"Critérios de aceite", "login funciona", "README", "Laravel + React",
		"docs/arch.md", "src/", "Sprint 42", "Story Points: 3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("context missing %q\n---\n%s", want, out)
		}
	}
}

func TestBuildEmptyIssueDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewBuilder().Build(context.Background(), jira.Issue{Key: "X-1"}, dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
}
