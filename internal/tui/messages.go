package tui

import (
	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
)

// --- data-loaded messages (results of async tea.Cmds) ---

type errMsg struct{ err error }

type connTestedMsg struct {
	err error
}

type boardsLoadedMsg struct {
	boards []jira.Board
	err    error
}

type columnsLoadedMsg struct {
	cols []jira.Column
	err  error
}

type issuesLoadedMsg struct {
	res jira.Result
	err error
}

// --- app-level messages (bubble up from sub-models to the root) ---

// launchMsg asks the root to quit and hand back the chosen issue+agent so the
// entrypoint can run the launch flow. Only used for the in-place strategy, which
// takes over this terminal; every other strategy launches from the dashboard and
// keeps it open (see launchedMsg).
type launchMsg struct {
	issue jira.Issue
	agent string
}

// launchedMsg reports the outcome of a windowed/tmux launch that ran in the
// background while the dashboard stayed open, so the user can launch another.
type launchedMsg struct {
	key      string
	agent    string
	strategy string // the concrete strategy used (tmux/window), for the message
	err      error
}

// updateAvailableMsg is delivered when a newer release exists on GitHub. The
// check runs in the background on dashboard start; failures and "already current"
// are swallowed (the command returns nil), so a network problem never surfaces.
type updateAvailableMsg struct{ latest string }

// openSettingsMsg switches the root from the dashboard to the wizard.
type openSettingsMsg struct{}

// wizardDoneMsg carries the saved config back to the root.
type wizardDoneMsg struct{ cfg *config.Config }

// wizardCancelMsg is sent when the user aborts the wizard.
type wizardCancelMsg struct{}
