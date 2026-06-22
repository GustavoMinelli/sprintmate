package tui

import (
	"context"
	"fmt"
	"sync"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/queue"
)

// jobItem adapts a queue.Job snapshot to the list.DefaultItem interface.
type jobItem struct{ job queue.Job }

func (j jobItem) Title() string {
	return stateBadge(j.job.State) + "  " + j.job.Key + "  " + j.job.Title
}

func (j jobItem) Description() string {
	d := j.job.Branch + " · " + j.job.Agent
	if j.job.State == queue.Failed && j.job.Err != nil {
		d += " · " + j.job.Err.Error()
	}
	return d
}

func (j jobItem) FilterValue() string { return j.job.Key + " " + j.job.Title }

// monitor is the autonomous-queue screen. It owns the engine, the scheduler and
// the nested review sub-model. The engine is mutated only here (on the Bubble
// Tea update goroutine); phase goroutines run RunPhase on value copies.
type monitor struct {
	cfg  *config.Config
	eng  *queue.Engine
	keys monitorKeymap

	list     list.Model
	review   review
	inReview bool

	// ctx is program-scoped and cancels every running phase on quit (no daemon).
	ctx    context.Context
	cancel context.CancelFunc
	// wg tracks in-flight phase goroutines so cancelCmd can wait for their
	// children to actually be reaped before the program exits.
	wg *sync.WaitGroup

	running        int // RunPhase cmds in flight (display + quit-confirm only)
	confirmingQuit bool
	// pending reserves issue keys whose worktree is being prepared (async), so a
	// second enqueue of the same key can't slip through before the first job is
	// added to the engine (HasOpen would not see it yet).
	pending        map[string]bool
	notice, status string
	width, height  int
}

