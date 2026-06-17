package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GustavoMinelli/sprintmate/internal/agents"
	"github.com/GustavoMinelli/sprintmate/internal/app"
	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
	"github.com/GustavoMinelli/sprintmate/internal/terminal"
)

// issueItem adapts a jira.Issue to the list.DefaultItem interface.
type issueItem struct{ issue jira.Issue }

func (i issueItem) Title() string { return i.issue.Key + "  " + i.issue.Title }

func (i issueItem) Description() string {
	var parts []string
	if i.issue.Status != "" {
		parts = append(parts, i.issue.Status)
	}
	if i.issue.Priority != "" {
		parts = append(parts, i.issue.Priority)
	}
	if i.issue.StoryPoints > 0 {
		parts = append(parts, fmt.Sprintf("%gpts", i.issue.StoryPoints))
	}
	return strings.Join(parts, " · ")
}

func (i issueItem) FilterValue() string { return i.issue.Key + " " + i.issue.Title }

// dashboard is the main screen: the issue list plus footer.
type dashboard struct {
	cfg    *config.Config
	client *jira.Client
	source jira.Source

	list     list.Model
	keys     keymap
	agentSet []string
	agentIdx int
	mascot   mascot // animation frame is supplied by the root model

	sprintLabel string
	boardName   string
	loading     bool
	launching   bool   // a windowed/tmux launch is in flight
	err         string
	notice      string // transient error (e.g. failed to open browser)
	status      string // transient positive message (e.g. agent launched)

	width, height int
}

func newDashboard(cfg *config.Config) dashboard {
	l := list.New(nil, newIssueDelegate(), 0, 0)
	l.SetShowTitle(false) // we render our own header
	l.DisableQuitKeybindings()

	// The list ships with a purple filter cursor; recolor it to the accent.
	l.Styles.Filter.Cursor.Color = colorAccent

	// Apply the user's configured keys to the list's own navigation/filter
	// bindings so they actually take effect.
	if len(cfg.Keys.Up) > 0 {
		l.KeyMap.CursorUp = key.NewBinding(key.WithKeys(cfg.Keys.Up...), key.WithHelp(cfg.Keys.Up[0], "up"))
	}
	if len(cfg.Keys.Down) > 0 {
		l.KeyMap.CursorDown = key.NewBinding(key.WithKeys(cfg.Keys.Down...), key.WithHelp(cfg.Keys.Down[0], "down"))
	}
	if len(cfg.Keys.Search) > 0 {
		l.KeyMap.Filter = key.NewBinding(key.WithKeys(cfg.Keys.Search...), key.WithHelp(cfg.Keys.Search[0], "filter"))
	}

	names := agents.Names()
	idx := indexOf(names, cfg.Agent.Default)
	if idx < 0 {
		idx = 0
	}

	return dashboard{
		cfg:      cfg,
		client:   jira.New(cfg.Jira.Host, cfg.Jira.Email, cfg.Jira.Token),
		source:   sourceFromConfig(cfg),
		list:     l,
		keys:     newKeymap(cfg.Keys),
		agentSet: names,
		agentIdx: idx,
		loading:  true,
	}
}

func (d dashboard) Init() tea.Cmd { return d.loadIssues() }

func (d dashboard) Update(msg tea.Msg) (dashboard, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = msg.Width, msg.Height
		// Leave room for the taller mascot header (sprite + spacing + footer/help).
		d.list.SetSize(max(10, msg.Width-4), max(5, msg.Height-12))
		return d, nil

	case errMsg:
		d.notice = msg.err.Error()
		return d, nil

	case issuesLoadedMsg:
		d.loading = false
		if msg.err != nil {
			d.err = msg.err.Error()
			return d, nil
		}
		d.err = ""
		d.sprintLabel = msg.res.SprintLabel
		d.boardName = msg.res.BoardName
		items := make([]list.Item, len(msg.res.Issues))
		for i, is := range msg.res.Issues {
			items[i] = issueItem{issue: is}
		}
		return d, d.list.SetItems(items)

	case launchedMsg:
		d.launching = false
		if msg.err != nil {
			d.status = ""
			d.notice = msg.err.Error()
			return d, nil
		}
		d.notice = ""
		d.status = fmt.Sprintf("✓ %s lançado em nova janela — escolha outra demanda ou q para sair", msg.key)
		return d, nil

	case tea.KeyPressMsg:
		// While filtering, let the list consume all keystrokes.
		if d.list.FilterState() == list.Filtering {
			break
		}
		switch {
		case key.Matches(msg, d.keys.launch):
			if it, ok := d.list.SelectedItem().(issueItem); ok {
				issue, agent := it.issue, d.currentAgent()
				// Validate before launching so a bad config surfaces here in the
				// dashboard instead of crashing the process.
				plan, err := app.BuildPlan(d.cfg, issue, agent)
				if err != nil {
					d.notice = err.Error()
					return d, nil
				}
				d.notice = ""
				// The in-place strategy takes over this terminal, so it has to
				// hand off through the root (quit, then launch). Every other
				// strategy opens the agent in its own window/tmux pane, so we
				// launch from here and keep the dashboard open for the next one.
				if terminal.Resolve(d.cfg.Launch.Strategy) == terminal.Inplace {
					return d, func() tea.Msg { return launchMsg{issue: issue, agent: agent} }
				}
				d.launching = true
				d.status = fmt.Sprintf("Lançando %s com %s…", issue.Key, agent)
				return d, d.launchCmd(plan)
			}
		case key.Matches(msg, d.keys.switchAgent):
			if len(d.agentSet) > 0 {
				d.agentIdx = (d.agentIdx + 1) % len(d.agentSet)
			}
			return d, nil
		case key.Matches(msg, d.keys.refresh):
			d.loading = true
			d.err = ""
			d.notice = ""
			d.status = ""
			return d, d.loadIssues()
		case key.Matches(msg, d.keys.openBrowser):
			if it, ok := d.list.SelectedItem().(issueItem); ok {
				d.notice = ""
				return d, openURLCmd(it.issue.URL)
			}
		case key.Matches(msg, d.keys.settings):
			return d, func() tea.Msg { return openSettingsMsg{} }
		case key.Matches(msg, d.keys.quit):
			return d, tea.Quit
		}
	}

	var cmd tea.Cmd
	d.list, cmd = d.list.Update(msg)
	return d, cmd
}

