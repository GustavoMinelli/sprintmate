package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GustavoMinelli/sprintmate/internal/agents"
	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
)

type wizardStep int

const (
	stepAPI wizardStep = iota
	stepBoard
	stepColumns
	stepSprint
	stepComment
	stepAgent
	stepWorkdir
)

// wizard is the setup screen: connect to Jira, then pick board / columns /
// sprint / default agent / working directory, and save the config.
type wizard struct {
	cfg        *config.Config
	isSettings bool // reopened from the dashboard (vs first-run)
	step       wizardStep

	// API step
	inputs  []textinput.Model // host, email, token
	focus   int
	testing bool

	client *jira.Client

	// board step
	boards   []jira.Board
	boardCur int
	loading  bool

	// columns step
	columns  []jira.Column
	colCur   int
	colCheck map[int]bool

	// sprint step
	sprintOpts []string
	sprintCur  int

	// comment step: opt-in to posting a "started" note to Jira on launch
	commentOpts []string
	commentCur  int

	// agent step
	agentOpts []string
	agentCur  int

	// workdir step: type a folder; the folder search drills in on Enter, and a
	// valid folder finishes the wizard. Every issue launches in this directory.
	workdir textinput.Model

	err           string
	width, height int
}

func newWizard(cfg *config.Config, isSettings bool) wizard {
	host := textinput.New()
	host.Placeholder = "https://company.atlassian.net"
	host.SetValue(cfg.Jira.Host)
	host.SetWidth(48)

	email := textinput.New()
	email.Placeholder = "you@company.com"
	email.SetValue(cfg.Jira.Email)
	email.SetWidth(48)

	token := textinput.New()
	token.EchoMode = textinput.EchoPassword
	token.SetWidth(48)
	if os.Getenv(config.EnvToken) != "" {
		// Token comes from the environment; don't seed it (and don't persist it).
		token.Placeholder = "using $" + config.EnvToken
	} else {
		token.Placeholder = "API token"
		token.SetValue(cfg.Jira.Token)
	}

	wd := textinput.New()
	wd.Placeholder = "~/Documents/projetos"
	wd.Prompt = "" // we render our own focus marker via projField()
	wd.SetWidth(40)
	wd.SetValue(cfg.Workdir)

	return wizard{
		cfg:        cfg,
		isSettings: isSettings,
		inputs:     []textinput.Model{host, email, token},
		sprintOpts: []string{config.SprintActive, config.SprintFuture, config.SprintAll},
		sprintCur:  sprintIndex(cfg.Jira.Sprint),
		commentOpts: []string{
			"No — keep my activity private (default)",
			"Yes — post a “started via SprintMate” note on the issue",
		},
		commentCur: boolIndex(cfg.Jira.OnLaunch.Comment),
		agentOpts:  agents.Names(),
		agentCur:   max(0, slices.Index(agents.Names(), cfg.Agent.Default)),
		workdir:    wd,
	}
}

func (w wizard) Init() tea.Cmd { return w.inputs[0].Focus() }

func (w wizard) Update(msg tea.Msg) (wizard, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width, w.height = msg.Width, msg.Height
		return w, nil

	case connTestedMsg:
		w.testing = false
		if msg.err != nil {
			w.err = friendlyErr(msg.err)
			return w, nil
		}
		w.err = ""
		w.loading = true
		return w, loadBoardsCmd(w.client)

	case boardsLoadedMsg:
		w.loading = false
		if msg.err != nil {
			w.err = friendlyErr(msg.err)
			return w, nil
		}
		w.boards = msg.boards
		w.boardCur = boardIndex(msg.boards, w.cfg.Jira.Board)
		w.step = stepBoard
		return w, nil

	case columnsLoadedMsg:
		w.loading = false
		if msg.err != nil {
			w.err = friendlyErr(msg.err)
			return w, nil
		}
		w.columns = msg.cols
		w.colCheck = preselectColumns(msg.cols, w.cfg.Jira.Columns)
		w.colCur = 0
		w.step = stepColumns
		return w, nil

	case tea.KeyPressMsg:
		return w.handleKey(msg)
	}

	// Forward to the focused text input on input steps.
	return w.forwardInput(msg)
}

