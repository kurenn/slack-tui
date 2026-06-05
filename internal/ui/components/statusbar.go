package components

import (
	"strings"

	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

// Hint is a single key-hint shown in the status bar.
type Hint struct{ Key, Desc string }

// H is a terse Hint constructor.
func H(key, desc string) Hint { return Hint{Key: key, Desc: desc} }

// StatusBar renders the bottom bar: mode block, location, focus name, and the
// focus/mode-dependent key hints (hidden when showHints is false).
func StatusBar(p theme.Palette, insert bool, location, focus string, hints []Hint, showHints bool, width int) string {
	bg := lipgloss.NewStyle().Background(p.StatusBg)

	var mode string
	if insert {
		mode = lipgloss.NewStyle().Background(p.Green).Foreground(p.Bg).Bold(true).Padding(0, 1).Render("-- INSERT --")
	} else {
		mode = lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 1).Render("NORMAL")
	}
	loc := bg.Foreground(p.Fg).Padding(0, 1).Render(location)
	foc := bg.Foreground(p.Dim).Padding(0, 1).Render(focus)

	var hb strings.Builder
	if showHints {
		for _, h := range hints {
			hb.WriteString(bg.Foreground(p.Fg).Bold(true).Render(h.Key))
			hb.WriteString(bg.Foreground(p.Dim).Render(" " + h.Desc + "   "))
		}
	}
	hintStr := hb.String()

	left := mode + loc + foc
	gap := width - lipgloss.Width(left) - lipgloss.Width(hintStr)
	if gap < 1 {
		gap = 1
	}
	return left + bg.Render(strings.Repeat(" ", gap)) + hintStr
}
