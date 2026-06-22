package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/queue"
)

type reviewTab int

const (
	tabPlan reviewTab = iota
	tabDiff
)

// review shows one job's captured plan and worktree diff, and triggers ship.
type review struct {
	cfg      *config.Config
	ctx      context.Context // monitor's program-scoped ctx; cancels ship on quit
	jobID    int
	key      string
	title    string
	dir      string
	branch   string
	state    queue.State
	planPath string

	tab      reviewTab // active pane: which one shows (narrow) / has scroll focus (wide)
	vpPlan   viewport.Model
	vpDiff   viewport.Model
	planText string
	diffText string
	shipping bool
	keys     reviewKeymap
	notice   string
	status   string
	width    int
	height   int
}

func newReview(cfg *config.Config, ctx context.Context, j queue.Job) review {
	return review{
		cfg:      cfg,
		ctx:      ctx,
		jobID:    j.ID,
		key:      j.Key,
		title:    j.Title,
		dir:      j.Dir,
		branch:   j.Branch,
		state:    j.State,
		planPath: j.PlanPath,
		tab:      tabPlan,
		vpPlan:   viewport.New(),
		vpDiff:   viewport.New(),
		planText: "(loading plan…)",
		diffText: "(loading diff…)",
		keys:     newReviewKeymap(cfg.Keys),
	}
}

func (r review) Update(msg tea.Msg) (review, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = msg.Width, msg.Height
		r.resizeViewports()
		r.syncContent()
		return r, nil

	case planLoadedMsg:
		if msg.id != r.jobID {
			return r, nil
		}
		if msg.err != nil {
			r.planText = "error loading plan: " + msg.err.Error()
		} else {
			r.planText = msg.text
		}
		r.syncContent()
		return r, nil

	case diffLoadedMsg:
		if msg.id != r.jobID {
			return r, nil
		}
		if msg.err != nil {
			r.diffText = "error loading diff: " + msg.err.Error()
		} else {
			r.diffText = colorizeDiff(msg.diff)
		}
		r.syncContent()
		return r, nil

	case shipDoneMsg:
		if msg.id != r.jobID {
			return r, nil
		}
		r.shipping = false
		if msg.err != nil {
			r.notice = "ship failed: " + msg.err.Error()
		} else if msg.prURL != "" {
			r.status = "✓ shipped: " + msg.prURL
		} else if len(msg.steps) > 0 {
			r.status = "✓ shipped: " + strings.Join(msg.steps, ", ")
		} else {
			r.status = "✓ shipped"
		}
		return r, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, r.keys.tab):
			// Narrow: switches which pane is shown. Wide: moves scroll focus
			// between the two side-by-side panes. Both viewports stay populated.
			if r.tab == tabPlan {
				r.tab = tabDiff
			} else {
				r.tab = tabPlan
			}
			return r, nil
		case key.Matches(msg, r.keys.approve):
			if r.state == queue.PlanReady {
				id := r.jobID
				r.status = "approved — executing…"
				return r, func() tea.Msg { return approveJobMsg{id: id} }
			}
			return r, nil
		case key.Matches(msg, r.keys.ship):
			return r.startShip()
		case key.Matches(msg, r.keys.back):
			return r, func() tea.Msg { return backToMonitorMsg{} }
		}
	}

	// Route scrolling to the focused viewport.
	var cmd tea.Cmd
	if r.tab == tabDiff {
		r.vpDiff, cmd = r.vpDiff.Update(msg)
	} else {
		r.vpPlan, cmd = r.vpPlan.Update(msg)
	}
	return r, cmd
}

// boxInsetX/Y are boxStyle's frame cost: RoundedBorder (2) + Padding(1,2) =>
// 4 horizontal / 2 vertical padding, so 6 columns / 4 rows total.
const boxInsetX = 6
const boxInsetY = 4

// resizeViewports sizes both viewports to the content area of their box(es) for
// the current terminal: full width in single-column mode, half width when the
// plan and diff sit side by side.
func (r *review) resizeViewports() {
	lay := computeLayout(r.width, r.height, true)
	outerW := lay.bodyWidth
	if lay.twoColumn {
		outerW = (lay.bodyWidth - paneGap) / 2
	}
	vpW := max(20, outerW-boxInsetX)
	vpH := max(5, lay.bodyHeight-boxInsetY)
	r.vpPlan.SetWidth(vpW)
	r.vpPlan.SetHeight(vpH)
	r.vpDiff.SetWidth(vpW)
	r.vpDiff.SetHeight(vpH)
}