func (w wizard) handleKey(msg tea.KeyPressMsg) (wizard, tea.Cmd) {
	k := msg.String()

	// Esc goes back a step (or cancels on the first step).
	if k == "esc" {
		return w.back()
	}

	switch w.step {
	case stepAPI:
		switch k {
		case "tab", "down":
			return w.refocus(w.focus + 1)
		case "shift+tab", "up":
			return w.refocus(w.focus - 1)
		case "enter":
			if w.testing {
				return w, nil
			}
			envToken := os.Getenv(config.EnvToken)
			if w.host() == "" || w.email() == "" || (w.token() == "" && envToken == "") {
				w.err = "enter host, email and token"
				return w, nil
			}
			tok := w.token()
			if tok == "" {
				tok = envToken // use the env token for the connection test
			}
			w.testing = true
			w.err = ""
			w.client = jira.New(w.host(), w.email(), tok)
			return w, testConnCmd(w.client)
		}
		return w.forwardInput(msg)

	case stepBoard:
		switch k {
		case "up", "k":
			w.boardCur = clampDec(w.boardCur)
		case "down", "j":
			w.boardCur = clampInc(w.boardCur, len(w.boards))
		case "enter":
			if len(w.boards) == 0 {
				return w, nil
			}
			w.loading = true
			return w, loadColumnsCmd(w.client, w.boards[w.boardCur].ID)
		}
		return w, nil

	case stepColumns:
		switch k {
		case "up", "k":
			w.colCur = clampDec(w.colCur)
		case "down", "j":
			w.colCur = clampInc(w.colCur, len(w.columns))
		case " ", "space":
			w.colCheck[w.colCur] = !w.colCheck[w.colCur]
		case "enter":
			w.step = stepSprint
		}
		return w, nil

	case stepSprint:
		switch k {
		case "up", "k":
			w.sprintCur = clampDec(w.sprintCur)
		case "down", "j":
			w.sprintCur = clampInc(w.sprintCur, len(w.sprintOpts))
		case "enter":
			w.step = stepComment
		}
		return w, nil

	case stepComment:
		switch k {
		case "up", "k":
			w.commentCur = clampDec(w.commentCur)
		case "down", "j":
			w.commentCur = clampInc(w.commentCur, len(w.commentOpts))
		case "enter":
			w.step = stepAgent
		}
		return w, nil

	case stepAgent:
		switch k {
		case "up", "k":
			w.agentCur = clampDec(w.agentCur)
		case "down", "j":
			w.agentCur = clampInc(w.agentCur, len(w.agentOpts))
		case "enter":
			w.step = stepWorkdir
			return w, w.workdir.Focus()
		}
		return w, nil

	case stepWorkdir:
		switch k {
		case "enter", "tab":
			return w.confirmWorkdir()
		}
		return w.forwardInput(msg)
	}
	return w, nil
}

// confirmWorkdir drives the folder search: while the user is typing a partial
// name it drills into the first matching subfolder ("type, Enter, it fills in");
// once the typed value is itself a directory it finishes the wizard.
func (w wizard) confirmWorkdir() (wizard, tea.Cmd) {
	value := strings.TrimSpace(w.workdir.Value())
	if value == "" {
		w.err = "enter the working directory"
		return w, nil
	}
	if base, leaf, matches := dirCompletion(value); leaf != "" && len(matches) > 0 {
		w.workdir.SetValue(filepath.Join(base, matches[0]) + "/")
		w.workdir.CursorEnd() // SetValue keeps the old cursor pos when the value grows
		w.err = ""
		return w, nil
	}
	resolved := filepath.Clean(resolveDir(value))
	if !isDir(resolved) {
		w.err = "folder not found: " + resolved
		return w, nil
	}
	w.workdir.SetValue(resolved)
	return w.finish()
}

// back moves to the previous step, or cancels the wizard from the first step.
func (w wizard) back() (wizard, tea.Cmd) {
	w.err = ""
	switch w.step {
	case stepAPI:
		if w.isSettings {
			return w, func() tea.Msg { return wizardCancelMsg{} }
		}
		return w, tea.Quit
	case stepBoard:
		w.step = stepAPI
	case stepColumns:
		w.step = stepBoard
	case stepSprint:
		w.step = stepColumns
	case stepComment:
		w.step = stepSprint
	case stepAgent:
		w.step = stepComment
	case stepWorkdir:
		w.step = stepAgent
	}
	return w, nil
}

