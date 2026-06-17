package tui

import (
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// mascot is SprintMate's companion: a tiny pixel-art robot drawn with half-block
// characters (▀ ▄ █) so it reads as a little sprite rather than an ASCII face.
// It only carries the animation frame; the host screen supplies the mood at
// render time (loading → working, error → worried, etc.).
type mascot struct {
	frame int
}

type mascotMood int

const (
	moodIdle mascotMood = iota
	moodWorking
	moodError
	moodHappy
)

// mascotTickMsg advances every sprite (blink, antenna pulse, eye scan).
type mascotTickMsg struct{}

const mascotInterval = 380 * time.Millisecond

func mascotTickCmd() tea.Cmd {
	return tea.Tick(mascotInterval, func(time.Time) tea.Msg { return mascotTickMsg{} })
}

func (m mascot) tick() mascot { m.frame++; return m }

// mascotGrid is the pixel map of the robot, one rune per pixel. Rows are paired
// into half-block cells at render time, so an even row count keeps it crisp.
//
//	a = antenna tip   o = outline   s = shell   h = highlight/mouth grille
//	c = dark face screen   e = eye
var mascotGrid = []string{
	"......a.....",
	"......o.....",
	".oooooooooo.",
	"osssssssssso",
	"osccccccccso",
	"osceecceecso",
	"oscchhhhccso",
	".oooooooooo.",
	".oossssssoo.",
	".osssssssso.",
	".oooooooooo.",
	"...oo..oo...",
}

// view renders the mascot sprite (multi-line, colored) for the given mood.
func (m mascot) view(mood mascotMood) string {
	rows := mascotGrid
	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}

	var b strings.Builder
	for y := 0; y+1 < len(rows); y += 2 {
		for x := 0; x < width; x++ {
			topC, topOn := m.pixelColor(pixelAt(rows[y], x), mood)
			botC, botOn := m.pixelColor(pixelAt(rows[y+1], x), mood)
			switch {
			case topOn && botOn:
				b.WriteString(lipgloss.NewStyle().Foreground(topC).Background(botC).Render("▀"))
			case topOn:
				b.WriteString(lipgloss.NewStyle().Foreground(topC).Render("▀"))
			case botOn:
				b.WriteString(lipgloss.NewStyle().Foreground(botC).Render("▄"))
			default:
				b.WriteByte(' ')
			}
		}
		if y+2 < len(rows) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// header lays the sprite beside a title badge and the buddy's spoken line.
func (m mascot) header(title string, mood mascotMood) string {
	right := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		mascotLineStyle.Render(m.say(mood)),
	)
	return lipgloss.JoinHorizontal(lipgloss.Center, m.view(mood), "  ", right)
}

// pixelColor maps a grid rune to its color for the current mood and frame,
// returning ok=false for transparent pixels.
func (m mascot) pixelColor(r rune, mood mascotMood) (color.Color, bool) {
	switch r {
	case '.':
		return nil, false
	case 'o':
		return mascotOutline, true
	case 's':
		return mascotShell, true
	case 'h':
		return mascotHi, true
	case 'c':
		return mascotScreen, true
	case 'a': // antenna tip blinks
		if m.frame%2 == 0 {
			return mascotHi, true
		}
		return mascotShell, true
	case 'e':
		return m.eyeColor(mood), true
	}
	return mascotShell, true
}

func (m mascot) eyeColor(mood mascotMood) color.Color {
	switch mood {
	case moodWorking: // pulse, like it's thinking
		if m.frame%2 == 0 {
			return mascotEye
		}
		return mascotOutline
	case moodError:
		return mascotEyeErr
	case moodHappy:
		return mascotEyeHappy
	default:
		if m.frame%18 == 7 { // occasional blink (eyes melt into the screen)
			return mascotScreen
		}
		return mascotEye
	}
}

var mascotSay = map[mascotMood][]string{
	moodIdle: {
		"bora pra sprint!",
		"qual issue a gente ataca?",
		"tô de prontidão",
		"partiu trampar",
	},
	moodWorking: {
		"buscando no Jira...",
		"caçando issues...",
		"já tô indo...",
	},
	moodError: {
		"opa, deu ruim",
		"algo travou aqui",
		"bora tentar de novo?",
	},
	moodHappy: {
		"mandando ver!",
		"bora!",
	},
}

// say picks a phrase, rotating slowly so it lingers a few seconds.
func (m mascot) say(mood mascotMood) string {
	list := mascotSay[mood]
	if len(list) == 0 {
		return ""
	}
	return list[(m.frame/12)%len(list)]
}

func pixelAt(row string, x int) rune {
	if x < len(row) {
		return rune(row[x])
	}
	return '.'
}
