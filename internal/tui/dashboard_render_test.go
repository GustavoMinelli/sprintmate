package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/GustavoMinelli/sprintmate/internal/jira"

	_ "github.com/GustavoMinelli/sprintmate/internal/agents/claude"
	_ "github.com/GustavoMinelli/sprintmate/internal/agents/codex"
)

func loadedDashboard(t *testing.T, w, h int) dashboard {
	t.Helper()
	d := newDashboard(validCfg(), "dev")
	d, _ = d.Update(tea.WindowSizeMsg{Width: w, Height: h})
	d, _ = d.Update(issuesLoadedMsg{res: jira.Result{
		BoardName:   "Sprint Board",
		SprintLabel: "Sprint 42",
		Issues: []jira.Issue{{
			Key: "DEMO-1", Title: "Login screen", Status: "In Progress",
			Priority: "High", StoryPoints: 5, Assignee: "Gustavo",
			Labels: []string{"backend", "auth"}, Description: "Rework the login flow end to end.",
			ProjectKey: "DEMO", Project: "Demo",
		}},
	}})
	return d
}

// TestDashboardWideShowsDetailPanel: on a wide terminal the master-detail panel
// renders the selected issue's fields (Assignee/Description), which never appear
// in the single-column list.
func TestDashboardWideShowsDetailPanel(t *testing.T) {
	d := loadedDashboard(t, 140, 40)
	view := d.View(mascot{})

	if !strings.Contains(view, "Assignee:") {
		t.Error("wide dashboard should render the detail panel with an Assignee row")
	}
	if !strings.Contains(view, "Gustavo") {
		t.Error("detail panel should show the assignee display name")
	}
	if !strings.Contains(view, "login flow") {
		t.Error("detail panel should show the issue description")
	}
}

// TestDashboardNarrowSingleColumn: below the two-column threshold the detail
// panel is suppressed (no Assignee row), leaving the list full width.
func TestDashboardNarrowSingleColumn(t *testing.T) {
	d := loadedDashboard(t, 80, 24)
	view := d.View(mascot{})

	if strings.Contains(view, "Assignee:") {
		t.Error("narrow dashboard should NOT render the detail panel")
	}
	if !strings.Contains(view, "DEMO-1") {
		t.Error("narrow dashboard should still list the issue")
	}
}

// TestDashboardQueueStrip: the pushed queue snapshot surfaces in the strip.
func TestDashboardQueueStrip(t *testing.T) {
	d := loadedDashboard(t, 140, 40)

	idle := d.View(mascot{})
	if !strings.Contains(idle, "Queue idle") {
		t.Error("with no monitor the strip should read idle")
	}

	d.qstats = queueStats{running: 2, slots: 3, pending: 4, awaiting: 1, active: true}
	active := d.View(mascot{})
	if !strings.Contains(active, "2/3") {
		t.Errorf("active strip should show running/slots, got:\n%s", active)
	}
	if !strings.Contains(active, "Awaiting approval") {
		t.Error("active strip should show the awaiting-approval count")
	}
}

// TestDashboardTinyTerminalNoPanic: degenerate sizes must not panic.
func TestDashboardTinyTerminalNoPanic(t *testing.T) {
	for _, sz := range [][2]int{{0, 0}, {10, 6}, {1, 1}, {88, 18}} {
		d := loadedDashboard(t, sz[0], sz[1])
		_ = d.View(mascot{})
	}
}