// finish assembles the config, saves it and signals completion.
func (w wizard) finish() (wizard, tea.Cmd) {
	c := w.cfg
	c.Jira.Host = w.host()
	c.Jira.Email = w.email()
	// Only persist a token the user actually typed. If they rely on the env var
	// (blank input), keep it out of the file via the env provenance flag.
	if w.token() != "" || os.Getenv(config.EnvToken) == "" {
		c.SetToken(w.token())
	}
	if len(w.boards) > 0 {
		c.Jira.Board = w.boards[w.boardCur].Name
	}
	c.Jira.Columns = w.selectedColumns()
	c.Jira.Sprint = w.sprintOpts[w.sprintCur]
	c.Jira.OnLaunch.Comment = w.commentCur == 1
	if c.Jira.Assignee == "" {
		c.Jira.Assignee = "currentUser"
	}
	if len(w.agentOpts) > 0 {
		c.Agent.Default = w.agentOpts[w.agentCur]
	}
	// Store the resolved, cleaned absolute path so it can't later resolve against
	// the process working dir at launch time.
	if wd := strings.TrimSpace(w.workdir.Value()); wd != "" {
		c.Workdir = filepath.Clean(resolveDir(wd))
	} else {
		c.Workdir = ""
	}
	if err := config.Save(c); err != nil {
		w.err = "error saving config: " + err.Error()
		return w, nil
	}
	return w, func() tea.Msg { return wizardDoneMsg{cfg: c} }
}

func (w wizard) forwardInput(msg tea.Msg) (wizard, tea.Cmd) {
	var cmd tea.Cmd
	switch w.step {
	case stepAPI:
		w.inputs[w.focus], cmd = w.inputs[w.focus].Update(msg)
	case stepWorkdir:
		w.workdir, cmd = w.workdir.Update(msg)
	}
	return w, cmd
}

func (w wizard) refocus(i int) (wizard, tea.Cmd) {
	if i < 0 {
		i = len(w.inputs) - 1
	}
	i %= len(w.inputs)
	for j := range w.inputs {
		w.inputs[j].Blur()
	}
	w.focus = i
	return w, w.inputs[i].Focus()
}

// --- view -----------------------------------------------------------------

func (w wizard) View(mas mascot) string {
	mood := moodIdle
	switch {
	case w.testing || w.loading:
		mood = moodWorking
	case w.err != "":
		mood = moodError
	}
	title := mas.header("SprintMate · Setup", mood)

	// Before we know the terminal size (first frame / tests) or on tiny windows,
	// fall back to the compact, content-sized box.
	if w.width < 70 || w.height < 18 {
		body := w.stepView()
		if w.loading {
			body = dimStyle.Render("Loading from Jira...")
		}
		inner := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", w.footerView())
		return boxStyle.Render(inner)
	}

	// Fill the terminal: the root wraps us in appStyle (Padding(1,2)), so the
	// box gets the full size minus that padding, and the box's own border(2)+
	// padding(1,2 => 4 cols / 2 rows) leaves the inner content area below.
	availW := w.width - 4
	availH := w.height - 2
	innerW := availW - 6 // border (2) + horizontal padding (4)
	innerH := availH - 4 // border (2) + vertical padding (2)

	// Wrap the footer to the content width up front so its true (wrapped) height
	// is known — otherwise a long help line silently grows the box past availH.
	footer := lipgloss.NewStyle().Width(innerW).Render(w.footerView())
	bodyH := innerH - lipgloss.Height(title) - 1 - lipgloss.Height(footer) - 1 // blanks above body and footer
	bodyH = max(3, bodyH)

	var body string
	switch {
	case w.loading:
		body = dimStyle.Render("Loading from Jira...")
	case w.step == stepWorkdir:
		body = w.workdirView(innerW, bodyH)
	default:
		body = w.stepView()
	}
	body = lipgloss.Place(innerW, bodyH, lipgloss.Left, lipgloss.Top, body)

	inner := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", footer)
	return boxStyle.Width(availW).Height(availH).Render(inner)
}

// footerView renders the help line, prefixed with the current error if any.
func (w wizard) footerView() string {
	help := helpStyle.Render(w.footerHelp())
	if w.err != "" {
		return errStyle.Render("⚠ "+w.err) + "\n" + help
	}
	return help
}

// workdirView is the full-terminal layout for the working-directory step: the
// folder input on the left, a live preview of the typed folder on the right.
// On narrower terminals the two stack vertically instead.
func (w wizard) workdirView(innerW, bodyH int) string {
	const gap = 2
	if innerW >= 70 { // side by side
		leftW := max(36, innerW*42/100)
		leftW = min(leftW, innerW-32-gap)
		rightW := innerW - leftW - gap
		left := panelFocusStyle.Width(leftW).Height(bodyH).Render(w.workdirFormPanel(leftW - 4))
		right := panelStyle.Width(rightW).Height(bodyH).Render(w.dirPreviewPanel(rightW-4, bodyH-2))
		return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
	}

	topH := max(6, bodyH/2)
	botH := bodyH - topH - 1
	top := panelFocusStyle.Width(innerW).Height(topH).Render(w.workdirFormPanel(innerW - 4))
	bot := panelStyle.Width(innerW).Height(botH).Render(w.dirPreviewPanel(innerW-4, botH-2))
	return lipgloss.JoinVertical(lipgloss.Left, top, "", bot)
}

