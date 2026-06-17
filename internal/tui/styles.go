package tui

import "charm.land/lipgloss/v2"

// Color palette: minimalist grayscale (no purple, no teal). Red/green show up
// only as semantic status (error / ok).
var (
	colorPrimary = lipgloss.Color("#E5E7EB") // light gray — titles, borders
	colorAccent  = lipgloss.Color("#9CA3AF") // mid gray — cursor, selection
	colorMuted   = lipgloss.Color("#626262")
	colorErr     = lipgloss.Color("#F87171")
	colorOK      = lipgloss.Color("#6EE7B7")
	colorText    = lipgloss.Color("#EEEEEE")
	colorInk     = lipgloss.Color("#111827") // dark text for light badges
)

// Mascot ("SprintMate" robozinho) pixel-art palette — grayscale shell with a
// dark "screen" face and bright eyes that recolor by mood.
var (
	mascotOutline  = lipgloss.Color("#374151") // body outline
	mascotShell    = lipgloss.Color("#9CA3AF") // body fill
	mascotHi       = lipgloss.Color("#E5E7EB") // highlights / mouth grille
	mascotScreen   = lipgloss.Color("#1F2937") // dark face screen
	mascotEye      = lipgloss.Color("#F9FAFB") // eyes (idle)
	mascotEyeErr   = lipgloss.Color("#F87171") // worried eyes
	mascotEyeHappy = lipgloss.Color("#6EE7B7") // happy eyes
)

// Glitch splash palette: bone-white skull, gray dither, a red glitch accent.
var (
	glitchBone   = lipgloss.Color("#E5E7EB")
	glitchShade  = lipgloss.Color("#6B7280")
	glitchAccent = lipgloss.Color("#F43F5E") // red glitch accent
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorInk). // dark text reads on the light badge
			Background(colorPrimary).
			Padding(0, 1)

	labelStyle  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	footerStyle = lipgloss.NewStyle().Foreground(colorMuted)
	helpStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	errStyle    = lipgloss.NewStyle().Foreground(colorErr).Bold(true)
	okStyle     = lipgloss.NewStyle().Foreground(colorOK)
	cursorStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	selStyle    = lipgloss.NewStyle().Foreground(colorAccent)
	dimStyle    = lipgloss.NewStyle().Foreground(colorMuted)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2)

	// panelStyle frames a sub-region (e.g. the form / preview columns of the
	// workdir step) inside the main box.
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)

	panelFocusStyle = panelStyle.BorderForeground(colorPrimary)

	panelTitleStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	dirStyle        = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true) // directories in the preview
	fileStyle       = lipgloss.NewStyle().Foreground(colorText)

	// mascotLineStyle styles the buddy's little spoken line next to the title.
	mascotLineStyle = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	// splashHintStyle dims the "press any key" line on the startup splash.
	splashHintStyle = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
)
