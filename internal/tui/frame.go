package tui

import "charm.land/lipgloss/v2"

// This file owns the shared "chrome" every main screen renders: the mascot
// header, an optional status strip, the body region, a footer status bar and the
// key-hints line. Centralizing it kills the per-screen hand-rolled JoinVertical
// blocks and the magic sizing numbers (Height-12 / Height-14) the screens used
// to guess with.

// twoColThreshold is the BODY width (not the terminal width) at/above which a
// screen renders a side-by-side split — the dashboard's list+detail and the
// review's plan|diff. Below it screens fall back to a single column.
const twoColThreshold = 88

// leftPanePct is the share of the body width the issue list takes in the
// dashboard's master-detail layout; the detail panel gets the rest.
const leftPanePct = 40

// minTwoColHeight keeps the split off cramped terminals where two panes would
// each be too short to be useful.
const minTwoColHeight = 18

// minBodyHeight floors the body so a tiny terminal still renders something.
const minBodyHeight = 3

// paneGap is the horizontal gap between the two panes of a split layout.
const paneGap = 2

// frameLayout is what computeLayout hands a screen BEFORE it builds its body, so
// the body (list / viewport / panels) can be sized to the real region instead of
// guessing.
type frameLayout struct {
	bodyWidth  int
	bodyHeight int
	twoColumn  bool
}

// chrome is one screen's renderable regions. The frame owns the vertical
// composition. strip / footer / hints may be "" to omit them.
type chrome struct {
	header string // mascot header (full sprite, untouched)
	strip  string // optional status strip under the header
	body   string // main region, already sized to bodyWidth × bodyHeight
	footer string // status bar (board/sprint/agent or running/pending/…)
	hints  string // key-hints line (may carry notice/status lines above it)
}

// frameReserve is the fixed vertical budget reserved around the body for the
// no-strip case: a blank under the header, a blank before the footer, the footer
// line, and up to three hint lines (update notice + status/notice + help). It
// matches the worst case the screens render, so the footer never scrolls off.
const frameReserve = 1 + 1 + 1 + 3

// computeLayout derives the body region from the RAW terminal size (the
// tea.WindowSizeMsg values). It subtracts the root appStyle Padding(1,2) — 4
// columns, 2 rows — then the measured header height and the chrome reserve.
//
// The header height is measured from a zero-value mascot on purpose: it's the
// sprite's rendered row count, which is independent of the animation frame, so
// Update (which never receives the live mascot) and View agree on the math.
func computeLayout(width, height int, hasStrip bool) frameLayout {
	availW := max(1, width-4)  // appStyle Padding(1,2): 2 left + 2 right
	availH := max(1, height-2) // appStyle Padding(1,2): 1 top + 1 bottom

	headerH := lipgloss.Height(mascot{}.header("", moodIdle))
	reserve := frameReserve
	if hasStrip {
		reserve += 1 + 1 // the strip line + a blank under it
	}

	return frameLayout{
		bodyWidth:  availW,
		bodyHeight: max(minBodyHeight, availH-headerH-reserve),
		twoColumn:  availW >= twoColThreshold && height >= minTwoColHeight,
	}
}

// renderFrame composes the chrome into the final screen string. The root's
// appStyle adds the outer padding, so the body stays left-aligned here.
func renderFrame(c chrome) string {
	parts := []string{c.header, ""}
	if c.strip != "" {
		parts = append(parts, c.strip, "")
	}
	parts = append(parts, c.body, "")
	if c.footer != "" {
		parts = append(parts, c.footer)
	}
	if c.hints != "" {
		parts = append(parts, c.hints)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// leftPaneWidth is the issue-list width in the dashboard's two-column body,
// clamped so the detail pane always keeps a usable minimum.
func leftPaneWidth(bodyW int) int {
	return clamp(bodyW*leftPanePct/100, 28, bodyW-30)
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// queueStats is a snapshot of the autonomous queue the root pushes into the
// dashboard, so the dashboard can render its status strip without importing the
// queue package or holding a (lifecycle-fragile) pointer to the engine.
type queueStats struct {
	running  int
	slots    int
	pending  int
	awaiting int
	active   bool // false until the monitor (engine) exists
}
