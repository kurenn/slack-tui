package onboarding

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/abrahamkuri/slack-tui/internal/theme"
)

var stepKicker = map[string]string{
	"theme": "appearance · 01", "accent": "appearance · 02", "density": "layout · 03",
	"keyboard": "tutorial · 04", "status": "presence · 05",
}
var stepHead = map[string][2]string{
	"theme":    {"Choose a theme", "The whole client follows your choice. j/k previews each live."},
	"accent":   {"Accent color", "Highlights the focused pane, your cursor, and selections."},
	"density":  {"Message density", "How tightly messages pack into the pane."},
	"keyboard": {"Learn the keys", "Modal, like vim — NORMAL navigates, INSERT types. Clear each drill."},
	"status":   {"Set your presence", "How teammates see you — change it anytime from ⌃K."},
}

// wizHeading is the shared kicker + title + subtitle block for a wizard step.
func (m Model) wizHeading(p theme.Palette) string {
	w := m.contentW()
	hd := stepHead[m.step()]
	kicker := lipgloss.NewStyle().Foreground(p.Dim2).Render(stepKicker[m.step()])
	head := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(hd[0])
	sub := wrapStyled(lipgloss.NewStyle().Foreground(p.Dim), hd[1], w)
	return lipgloss.JoinVertical(lipgloss.Left, kicker, head, sub)
}

func (m Model) viewWizardBody(p theme.Palette) string {
	return lipgloss.JoinVertical(lipgloss.Left, m.wizHeading(p), "", m.viewOptions(p, m.contentW()))
}

func (m Model) viewOptions(p theme.Palette, w int) string {
	num := func(i int) string { return lipgloss.NewStyle().Foreground(p.Dim2).Render(fmt.Sprintf("%d ", i+1)) }
	var rows []string
	switch m.step() {
	case "theme":
		for i, o := range themeOpts {
			name := lipgloss.NewStyle().Foreground(p.Fg).Width(11).Render(o.name)
			rows = append(rows, selRow(p, w, i == m.optSel, num(i)+name+swatch(o.val), ""))
		}
	case "accent":
		for i, o := range accentOpts {
			rows = append(rows, selRow(p, w, i == m.optSel, num(i)+chip(p, o.val)+" "+lipgloss.NewStyle().Foreground(p.Fg).Render(o.name), ""))
		}
	case "density":
		desc := map[string]string{"compact": "fits more on screen", "comfortable": "more room to breathe"}
		for i, o := range densityOpts {
			name := lipgloss.NewStyle().Foreground(p.Fg).Width(13).Render(o.name)
			d := lipgloss.NewStyle().Foreground(p.Dim2).Render(desc[o.val])
			rows = append(rows, selRow(p, w, i == m.optSel, num(i)+name+d, ""))
		}
	case "status":
		for i, o := range statusOpts {
			dot := lipgloss.NewStyle().Foreground(presenceColor(p, o.val)).Render("● ")
			name := lipgloss.NewStyle().Foreground(p.Fg).Width(15).Render(o.label)
			d := lipgloss.NewStyle().Foreground(p.Dim2).Render(o.desc)
			rows = append(rows, selRow(p, w, i == m.optSel, num(i)+dot+name+d, ""))
		}
	}
	return strings.Join(rows, "\n")
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
		glyph, c := "○", p.Dim2
		if i <= m.stepIndex {
			glyph, c = "◉", p.Accent
		}
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render(glyph))
	}
	b.WriteString(lipgloss.NewStyle().Foreground(p.Dim2).Render("]"))
	return b.String()
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
	var cont string
	if m.step() == "keyboard" && !m.kbDone {
		cont = lipgloss.NewStyle().Foreground(p.Dim2).Render("complete the drills to continue")
	} else {
		cont = lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 1).Render(label)
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

func wrapStyled(s lipgloss.Style, text string, w int) string {
	return s.Width(w).Render(text)
}
