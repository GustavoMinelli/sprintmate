package tui

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/jira"

	// register agents so agentSet is non-empty in tests
	_ "github.com/GustavoMinelli/sprintmate/internal/agents/claude"
	_ "github.com/GustavoMinelli/sprintmate/internal/agents/codex"
)

func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		r := []rune(s)[0]
		return tea.KeyPressMsg{Code: r, Text: s}
	}
}

func validCfg() *config.Config {
	c := config.Default()
	c.Jira.Host = "https://empresa.atlassian.net"
	c.Jira.Email = "voce@empresa.com"
	c.Jira.Token = "tok"
	c.Jira.Board = "Sprint Board"
	c.Workdir = os.TempDir() // an existing dir so launch validation passes
	return c
}

func TestKeyPressStrings(t *testing.T) {
	// Sanity-check that our synthetic keys produce the expected String(), which
	// is what key.Matches compares against.
	cases := map[string]string{"enter": "enter", "tab": "tab", "r": "r"}
	for in, want := range cases {
		if got := keyPress(in).String(); got != want {
			t.Errorf("keyPress(%q).String() = %q, want %q", in, got, want)
		}
	}
}

func TestIssueItem(t *testing.T) {
	it := issueItem{issue: jira.Issue{Key: "DEMO-1", Title: "Login", Status: "Doing", Priority: "High", StoryPoints: 3}}
	if it.Title() != "DEMO-1  Login" {
		t.Errorf("Title = %q", it.Title())
	}
	if it.Description() != "Doing · High · 3pts" {
		t.Errorf("Description = %q", it.Description())
	}
	if it.FilterValue() != "DEMO-1 Login" {
		t.Errorf("FilterValue = %q", it.FilterValue())
	}
}

func TestSourceFromConfig(t *testing.T) {
	c := validCfg()
	c.Jira.Columns = []string{"To Do"}
	c.Jira.Fields.Sprint = "customfield_1"
	src := sourceFromConfig(c)
	if src.Board != "Sprint Board" || src.SprintFieldID != "customfield_1" || len(src.Columns) != 1 {
		t.Errorf("source = %+v", src)
	}
}

func TestDashboardLoadAndLaunch(t *testing.T) {
	d := newDashboard(validCfg(), "dev")
	d, _ = d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	res := jira.Result{
		Issues:      []jira.Issue{{Key: "DEMO-1", Title: "Login", ProjectKey: "DEMO"}},
		SprintLabel: "Sprint 42",
	}
	d, _ = d.Update(issuesLoadedMsg{res: res})
	if d.loading {
		t.Fatal("should not be loading after issuesLoadedMsg")
	}
	if d.sprintLabel != "Sprint 42" {
		t.Errorf("sprintLabel = %q", d.sprintLabel)
	}
	it, ok := d.list.SelectedItem().(issueItem)
	if !ok || it.issue.Key != "DEMO-1" {
		t.Fatalf("selected item = %v, %v", it, ok)
	}

	// switch agent (claude -> codex)
	first := d.currentAgent()
	d, _ = d.Update(keyPress("tab"))
	if d.currentAgent() == first {
		t.Errorf("tab should switch agent from %q", first)
	}

	// The in-place strategy hands off through the root: enter -> launchMsg.
	d.cfg.Launch.Strategy = config.StrategyInplace
	d2, cmd := d.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("enter should return a command")
	}
	lm, ok := cmd().(launchMsg)
	if !ok || lm.issue.Key != "DEMO-1" {
		t.Fatalf("expected launchMsg for DEMO-1, got %#v", cmd())
	}
	if d2.launching {
		t.Error("in-place hand-off should not mark the dashboard as launching")
	}
}

func TestDashboardWindowedLaunchKeepsDashboard(t *testing.T) {
	c := validCfg()
	c.Launch.Strategy = config.StrategyWindow // launches in its own window
	d := newDashboard(c, "dev")
	d, _ = d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	d, _ = d.Update(issuesLoadedMsg{res: jira.Result{
		Issues: []jira.Issue{{Key: "DEMO-1", Title: "Login", ProjectKey: "DEMO"}},
	}})

	// A windowed launch runs in the background and keeps the dashboard open:
	// enter sets the launching state and returns a command, but does NOT hand
	// off via launchMsg (which would quit). We don't run the command here — it
	// would open a real terminal window.
	d2, cmd := d.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("enter should return a launch command")
	}
	if !d2.launching {
		t.Error("windowed launch should mark the dashboard as launching")
	}
	if d2.status == "" {
		t.Error("windowed launch should show a status message")
	}

	// The outcome arrives as launchedMsg and the dashboard stays put, ready for
	// the next demanda.
	d3, _ := d2.Update(launchedMsg{key: "DEMO-1", agent: "claude"})
	if d3.launching {
		t.Error("dashboard should clear launching after launchedMsg")
	}
	if d3.status == "" {
		t.Error("a successful launch should leave a confirmation status")
	}
}

