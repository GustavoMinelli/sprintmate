// Package issuecontext builds the .issue-context.md file handed to the AI
// agent: it combines the Jira issue, a snapshot of the project (README, docs,
// top-level layout) and recent git activity into a single markdown document.
package issuecontext

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/GustavoMinelli/sprintmate/internal/git"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
)

// Filename is the context file written into the project directory.
const Filename = ".issue-context.md"

// Builder renders issue context. The zero value is usable; NewBuilder applies
// sensible limits.
type Builder struct {
	MaxReadmeBytes int
	MaxCommits     int
	MaxTreeEntries int
}

// NewBuilder returns a Builder with default limits.
func NewBuilder() *Builder {
	return &Builder{MaxReadmeBytes: 4000, MaxCommits: 10, MaxTreeEntries: 40}
}

// ignored directories that add noise to the project snapshot.
var ignoredDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".idea": true,
	".vscode": true, "dist": true, "build": true, "target": true,
}

// Build writes Filename into projectDir and returns its path.
func (b *Builder) Build(ctx context.Context, issue jira.Issue, projectDir string) (string, error) {
	md := b.render(ctx, issue, projectDir)
	path := filepath.Join(projectDir, Filename)
	// 0o600: the file aggregates the full issue (description, comments, author
	// names) and may contain sensitive data, so keep it owner-only — matching
	// how config.Save guards the API token.
	if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
		return "", fmt.Errorf("writing context file: %w", err)
	}
	return path, nil
}

// Render returns the markdown without writing it (useful for tests/preview).
func (b *Builder) Render(ctx context.Context, issue jira.Issue, projectDir string) string {
	return b.render(ctx, issue, projectDir)
}

func (b *Builder) render(ctx context.Context, issue jira.Issue, projectDir string) string {
	var s strings.Builder
	fmt.Fprintf(&s, "# %s\n\n", issue.Key)

	fmt.Fprintf(&s, "## Título\n%s\n\n", fallback(issue.Title, "(sem título)"))

	section(&s, "Status", oneLine(issue.Status, issue.Priority, issue.Sprint))
	if issue.StoryPoints > 0 {
		fmt.Fprintf(&s, "Story Points: %g\n\n", issue.StoryPoints)
	}
	if len(issue.Labels) > 0 {
		fmt.Fprintf(&s, "Labels: %s\n\n", strings.Join(issue.Labels, ", "))
	}

	section(&s, "Descrição", fallback(issue.Description, "(sem descrição)"))
	if issue.AcceptanceCriteria != "" {
		section(&s, "Critérios de aceite", issue.AcceptanceCriteria)
	}

	if len(issue.Comments) > 0 {
		s.WriteString("## Comentários\n")
		for _, c := range issue.Comments {
			fmt.Fprintf(&s, "- **%s**: %s\n", fallback(c.Author, "?"), oneLineText(c.Body))
		}
		s.WriteString("\n")
	}

	// Project snapshot
	s.WriteString("## Projeto\n")
	fmt.Fprintf(&s, "Diretório: `%s`\n\n", projectDir)
	if tree := b.topLevel(projectDir); tree != "" {
		fmt.Fprintf(&s, "Estrutura principal:\n```\n%s\n```\n\n", tree)
	}
	if docs := b.listDocs(projectDir); docs != "" {
		fmt.Fprintf(&s, "Documentação (`docs/`):\n%s\n\n", docs)
	}
	if readme := b.readREADME(projectDir); readme != "" {
		section(&s, "README", readme)
	}

	// Git context
	if git.IsRepo(ctx, projectDir) {
		s.WriteString("## Git\n")
		if branch, err := git.CurrentBranch(ctx, projectDir); err == nil {
			fmt.Fprintf(&s, "Branch atual: `%s`\n\n", branch)
		}
		if commits, err := git.RecentCommits(ctx, projectDir, b.MaxCommits); err == nil && len(commits) > 0 {
			s.WriteString("Últimos commits:\n")
			for _, c := range commits {
				fmt.Fprintf(&s, "- %s\n", c)
			}
			s.WriteString("\n")
		}
	}

	fmt.Fprintf(&s, "---\n_Gerado pelo SprintMate. Issue: %s_\n", issue.URL)
	return s.String()
}

func (b *Builder) topLevel(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || ignoredDirs[name] {
			continue
		}
		if e.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > b.MaxTreeEntries {
		extra := len(names) - b.MaxTreeEntries
		names = names[:b.MaxTreeEntries]
		names = append(names, fmt.Sprintf("... (+%d)", extra))
	}
	return strings.Join(names, "\n")
}

func (b *Builder) listDocs(dir string) string {
	docsDir := filepath.Join(dir, "docs")
	info, err := os.Stat(docsDir)
	if err != nil || !info.IsDir() {
		return ""
	}
	var files []string
	_ = filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		files = append(files, "- "+rel)
		if len(files) >= b.MaxTreeEntries {
			return filepath.SkipAll
		}
		return nil
	})
	return strings.Join(files, "\n")
}

func (b *Builder) readREADME(dir string) string {
	for _, name := range []string{"README.md", "README.MD", "readme.md", "Readme.md", "README"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if len(data) > b.MaxReadmeBytes {
			// Back up to a valid UTF-8 boundary so we never split a rune.
			cut := b.MaxReadmeBytes
			for cut > 0 && !utf8.RuneStart(data[cut]) {
				cut--
			}
			return strings.TrimSpace(string(data[:cut])) + "\n... (truncado)"
		}
		return strings.TrimSpace(string(data))
	}
	return ""
}

func section(s *strings.Builder, title, body string) {
	fmt.Fprintf(s, "## %s\n%s\n\n", title, body)
}

func oneLine(parts ...string) string {
	var kept []string
	labels := []string{"Status", "Prioridade", "Sprint"}
	for i, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, labels[i]+": "+p)
		}
	}
	if len(kept) == 0 {
		return "-"
	}
	return strings.Join(kept, " | ")
}

func oneLineText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return truncateRunes(s, 280, "…")
}

// truncateRunes shortens s to at most n runes (never splitting a multibyte
// rune), appending suffix when truncated.
func truncateRunes(s string, n int, suffix string) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + suffix
}

func fallback(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