// workdirFormPanel renders the folder input plus a hint and validity mark.
func (w wizard) workdirFormPanel(contentW int) string {
	inputW := max(6, contentW-11) // projField() prefixes a 10-col label + slack
	w.workdir.SetWidth(inputW)

	var b strings.Builder
	b.WriteString(panelTitleStyle.Render("Working directory") + "\n\n")
	b.WriteString(projField("Folder", w.workdir, true, inputW) + "\n\n")
	b.WriteString(dimStyle.Render("Where the agents will run.") + "\n")
	b.WriteString(dimStyle.Render("Every issue opens in this folder.") + "\n")
	if v := strings.TrimSpace(w.workdir.Value()); v != "" {
		if resolved := resolveDir(v); isDir(resolved) {
			b.WriteString("\n" + okStyle.Render("✓ "+truncate(resolved, max(3, contentW-2))))
		}
	}
	return b.String()
}

// dirPreviewPanel renders a live preview of the folder being typed.
func (w wizard) dirPreviewPanel(contentW, contentH int) string {
	header := panelTitleStyle.Render("Find folder")
	preview := renderDirCompletion(w.workdir.Value(), contentW, contentH-2)
	return header + "\n\n" + preview
}

func (w wizard) stepView() string {
	switch w.step {
	case stepAPI:
		status := ""
		if w.testing {
			status = dimStyle.Render("testing connection...")
		}
		rows := []string{
			labelStyle.Render("1/7 · Jira API connection"),
			"",
			field("Host ", w.inputs[0], w.focus == 0),
			field("Email", w.inputs[1], w.focus == 1),
			field("Token", w.inputs[2], w.focus == 2),
			"",
			status,
		}
		return lipgloss.JoinVertical(lipgloss.Left, rows...)

	case stepBoard:
		names := make([]string, len(w.boards))
		for i, b := range w.boards {
			names[i] = b.Name
		}
		return label("2/7 · Choose board") + "\n\n" + renderChoices(names, w.boardCur, nil)

	case stepColumns:
		names := make([]string, len(w.columns))
		for i, c := range w.columns {
			names[i] = c.Name
		}
		return label("3/7 · Columns to pull (space toggles)") + "\n\n" + renderChoices(names, w.colCur, w.colCheck)

	case stepSprint:
		return label("4/7 · Sprint") + "\n\n" + renderChoices(w.sprintOpts, w.sprintCur, nil)

	case stepComment:
		help := dimStyle.Render(
			"When on, SprintMate posts a short “started via SprintMate” note on the\n" +
				"issue each time you launch it, so the team can see work has begun.\n" +
				"Leave it off to keep your activity private — nothing is written to Jira.")
		return label("5/7 · Post a comment to Jira when you launch an issue?") + "\n\n" +
			renderChoices(w.commentOpts, w.commentCur, nil) + "\n" + help

	case stepAgent:
		opts := make([]string, len(w.agentOpts))
		for i, a := range w.agentOpts {
			mark := dimStyle.Render(" (not installed)")
			if ag, ok := agents.Get(a); ok && ag.IsInstalled() {
				mark = okStyle.Render(" (installed)")
			}
			opts[i] = a + mark
		}
		return label("6/7 · Default agent") + "\n\n" + renderChoices(opts, w.agentCur, nil)

	case stepWorkdir:
		var b strings.Builder
		b.WriteString(label("7/7 · Working directory") + "\n\n")
		b.WriteString(projField("Folder", w.workdir, true, 40) + "\n")
		if path := strings.TrimSpace(w.workdir.Value()); path != "" {
			if _, leaf, matches := dirCompletion(path); leaf != "" && len(matches) > 0 {
				b.WriteString(dimStyle.Render("  enter → "+truncate(matches[0]+"/", 46)) + "\n")
			} else if isDir(resolveDir(path)) {
				b.WriteString(okStyle.Render("  ✓ "+truncate(resolveDir(path), 50)) + "\n")
			} else {
				b.WriteString(errStyle.Render("  ✗ folder not found") + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render("Every issue opens in this folder."))
		return b.String()
	}
	return ""
}

func (w wizard) footerHelp() string {
	switch w.step {
	case stepAPI:
		return "tab: field · enter: test and continue · esc: " + cancelLabel(w.isSettings)
	case stepColumns:
		return "↑/↓ move · space toggle · enter: continue · esc: back"
	case stepWorkdir:
		return "type to search · enter: open folder · enter on a valid folder: finish · esc: back"
	default:
		return "↑/↓ move · enter: choose · esc: back"
	}
}

// --- helpers ---------------------------------------------------------------

func (w wizard) host() string  { return strings.TrimSpace(w.inputs[0].Value()) }
func (w wizard) email() string { return strings.TrimSpace(w.inputs[1].Value()) }
func (w wizard) token() string { return strings.TrimSpace(w.inputs[2].Value()) }

func (w wizard) selectedColumns() []string {
	var out []string
	for i, c := range w.columns {
		if w.colCheck[i] {
			out = append(out, c.Name)
		}
	}
	return out
}

func renderChoices(items []string, cursor int, checked map[int]bool) string {
	if len(items) == 0 {
		return dimStyle.Render("  (nothing found)")
	}
	var b strings.Builder
	for i, it := range items {
		prefix := "  "
		if i == cursor {
			prefix = cursorStyle.Render("> ")
		}
		box := ""
		if checked != nil {
			if checked[i] {
				box = okStyle.Render("[x] ")
			} else {
				box = "[ ] "
			}
		}
		line := prefix + box + it
		if i == cursor {
			line = prefix + box + selStyle.Render(it)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func field(labelText string, ti textinput.Model, focused bool) string {
	var l string
	if focused {
		l = cursorStyle.Render("› " + labelText + " ")
	} else {
		l = "  " + labelText + " "
	}
	return l + ti.View()
}

func label(s string) string { return labelStyle.Render(s) }

// projField renders a labelled input that never overflows its panel: a focused
// input scrolls within its set width, a blurred one shows a truncated value (or
// dimmed placeholder) so a long path can't wrap the layout.
func projField(labelText string, ti textinput.Model, focused bool, inputW int) string {
	if focused {
		return cursorStyle.Render("› "+labelText+" ") + ti.View()
	}
	prefix := "  " + labelText + " "
	if v := ti.Value(); v != "" {
		return prefix + truncate(v, inputW)
	}
	return prefix + dimStyle.Render(truncate(ti.Placeholder, inputW))
}

// resolveDir turns a typed folder into an absolute path: it expands a leading
// ~ and resolves any relative path against the user's HOME directory — never
// the process working dir, which is the project itself when run via `go run`.
func resolveDir(path string) string {
	p := config.ExpandPath(strings.TrimSpace(path))
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, p)
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// isDir reports whether path is an existing directory.
func isDir(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// homeDir returns the user's home directory, or "" if it can't be resolved.
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// dirCompletion turns the typed folder value into autocomplete state: the
// directory whose entries we list (base), the partial last segment used to
// filter them (leaf) and the matching subdirectories (case-insensitive prefix).
// A blank value or one ending in "/" lists every subdirectory of base; an empty
// value falls back to HOME so the user sees their own folders. This is what
// powers the "type, Enter, it fills in" search: matches[0] is what Enter places.
func dirCompletion(value string) (base, leaf string, matches []string) {
	v := strings.TrimSpace(value)
	switch {
	case v == "":
		base, leaf = homeDir(), ""
	case strings.HasSuffix(v, "/"):
		base, leaf = resolveDir(v), ""
	default:
		// Split off the partial name; everything before the last "/" is the
		// directory we list, anchored to HOME when no separator was typed.
		if i := strings.LastIndex(v, "/"); i >= 0 {
			base, leaf = resolveDir(v[:i+1]), v[i+1:]
		} else {
			base, leaf = homeDir(), v
		}
	}
	if base == "" {
		base = homeDir()
	}

	dirs, err := listDir(base, 800)
	if err != nil {
		return base, leaf, nil
	}
	if leaf == "" {
		return base, leaf, dirs
	}
	low := strings.ToLower(leaf)
	for _, d := range dirs {
		if strings.HasPrefix(strings.ToLower(d), low) {
			matches = append(matches, d)
		}
	}
	return base, leaf, matches
}

// renderDirCompletion previews the folder search: a status line plus the
// matching subdirectories, the first one highlighted as the candidate Enter
// will drill into. Bounded to width × height.
func renderDirCompletion(value string, width, height int) string {
	width = max(1, width)
	height = max(1, height)

	base, leaf, matches := dirCompletion(value)

	head := dimStyle.Render(truncate("searching in "+base, width))
	if resolved := resolveDir(strings.TrimSpace(value)); strings.TrimSpace(value) != "" && isDir(resolved) {
		head = okStyle.Render(truncate("✓ "+resolved, width))
	}

	if len(matches) == 0 {
		hint := "(no folders here)"
		if leaf != "" {
			hint = "(no folder starts with “" + leaf + "”)"
		}
		return head + "\n\n" + dimStyle.Render(truncate(hint, width))
	}

	lines := make([]string, 0, len(matches))
	for i, m := range matches {
		if i == 0 {
			lines = append(lines, cursorStyle.Render("> ")+selStyle.Render(truncate(m+"/", width-2)))
		} else {
			lines = append(lines, "  "+dirStyle.Render(truncate(m+"/", width-2)))
		}
	}

	rows := max(1, height-2) // reserve the status line + blank
	if len(lines) > rows {
		hidden := len(lines) - (rows - 1)
		lines = append(lines[:rows-1], dimStyle.Render(fmt.Sprintf("… +%d", hidden)))
	}
	return head + "\n\n" + strings.Join(lines, "\n")
}

// listDir lists up to limit subdirectories of dir, sorted case-insensitively.
// The error is surfaced (e.g. permission denied) so the browser can show why a
// folder can't be opened. The folder search is directories-only by design.
func listDir(dir string, limit int) (dirs []string, err error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	entries, _ := f.ReadDir(limit)
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

// truncate shortens s to at most max display columns, adding an ellipsis.
// It expects unstyled text (no ANSI escapes).
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		cw := lipgloss.Width(string(r))
		if w+cw > max-1 {
			break
		}
		b.WriteRune(r)
		w += cw
	}
	return b.String() + "…"
}

func cancelLabel(isSettings bool) string {
	if isSettings {
		return "cancel"
	}
	return "quit"
}

// wizardNetTimeout bounds each wizard network step so a slow Jira can't hang
// the setup screen indefinitely.
const wizardNetTimeout = 30 * time.Second

func testConnCmd(c *jira.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), wizardNetTimeout)
		defer cancel()
		_, err := c.TestConnection(ctx)
		return connTestedMsg{err: err}
	}
}

func loadBoardsCmd(c *jira.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), wizardNetTimeout)
		defer cancel()
		b, err := c.ListBoards(ctx)
		return boardsLoadedMsg{boards: b, err: err}
	}
}

