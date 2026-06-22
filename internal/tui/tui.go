// Package tui implements the SprintMate terminal UI with Bubble Tea v2: a root
// model that switches between the setup wizard and the issue dashboard.
package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
)

type screen int

const (
	screenWizard screen = iota
	screenDashboard
)

var appStyle = lipgloss.NewStyle().Padding(1, 2)

// Result is returned by Run when the user chose to launch an agent.
type Result struct {
	Launch bool
	Issue  jira.Issue
	Agent  string
}

// model is the root Bubble Tea model.
type model struct {
	screen screen
	wiz    wizard
	dash   dashboard
	cfg    *config.Config

	// mascot ticks once at the root so a single animation drives every screen.
	mascot     mascot
	showSplash bool

	version string // running build, threaded to the dashboard's update check

	width, height int
	result        *Result
}

// Run starts the TUI. cfg may be nil/incomplete (the wizard opens). When
// startInWizard is true the wizard opens even if cfg is valid (settings mode).
// version is the running build, used by the dashboard to flag a newer release.
// It returns the possibly-updated config and an optional launch Result.
func Run(cfg *config.Config, startInWizard bool, version string) (*config.Config, *Result, error) {
	m := newModel(cfg, startInWizard, version)
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return cfg, nil, err
	}
	fm := final.(model)
	return fm.cfg, fm.result, nil
}

func newModel(cfg *config.Config, startInWizard bool, version string) model {
	valid := cfg != nil && cfg.Validate() == nil
	base := cfg
	if base == nil {
		base = config.Default()
	}
	if startInWizard || !valid {
		return model{screen: screenWizard, wiz: newWizard(base, valid), cfg: base, showSplash: true, version: version}
	}
	return model{screen: screenDashboard, dash: newDashboard(cfg, version), cfg: cfg, showSplash: true, version: version}
}

func (m model) Init() tea.Cmd {
	var screenCmd tea.Cmd
	if m.screen == screenWizard {
		screenCmd = m.wiz.Init()
	} else {
		screenCmd = m.dash.Init()
	}
	// The screen loads underneath the splash; the mascot tick animates everything.
	return tea.Batch(screenCmd, mascotTickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = ws.Width, ws.Height
		// fall through so the active screen also receives the size
	}

	// The mascot animation is owned here so only one tick chain runs.
	if _, ok := msg.(mascotTickMsg); ok {
		m.mascot = m.mascot.tick()
		return m, mascotTickCmd()
	}

	// The startup splash stays until the first key press; other messages still
	// flow through so the screen loads underneath it.
	if m.showSplash {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			m.showSplash = false
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case launchMsg:
		m.result = &Result{Launch: true, Issue: msg.issue, Agent: msg.agent}
		return m, tea.Quit

	case openSettingsMsg:
		m.screen = screenWizard
		m.wiz = newWizard(m.cfg, true)
		m.wiz, _ = m.wiz.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.wiz.Init()

	case wizardDoneMsg:
		m.cfg = msg.cfg
		m.dash = newDashboard(msg.cfg, m.version)
		m.dash, _ = m.dash.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.screen = screenDashboard
		return m, m.dash.Init()

	case wizardCancelMsg:
		if m.cfg != nil && m.cfg.Validate() == nil {
			m.dash = newDashboard(m.cfg, m.version)
			m.dash, _ = m.dash.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.screen = screenDashboard
			return m, m.dash.Init()
		}
		return m, tea.Quit
	}

	switch m.screen {
	case screenWizard:
		var cmd tea.Cmd
		m.wiz, cmd = m.wiz.Update(msg)
		return m, cmd
	case screenDashboard:
		var cmd tea.Cmd
		m.dash, cmd = m.dash.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() tea.View {
	var content string
	switch {
	case m.showSplash:
		content = splashView(m.mascot, m.width, m.height)
	case m.screen == screenWizard:
		content = m.wiz.View(m.mascot) // pass the root's animation frame
	case m.screen == screenDashboard:
		content = m.dash.View(m.mascot)
	}
	v := tea.NewView(appStyle.Render(content))
	v.AltScreen = true
	v.WindowTitle = "SprintMate"
	return v
}