func newMonitor(cfg *config.Config) monitor {
	l := list.New(nil, newIssueDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false) // the frame renders a single, authoritative hints line
	l.DisableQuitKeybindings()
	l.Styles.Filter.Cursor.Color = colorActive
	if len(cfg.Keys.Up) > 0 {
		l.KeyMap.CursorUp = key.NewBinding(key.WithKeys(cfg.Keys.Up...), key.WithHelp(cfg.Keys.Up[0], "up"))
	}
	if len(cfg.Keys.Down) > 0 {
		l.KeyMap.CursorDown = key.NewBinding(key.WithKeys(cfg.Keys.Down...), key.WithHelp(cfg.Keys.Down[0], "down"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	return monitor{
		cfg:     cfg,
		eng:     queue.New(cfg.Queue.Concurrency, cfg.Queue.AutoApprove),
		keys:    newMonitorKeymap(cfg.Keys),
		list:    l,
		ctx:     ctx,
		cancel:  cancel,
		wg:      &sync.WaitGroup{},
		pending: map[string]bool{},
	}
}

func (m monitor) Init() tea.Cmd { return nil }

// cancelCmd cancels all running phases and waits for their agent subprocesses to
// be reaped; the root runs it before tea.Quit so no agent is orphaned.
func (m monitor) cancelCmd() tea.Cmd {
	cancel, wg := m.cancel, m.wg
	return func() tea.Msg {
		if cancel != nil {
			cancel()
		}
		if wg != nil {
			wg.Wait()
		}
		return nil
	}
}

func (m monitor) Update(msg tea.Msg) (monitor, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		lay := computeLayout(msg.Width, msg.Height, false)
		m.list.SetSize(max(10, lay.bodyWidth), max(5, lay.bodyHeight))
		if m.inReview {
			var cmd tea.Cmd
			m.review, cmd = m.review.Update(msg) // keep the open viewport sized
			return m, cmd
		}
		return m, nil

	case openMonitorMsg:
		return m, nil // root already flipped the screen

	case enqueueMsg:
		// Reject if a job for this key is already open OR currently being prepared
		// (the worktree path is keyed by issue, so two would collide).
		if m.eng.HasOpen(msg.issue.Key) || m.pending[msg.issue.Key] {
			m.notice = fmt.Sprintf("%s already has a queued or running job", msg.issue.Key)
			return m, nil
		}
		m.notice = ""
		m.pending[msg.issue.Key] = true // reserve synchronously, on this goroutine
		return m, prepareJobCmd(m.ctx, m.cfg, msg.issue, msg.agent)

	case jobPreparedMsg:
		delete(m.pending, msg.key) // release the reservation
		if msg.err != nil {
			m.notice = msg.err.Error()
			return m, nil
		}
		j := m.eng.Add(msg.job)
		m.notice = ""
		m.status = fmt.Sprintf("✓ %s queued", j.Key)
		return m, tea.Batch(m.refreshList(), m.dispatch())

	case phaseDoneMsg:
		if m.running > 0 {
			m.running--
		}
		m.eng.Finish(msg.id, msg.err)
		j := m.eng.Get(msg.id)
		cmds := []tea.Cmd{m.refreshList()}
		switch {
		case msg.err != nil && j != nil:
			m.notice = fmt.Sprintf("%s %s: %v", j.Key, j.State, msg.err)
			cmds = append(cmds, notifyCmd(m.cfg, "failed", j.Key))
		case msg.phase == queue.ExecPhase && j != nil:
			m.status = fmt.Sprintf("✓ %s done", j.Key)
			cmds = append(cmds, notifyCmd(m.cfg, "done", j.Key))
		}
		cmds = append(cmds, m.dispatch())
		// Refresh an open review of this job.
		if m.inReview && m.review.jobID == msg.id && j != nil {
			m.review.state = j.State
			if msg.phase == queue.PlanPhase && msg.err == nil {
				cmds = append(cmds, loadPlanCmd(j.ID, m.review.planPath))
			}
			if msg.phase == queue.ExecPhase {
				cmds = append(cmds, loadDiffCmd(j.ID, m.review.dir, true))
			}
		}
		return m, tea.Batch(cmds...)

	case approveJobMsg:
		m.eng.Approve(msg.id)
		return m, tea.Batch(m.refreshList(), m.dispatch())

	case openReviewMsg:
		j := m.eng.Get(msg.jobID)
		if j == nil {
			return m, nil
		}
		m.review = newReview(m.cfg, m.ctx, *j)
		m.review, _ = m.review.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.inReview = true
		return m, tea.Batch(
			loadPlanCmd(j.ID, m.review.planPath),
			loadDiffCmd(j.ID, m.review.dir, executed(j.State)),
		)

	case planLoadedMsg, diffLoadedMsg, shipDoneMsg:
		var cmd tea.Cmd
		m.review, cmd = m.review.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Default: route to the active widget.
	if m.inReview {
		var cmd tea.Cmd
		m.review, cmd = m.review.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m monitor) handleKey(msg tea.KeyPressMsg) (monitor, tea.Cmd) {
	// Quit (with confirmation when jobs are running) is handled by the monitor on
	// both the monitor and review screens, since it owns the engine.
	if m.confirmingQuit {
		if key.Matches(msg, m.keys.quit) {
			return m, func() tea.Msg { return quitConfirmedMsg{} }
		}
		m.confirmingQuit = false
		m.notice = ""
		return m, nil
	}
	if key.Matches(msg, m.keys.quit) {
		if m.eng.Active() > 0 {
			m.confirmingQuit = true
			m.notice = "jobs are running — press q again to stop them and quit"
			return m, nil
		}
		return m, func() tea.Msg { return quitConfirmedMsg{} }
	}

	// In review, every other key goes to the review sub-model.
	if m.inReview {
		var cmd tea.Cmd
		m.review, cmd = m.review.Update(msg)
		return m, cmd
	}

	// While filtering, let the list consume keystrokes.
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, m.keys.open):
		if it, ok := m.list.SelectedItem().(jobItem); ok {
			id := it.job.ID
			return m, func() tea.Msg { return openReviewMsg{jobID: id} }
		}
	case key.Matches(msg, m.keys.approve):
		if it, ok := m.list.SelectedItem().(jobItem); ok && it.job.State == queue.PlanReady {
			m.eng.Approve(it.job.ID)
			m.status = fmt.Sprintf("approved %s", it.job.Key)
			return m, tea.Batch(m.refreshList(), m.dispatch())
		}
	case key.Matches(msg, m.keys.back):
		return m, func() tea.Msg { return backToDashboardMsg{} }
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// dispatch drains the engine, starting every runnable phase. Each Dequeue
// reserves a slot, so concurrency and the approval gate are enforced by the
// engine; this just turns reservations into goroutines.
func (m *monitor) dispatch() tea.Cmd {
	var cmds []tea.Cmd
	for {
		job, phase := m.eng.Dequeue()
		if job == nil {
			break
		}
		m.running++
		m.wg.Add(1)
		wg := m.wg
		inner := runPhaseCmd(m.ctx, m.cfg, *job, phase)
		cmds = append(cmds, func() tea.Msg {
			defer wg.Done()
			return inner()
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *monitor) refreshList() tea.Cmd {
	jobs := m.eng.Jobs()
	items := make([]list.Item, len(jobs))
	for i, j := range jobs {
		items[i] = jobItem{job: *j}
	}
	return m.list.SetItems(items)
}

func (m monitor) View(mas mascot) string {
	mood := moodIdle
	switch {
	case m.running > 0:
		mood = moodWorking
	case m.notice != "":
		mood = moodError
	case m.status != "":
		mood = moodHappy
	}

	body := m.list.View()
	if len(m.eng.Jobs()) == 0 {
		body = dimStyle.Render("  No autonomous runs yet — press e on an issue in the dashboard to queue one.")
	}

	return renderFrame(chrome{
		header: mas.header("SprintMate · Queue", mood),
		body:   body,
		footer: m.footerView(),
		hints:  m.hintsView(),
	})
}

// footerView is the queue's status bar: running / pending / awaiting counts.
func (m monitor) footerView() string {
	sep := "    "
	return footerLabelStyle.Render("Running: ") + footerValueStyle.Render(fmt.Sprintf("%d/%d", m.eng.Active(), m.cfg.Queue.Concurrency)) +
		sep + footerLabelStyle.Render("Pending: ") + footerValueStyle.Render(fmt.Sprint(m.eng.Pending())) +
		sep + footerLabelStyle.Render("Awaiting approval: ") + footerValueStyle.Render(fmt.Sprint(m.eng.AwaitingApproval()))
}

// hintsView is the key-hints line plus any transient status/notice above it.
func (m monitor) hintsView() string {
	help := helpStyle.Render("↑/↓ select · enter review · a approve · esc dashboard · q quit")
	if m.status != "" {
		help = okStyle.Render(m.status) + "\n" + help
	}
	if m.notice != "" {
		help = errStyle.Render("⚠ "+m.notice) + "\n" + help
	}
	return help
}

// stateBadge renders a styled chip for a job state.
func stateBadge(s queue.State) string {
	switch s {
	case queue.Queued:
		return dimStyle.Render("queued")
	case queue.Planning:
		return cursorStyle.Render("planning…")
	case queue.PlanReady:
		return labelStyle.Render("plan ready")
	case queue.Executing:
		return cursorStyle.Render("executing…")
	case queue.Done:
		return okStyle.Render("✓ done")
	case queue.Failed:
		return errStyle.Render("✗ failed")
	default:
		return s.String()
	}
}

// executed reports whether a job has reached a phase where a diff is meaningful.
func executed(s queue.State) bool {
	return s == queue.Executing || s == queue.Done || s == queue.Failed
}