func loadColumnsCmd(c *jira.Client, boardID int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), wizardNetTimeout)
		defer cancel()
		cols, err := c.BoardColumns(ctx, boardID)
		return columnsLoadedMsg{cols: cols, err: err}
	}
}

func sprintIndex(s string) int {
	switch s {
	case config.SprintFuture:
		return 1
	case config.SprintAll:
		return 2
	default:
		return 0
	}
}

func boolIndex(b bool) int {
	if b {
		return 1
	}
	return 0
}

func boardIndex(boards []jira.Board, nameOrID string) int {
	for i, b := range boards {
		if b.Name == nameOrID || fmt.Sprint(b.ID) == nameOrID {
			return i
		}
	}
	return 0
}

func preselectColumns(cols []jira.Column, configured []string) map[int]bool {
	m := map[int]bool{}
	want := map[string]bool{}
	for _, c := range configured {
		want[strings.ToLower(c)] = true
	}
	for i, c := range cols {
		// default: all selected when nothing configured yet
		m[i] = len(want) == 0 || want[strings.ToLower(c.Name)]
	}
	return m
}

// friendlyErr maps the common connection failures to a plain, friendly message
// for the type-and-Enter audience, passing anything else through verbatim.
func friendlyErr(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, jira.ErrAuth) {
		return err.Error() // already a clear, actionable sentence
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out talking to Jira — check the host and your connection"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timed out talking to Jira — check the host and your connection"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "couldn't resolve the Jira host — check the address"
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "dial tcp") {
		return "couldn't connect to Jira — check the host and your connection"
	}
	return msg
}

func clampDec(i int) int {
	if i > 0 {
		return i - 1
	}
	return 0
}

func clampInc(i, n int) int {
	if i < n-1 {
		return i + 1
	}
	if n == 0 {
		return 0
	}
	return n - 1
}