func (d dashboard) View() string {
	mood := moodIdle
	switch {
	case d.loading || d.launching:
		mood = moodWorking
	case d.err != "" || d.notice != "":
		mood = moodError
	case d.status != "":
		mood = moodHappy
	}
	header := d.mascot.header("SprintMate", mood)

	var body string
	switch {
	case d.loading:
		body = dimStyle.Render("  Carregando issues do Jira...")
	case d.err != "":
		body = errStyle.Render("  Erro: "+d.err) + "\n\n" +
			helpStyle.Render("  r: tentar de novo   ·   s: configurações   ·   q: sair")
	default:
		body = d.list.View()
	}

	board := d.boardName
	if board == "" {
		board = "-"
	}
	footer := footerStyle.Render(fmt.Sprintf("Board: %s    Sprint: %s    Agent: %s",
		board, orDash(d.sprintLabel), d.currentAgent()))
	help := helpStyle.Render("↑/↓ navegar · enter abrir · tab agente · / buscar · r atualizar · o navegador · s config · q sair")
	if d.status != "" {
		help = okStyle.Render(d.status) + "\n" + help
	}
	if d.notice != "" {
		help = errStyle.Render("⚠ "+d.notice) + "\n" + help
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer, help)
}

// newIssueDelegate is the list's row renderer with the selector recolored to
// our palette (the stock delegate highlights the selected row in purple).
func newIssueDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		BorderForeground(colorPrimary).
		Foreground(colorAccent)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		BorderForeground(colorPrimary).
		Foreground(colorPrimary)
	d.Styles.FilterMatch = d.Styles.FilterMatch.Foreground(colorAccent)
	return d
}

func (d dashboard) currentAgent() string {
	if len(d.agentSet) == 0 {
		return d.cfg.Agent.Default
	}
	return d.agentSet[d.agentIdx]
}

// launchCmd prepares and launches the agent in the background (a new window or
// tmux pane), then reports the outcome so the dashboard can stay open.
func (d dashboard) launchCmd(plan app.Plan) tea.Cmd {
	cfg := d.cfg
	return func() tea.Msg {
		err := app.PrepareAndLaunch(context.Background(), cfg, plan)
		return launchedMsg{key: plan.Issue.Key, agent: plan.AgentName, err: err}
	}
}

func (d dashboard) loadIssues() tea.Cmd {
	client, src := d.client, d.source
	return func() tea.Msg {
		res, err := client.FetchIssues(context.Background(), src)
		return issuesLoadedMsg{res: res, err: err}
	}
}

func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if err := terminal.OpenURL(url); err != nil {
			return errMsg{err: fmt.Errorf("não foi possível abrir o navegador: %w", err)}
		}
		return nil
	}
}

func sourceFromConfig(cfg *config.Config) jira.Source {
	return jira.Source{
		Board:              cfg.Jira.Board,
		Sprint:             cfg.Jira.Sprint,
		Columns:            cfg.Jira.Columns,
		Assignee:           cfg.Jira.Assignee,
		JQL:                cfg.Jira.JQL,
		SprintFieldID:      cfg.Jira.Fields.Sprint,
		StoryPointsFieldID: cfg.Jira.Fields.StoryPoints,
	}
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
