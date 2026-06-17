package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// bannerFont is a 5-row block alphabet, just the letters SprintMate needs.
var bannerFont = map[rune][]string{
	'S': {"█████", "█    ", "█████", "    █", "█████"},
	'P': {"█████", "█   █", "█████", "█    ", "█    "},
	'R': {"█████", "█   █", "█████", "█  █ ", "█   █"},
	'I': {"███", " █ ", " █ ", " █ ", "███"},
	'N': {"█   █", "██  █", "█ █ █", "█  ██", "█   █"},
	'T': {"█████", "  █  ", "  █  ", "  █  ", "  █  "},
	'M': {"█   █", "██ ██", "█ █ █", "█   █", "█   █"},
	'A': {"█████", "█   █", "█████", "█   █", "█   █"},
	'E': {"█████", "█    ", "█████", "█    ", "█████"},
}

// renderBanner spells text in the 5-row block font, letters separated by a
// column of space. Unknown runes are skipped.
func renderBanner(text string) string {
	rows := make([]string, 5)
	for i, ch := range text {
		glyph, ok := bannerFont[ch]
		if !ok {
			continue
		}
		for r := 0; r < 5; r++ {
			if i > 0 {
				rows[r] += " "
			}
			rows[r] += glyph[r]
		}
	}
	return strings.Join(rows, "\n")
}

// glitchSkull is a blocky stencil skull with hollow eye sockets and a tooth
// row — the centerpiece of the glitch splash.
var glitchSkull = []string{
	"  ▄▄▄▄▄▄▄▄▄▄▄▄  ",
	" ▟████████████▙ ",
	" ██████████████ ",
	" ███  ████  ███ ",
	" ███  ████  ███ ",
	" ██████████████ ",
	"  ████▄▄▄█████ ",
	"  ██ █ ██ █ ██ ",
	"  ▀▀▀▀▀▀▀▀▀▀▀▀  ",
}

var (
	boneStyle   = lipgloss.NewStyle().Foreground(glitchBone).Bold(true)
	shadeStyle  = lipgloss.NewStyle().Foreground(glitchShade)
	glitchStyle = lipgloss.NewStyle().Foreground(glitchAccent).Bold(true)
)

// corrupt swaps a few solid blocks for dithered ones, deterministically from
// seed, so the glitch shimmers without changing the line width.
func corrupt(line string, seed int) string {
	runes := []rune(line)
	for i, r := range runes {
		if r != '█' {
			continue
		}
		switch (i*7 + seed) % 11 {
		case 0:
			runes[i] = '▓'
		case 3:
			runes[i] = '▒'
		case 6:
			runes[i] = '░'
		}
	}
	return string(runes)
}

// glitch renders a multi-line block with a hacker flicker: most rows are bone,
// the occasional row jitters into a red, dithered, shifted ghost.
func glitch(block string, frame int) string {
	lines := strings.Split(block, "\n")
	out := make([]string, len(lines))
	for i, ln := range lines {
		switch (frame*5 + i*7) % 23 {
		case 0, 11: // red, shifted, corrupted scanline
			out[i] = glitchStyle.Render(" " + corrupt(ln, frame+i))
		case 4: // faint dithered ghost
			out[i] = shadeStyle.Render(corrupt(ln, frame*2+i))
		default:
			out[i] = boneStyle.Render(ln)
		}
	}
	return strings.Join(out, "\n")
}

// splashView is the startup screen: a glitching skull over a corrupted
// SprintMate wordmark, with the robozinho tagged into the corner. It waits for
// a key press (no auto-dismiss).
func splashView(mc mascot, width, height int) string {
	skull := glitch(strings.Join(glitchSkull, "\n"), mc.frame)
	tag := glitchStyle.Render("‹ ACESSO LIBERADO ›")
	hint := splashHintStyle.Render("// pressione qualquer tecla")

	// Narrow terminals: skull + tag only, no 57-col wordmark.
	if width > 0 && width < 64 {
		small := lipgloss.JoinVertical(lipgloss.Center, skull, "", tag, "", hint)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, small)
	}

	word := glitch(renderBanner("SPRINTMATE"), mc.frame)
	block := lipgloss.JoinVertical(lipgloss.Center, skull, "", word, "", tag, "", hint)
	robot := mc.view(moodIdle)

	if width <= 0 || height <= 0 {
		return lipgloss.JoinVertical(lipgloss.Left, block, "", robot)
	}

	topH := max(1, height-lipgloss.Height(robot)-1)
	top := lipgloss.Place(width, topH, lipgloss.Center, lipgloss.Center, block)
	bottom := lipgloss.PlaceHorizontal(width, lipgloss.Right, robot)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}
