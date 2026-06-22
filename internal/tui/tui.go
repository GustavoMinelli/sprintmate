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
	screenMonitor
	screenReview
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
	mon    monitor // autonomous queue; created lazily, outlives screen switches
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

	// The mascot animation is owned here so only one tick chain runs. The tick is
	// also the dashboard's queue-strip clock: it fires every ~380ms on every
	// screen, so the strip stays live while jobs advance in the background.
	if _, ok := msg.(mascotTickMsg); ok {
		m.mascot = m.mascot.tick()
		m.dash.qstats = m.queueSnapshot()
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

	case enqueueMsg:
		// Queue the job but stay on the current screen, so several issues can be
		// enqueued from the dashboard before opening the monitor.
		m.ensureMonitor()
		var cmd tea.Cmd
		m.mon, cmd = m.mon.Update(msg)
		return m, cmd

	case openMonitorMsg:
		m.ensureMonitor()
		m.screen = screenMonitor
		var cmd tea.Cmd
		m.mon, cmd = m.mon.Update(msg)
		return m, cmd

	case jobPreparedMsg, phaseDoneMsg, approveJobMsg, planLoadedMsg, diffLoadedMsg, shipDoneMsg:
		// Engine/supervision/review-async messages always reach the monitor, so
		// jobs keep advancing while the user is on the dashboard or in review.
		if m.mon.eng == nil {
			return m, nil
		}
		var cmd tea.Cmd
		m.mon, cmd = m.mon.Update(msg)
		return m, cmd

	case openReviewMsg:
		m.screen = screenReview
		var cmd tea.Cmd
		m.mon, cmd = m.mon.Update(msg)
		return m, cmd

	case backToMonitorMsg:
		m.screen = screenMonitor
		m.mon.inReview = false
		return m, nil

	case backToDashboardMsg:
		m.screen = screenDashboard
		return m, nil

	case requestQuitMsg:
		// Quit requested from the dashboard. If autonomous jobs are running, send
		// the user to the monitor to confirm (so their agents aren't orphaned);
		// otherwise cancel any monitor context and quit.
		if m.mon.eng != nil && m.mon.eng.Active() > 0 {
			m.screen = screenMonitor
			m.mon.confirmingQuit = true
			m.mon.notice = "jobs are running — press q again to stop them and quit"
			return m, nil
		}
		if m.mon.eng != nil {
			return m, tea.Sequence(m.mon.cancelCmd(), tea.Quit)
		}
		return m, tea.Quit

	case quitConfirmedMsg:
		return m, tea.Sequence(m.mon.cancelCmd(), tea.Quit)

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
		m.dash.qstats = m.queueSnapshot() // keep the strip fresh on navigation frames too
		var cmd tea.Cmd
		m.dash, cmd = m.dash.Update(msg)
		return m, cmd
	case screenMonitor, screenReview:
		var cmd tea.Cmd
		m.mon, cmd = m.mon.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ensureMonitor lazily creates the queue monitor on first use and sizes it.
func (m *model) ensureMonitor() {
	if m.mon.eng == nil {
		m.mon = newMonitor(m.cfg)
		m.mon, _ = m.mon.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	}
}

// queueSnapshot reads the live queue counts off the engine. The root is the only
// place that knows about both the dashboard and the monitor, and it runs on the
// Bubble Tea update goroutine — the same one that mutates the (lock-free) engine
// — so these reads are safe. It returns an inactive snapshot until the monitor
// (and its engine) exist.
func (m model) queueSnapshot() queueStats {
	if m.mon.eng == nil {
		return queueStats{active: false}
	}
	return queueStats{
		running:  m.mon.eng.Active(),
		slots:    m.cfg.Queue.Concurrency,
		pending:  m.mon.eng.Pending(),
		awaiting: m.mon.eng.AwaitingApproval(),
		active:   true,
	}
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
	case m.screen == screenMonitor:
		content = m.mon.View(m.mascot)
	case m.screen == screenReview:
		content = m.mon.review.View(m.mascot)
	}
	v := tea.NewView(appStyle.Render(content))
	v.AltScreen = true
	v.WindowTitle = "SprintMate"
	return v
}
