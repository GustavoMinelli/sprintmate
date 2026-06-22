package tui

import (
	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
	"github.com/GustavoMinelli/sprintmate/internal/queue"
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

// --- autonomous queue / review messages ---

// enqueueMsg asks the monitor to prepare and queue a headless run for an issue.
type enqueueMsg struct {
	issue jira.Issue
	agent string
}

// openMonitorMsg switches the root to the queue monitor.
type openMonitorMsg struct{}

// openReviewMsg opens the review screen for a job.
type openReviewMsg struct{ jobID int }

// backToMonitorMsg / backToDashboardMsg are navigation pops.
type backToMonitorMsg struct{}
type backToDashboardMsg struct{}

// jobPreparedMsg carries a worktree-prepared job (or an error) back to the
// monitor so it can be added to the engine. key is always set (even on error) so
// the monitor can release the in-flight reservation for that issue.
type jobPreparedMsg struct {
	key string
	job *queue.Job
	err error
}

// phaseDoneMsg reports that one RunPhase tea.Cmd finished.
type phaseDoneMsg struct {
	id    int
	phase queue.Phase
	err   error
}

// approveJobMsg routes a review-screen approval to the monitor, where the
// scheduler lives.
type approveJobMsg struct{ id int }

// planLoadedMsg / diffLoadedMsg carry async review content (tagged with the job
// id so stale results for other jobs are dropped).
type planLoadedMsg struct {
	id   int
	text string
	err  error
}
type diffLoadedMsg struct {
	id   int
	diff string
	err  error
}

// shipDoneMsg reports the result of the ship pipeline.
type shipDoneMsg struct {
	id    int
	prURL string
	steps []string
	err   error
}

// quitConfirmedMsg is sent when the user confirms quitting with active jobs.
type quitConfirmedMsg struct{}

// requestQuitMsg is emitted by the dashboard's quit key so the root can route it
// through the monitor's confirm/cancel path when autonomous jobs are running
// (otherwise quitting would orphan the agent subprocesses).
type requestQuitMsg struct{}