func (r review) startShip() (review, tea.Cmd) {
	switch {
	case r.state != queue.Done:
		r.notice = "ship is available once the run is done"
		return r, nil
	case !r.cfg.Ship.Enabled():
		r.notice = "shipping is disabled — enable it under ship: in your config"
		return r, nil
	case r.shipping:
		return r, nil
	}
	r.shipping = true
	r.notice = ""
	r.status = "shipping…"
	job := queue.Job{ID: r.jobID, Dir: r.dir, Branch: r.branch, Key: r.key, Title: r.title}
	return r, shipCmd(r.ctx, r.cfg, job)
}

// syncContent keeps both viewports populated; the View decides which to show.
func (r *review) syncContent() {
	r.vpPlan.SetContent(r.planText)
	r.vpDiff.SetContent(r.diffText)
}

func (r review) View(mas mascot) string {
	mood := moodWorking
	switch {
	case r.notice != "":
		mood = moodError
	case r.state == queue.Done:
		mood = moodHappy
	}
	lay := computeLayout(r.width, r.height, true)

	return renderFrame(chrome{
		header: mas.header("SprintMate · Review · "+r.key, mood),
		strip:  r.tabsView(lay),
		body:   r.bodyView(lay),
		footer: dimStyle.Render("  state: ") + reviewStateStyle(r.state).Render(r.state.String()),
		hints:  r.hintsView(),
	})
}

// tabsView labels the panes. Single column: a selector showing the active tab.
// Side by side: both labelled, the scroll-focused one highlighted.
func (r review) tabsView(lay frameLayout) string {
	plan, diff := dimStyle.Render("Plan"), dimStyle.Render("Diff")
	if lay.twoColumn {
		if r.tab == tabPlan {
			plan = panelTitleStyle.Render("Plan")
		} else {
			diff = panelTitleStyle.Render("Diff")
		}
		halfW := (lay.bodyWidth - paneGap) / 2
		left := lipgloss.NewStyle().Width(halfW).Render("  " + plan)
		return left + strings.Repeat(" ", paneGap) + "  " + diff
	}
	if r.tab == tabPlan {
		plan = panelTitleStyle.Render("[ Plan ]")
	} else {
		diff = panelTitleStyle.Render("[ Diff ]")
	}
	return "  " + plan + "    " + diff
}

// bodyView is the single box (narrow) or the two side-by-side panes (wide).
func (r review) bodyView(lay frameLayout) string {
	if lay.twoColumn {
		halfW := (lay.bodyWidth - paneGap) / 2
		left := r.paneBox(r.vpPlan.View(), halfW, lay.bodyHeight, r.tab == tabPlan)
		right := r.paneBox(r.vpDiff.View(), halfW, lay.bodyHeight, r.tab == tabDiff)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", paneGap), right)
	}
	active := r.vpPlan
	if r.tab == tabDiff {
		active = r.vpDiff
	}
	return boxStyle.Width(lay.bodyWidth).Height(lay.bodyHeight).Render(active.View())
}

// paneBox wraps a viewport in a sized box, highlighting the focused pane's
// border with the signature accent.
func (r review) paneBox(content string, outerW, outerH int, focused bool) string {
	st := boxStyle.Width(outerW).Height(outerH)
	if focused {
		st = st.BorderForeground(colorActive)
	}
	return st.Render(content)
}

func (r review) hintsView() string {
	help := helpStyle.Render("tab switch · ↑/↓ scroll · a approve · S ship · esc back · q quit")
	if r.status != "" {
		help = okStyle.Render(r.status) + "\n" + help
	}
	if r.notice != "" {
		help = errStyle.Render("⚠ "+r.notice) + "\n" + help
	}
	return help
}

// reviewStateStyle colors the state label: accent while a phase runs, semantic
// green/red at terminal states.
func reviewStateStyle(s queue.State) lipgloss.Style {
	switch s {
	case queue.Planning, queue.Executing:
		return activeStyle
	case queue.Done:
		return okStyle
	case queue.Failed:
		return errStyle
	default:
		return labelStyle
	}
}

// colorizeDiff applies the palette to a unified diff for the viewport.
func colorizeDiff(d string) string {
	if strings.TrimSpace(d) == "" {
		return "(no changes)"
	}
	var b strings.Builder
	for _, line := range strings.Split(d, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			b.WriteString(okStyle.Render(line))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			b.WriteString(errStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			b.WriteString(labelStyle.Render(line))
		default:
			b.WriteString(dimStyle.Render(line))
		}
		b.WriteString("\n")
	}
	return b.String()
}