func TestDashboardSettingsKey(t *testing.T) {
	d := newDashboard(validCfg(), "dev")
	d, _ = d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	d, _ = d.Update(issuesLoadedMsg{res: jira.Result{}})
	_, cmd := d.Update(keyPress("s"))
	if cmd == nil {
		t.Fatal("s should return a command")
	}
	if _, ok := cmd().(openSettingsMsg); !ok {
		t.Fatalf("expected openSettingsMsg, got %#v", cmd())
	}
}

func TestWizardTransitions(t *testing.T) {
	w := newWizard(config.Default(), false)
	w.client = jira.New("h", "e", "t")

	w, _ = w.Update(connTestedMsg{})
	if !w.loading || w.err != "" {
		t.Fatalf("after conn ok: loading=%v err=%q", w.loading, w.err)
	}

	w, _ = w.Update(boardsLoadedMsg{boards: []jira.Board{{ID: 1, Name: "B"}}})
	if w.step != stepBoard || w.loading {
		t.Fatalf("expected stepBoard, got step=%d loading=%v", w.step, w.loading)
	}

	w, _ = w.Update(columnsLoadedMsg{cols: []jira.Column{{Name: "To Do"}, {Name: "Doing"}}})
	if w.step != stepColumns {
		t.Fatalf("expected stepColumns, got %d", w.step)
	}
	if !w.colCheck[0] || !w.colCheck[1] {
		t.Errorf("columns should default to all-selected: %v", w.colCheck)
	}
}

func TestWizardConnError(t *testing.T) {
	w := newWizard(config.Default(), false)
	w.client = jira.New("h", "e", "t")
	w, _ = w.Update(connTestedMsg{err: jira.ErrAuth})
	if w.err == "" || w.loading {
		t.Errorf("conn error should surface and stop loading: err=%q loading=%v", w.err, w.loading)
	}
}

func TestWizardFinishSavesConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	w := newWizard(config.Default(), false)
	w.inputs[0].SetValue("https://empresa.atlassian.net")
	w.inputs[1].SetValue("voce@empresa.com")
	w.inputs[2].SetValue("secret")
	w.boards = []jira.Board{{ID: 1, Name: "Sprint Board"}}
	w.columns = []jira.Column{{Name: "To Do"}, {Name: "Doing"}}
	w.colCheck = map[int]bool{0: true, 1: false}
	w.sprintCur = 0
	w.workdir.SetValue(t.TempDir())

	_, cmd := w.finish()
	if cmd == nil {
		t.Fatal("finish should return a command")
	}
	done, ok := cmd().(wizardDoneMsg)
	if !ok {
		t.Fatalf("expected wizardDoneMsg, got %#v", cmd())
	}
	if done.cfg.Jira.Board != "Sprint Board" {
		t.Errorf("board = %q", done.cfg.Jira.Board)
	}
	if len(done.cfg.Jira.Columns) != 1 || done.cfg.Jira.Columns[0] != "To Do" {
		t.Errorf("columns = %v", done.cfg.Jira.Columns)
	}

	// persisted and reloadable
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.Jira.Email != "voce@empresa.com" {
		t.Errorf("reloaded email = %q", loaded.Jira.Email)
	}
}

func TestRootScreenSwitching(t *testing.T) {
	m := newModel(validCfg(), false, "dev")
	if m.screen != screenDashboard {
		t.Fatalf("valid config should start on dashboard, got %d", m.screen)
	}
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(model)

	mm, _ := m.Update(openSettingsMsg{})
	if mm.(model).screen != screenWizard {
		t.Fatal("openSettingsMsg should switch to wizard")
	}

	back, _ := mm.(model).Update(wizardDoneMsg{cfg: validCfg()})
	if back.(model).screen != screenDashboard {
		t.Fatal("wizardDoneMsg should switch to dashboard")
	}
}

func TestNewModelStartsWizardWhenInvalid(t *testing.T) {
	if m := newModel(nil, false, "dev"); m.screen != screenWizard {
		t.Error("nil config should start in wizard")
	}
	if m := newModel(config.Default(), false, "dev"); m.screen != screenWizard {
		t.Error("incomplete config should start in wizard")
	}
}

// Update must satisfy tea.Model (compile-time check).
var _ tea.Model = model{}
