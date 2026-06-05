package onboarding

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/abrahamkuri/slack-tui/internal/theme"
)

var stepKicker = map[string]string{
	"theme": "appearance · 01", "accent": "appearance · 02", "density": "layout · 03",
	"keyboard": "tutorial · 04", "status": "presence · 05",
}
var stepHead = map[string][2]string{
	"theme":    {"Choose a theme", "Pick the palette for your workspace — the whole client follows your choice. j/k previews each live."},
	"accent":   {"Accent color", "The accent highlights the focused pane, your cursor, the mode bar, and selections."},
	"density":  {"Message density", "How tightly messages pack into the pane."},
	"keyboard": {"Learn the keys", "slack-tui is modal, like vim — NORMAL navigates, INSERT types. Clear each drill."},
	"status":   {"Set your presence", "How teammates see you. Change it anytime from the command palette (⌃K)."},
}

func (m Model) viewWizard(p theme.Palette) string {
	w := m.stageW()
	step := m.step()

	rule := lipgloss.NewStyle().Foreground(p.Dim).Render("setup  ") + m.stepRail(p)
	kicker := lipgloss.NewStyle().Foreground(p.Dim2).Render(stepKicker[step])
	hd := stepHead[step]
	head := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(hd[0])
	sub := wrapStyled(lipgloss.NewStyle().Foreground(p.Dim), hd[1], w)

	var body string
	switch step {
	case "theme", "accent", "density", "status":
		body = m.viewOptions(p, w)
	case "keyboard":
		body = m.viewTrainer(p, w)
	}

	footer := m.viewFooter(p, w)
	return lipgloss.JoinVertical(lipgloss.Left, rule, "", kicker, head, sub, "", body, "", footer)
}

func (m Model) stepRail(p theme.Palette) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(p.Dim2).Render("["))
	for i := range wizSteps {
		if i > 0 {
			c := p.Dim2
			if i <= m.stepIndex {
				c = p.Accent
			}
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render("──"))
		}
		glyph := "○"
		c := p.Dim2
		if i < m.stepIndex {
			glyph, c = "◉", p.Accent
		} else if i == m.stepIndex {
			glyph, c = "◉", p.Accent
		}
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render(glyph))
	}
	b.WriteString(lipgloss.NewStyle().Foreground(p.Dim2).Render("]"))
	return b.String()
}

func (m Model) viewOptions(p theme.Palette, w int) string {
	var rows []string
	switch m.step() {
	case "theme":
		for i, o := range themeOpts {
			name := lipgloss.NewStyle().Width(11).Render(o.name)
			rows = append(rows, optRow(p, w, i == m.optSel, i+1, name+swatch(o.val)))
		}
	case "accent":
		for i, o := range accentOpts {
			rows = append(rows, optRow(p, w, i == m.optSel, i+1, chip(p, o.val)+" "+o.name))
		}
	case "density":
		desc := map[string]string{"compact": "fits more on screen", "comfortable": "more room to breathe"}
		for i, o := range densityOpts {
			content := o.name + lipgloss.NewStyle().Foreground(p.Dim2).Render("  — "+desc[o.val])
			rows = append(rows, optRow(p, w, i == m.optSel, i+1, content))
		}
	case "status":
		for i, o := range statusOpts {
			dot := lipgloss.NewStyle().Foreground(presenceColor(p, o.val)).Render("●")
			content := dot + "  " + o.label + lipgloss.NewStyle().Foreground(p.Dim2).Render("  — "+o.desc)
			rows = append(rows, optRow(p, w, i == m.optSel, i+1, content))
		}
	}
	return strings.Join(rows, "\n")
}

// optRow renders a selectable wizard row with a leading number and selection bar.
func optRow(p theme.Palette, w int, selected bool, number int, content string) string {
	num := lipgloss.NewStyle().Foreground(p.Dim2).Render(itoa(number))
	check := "  "
	if selected {
		check = lipgloss.NewStyle().Foreground(p.Accent).Render("◉ ")
	}
	bar := "  "
	bgc := p.Bg
	if selected {
		bar = lipgloss.NewStyle().Foreground(p.Accent).Render("▌") + " "
		bgc = p.SelBg
	}
	line := bar + num + " " + check + content
	return lipgloss.NewStyle().Width(w).Background(bgc).Render(padRightTo(line, w))
}

// swatch renders a theme's 5-color preview strip.
func swatch(themeVal string) string {
	tp := theme.Resolve(themeVal, "auto")
	cols := []lipgloss.Color{tp.Bg, tp.Blue, tp.Green, tp.Purple, tp.Orange}
	var b strings.Builder
	for _, c := range cols {
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render("██"))
	}
	return b.String()
}

func chip(p theme.Palette, accentVal string) string {
	c := p.Accent
	switch accentVal {
	case "cyan":
		c = lipgloss.Color("#56d4dd")
	case "green":
		c = lipgloss.Color("#7ee787")
	case "purple":
		c = lipgloss.Color("#c8a2ff")
	case "orange":
		c = lipgloss.Color("#f0a868")
	case "magenta":
		c = lipgloss.Color("#ff7bd5")
	}
	return lipgloss.NewStyle().Foreground(c).Render("██")
}

func (m Model) viewFooter(p theme.Palette, w int) string {
	var back string
	if m.stepIndex > 0 {
		back = lipgloss.NewStyle().Foreground(p.Dim).Render("← back ") + lipgloss.NewStyle().Foreground(p.Dim2).Render("esc")
	}
	label := "continue ↵"
	if m.stepIndex == len(wizSteps)-1 {
		label = "finish ↵"
	}
	disabled := m.step() == "keyboard" && !m.kbDone
	cont := lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 1).Render(label)
	if disabled {
		cont = lipgloss.NewStyle().Foreground(p.Dim2).Render("complete the drills to continue")
	}
	gap := w - lipgloss.Width(back) - lipgloss.Width(cont)
	if gap < 1 {
		gap = 1
	}
	return back + strings.Repeat(" ", gap) + cont
}

func presenceColor(p theme.Palette, status string) lipgloss.Color {
	switch status {
	case "online":
		return p.Green
	case "away":
		return p.Yellow
	case "dnd":
		return p.Red
	default:
		return p.Dim2
	}
}

// ── small text utils ─────────────────────────────────────────────────────────

func wrapStyled(s lipgloss.Style, text string, w int) string {
	return s.Width(w).Render(text)
}

func padRightTo(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
