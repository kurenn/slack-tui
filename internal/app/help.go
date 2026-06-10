package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kurenn/slack-tui/internal/theme"
)

// Help overlay — the full keymap cheat sheet (`?` to open).

type helpRow struct{ keys, desc string }

var helpRows = []helpRow{
	{"j / k · ↓ / ↑", "move · scroll tall messages"},
	{"gg / G", "jump to top / bottom"},
	{"ctrl+d / ctrl+u", "half-page scroll"},
	{"tab · h / l · ← / →", "switch panes"},
	{"enter / t", "open thread"},
	{"i", "write message"},
	{"r", "reply in thread"},
	{"alt+enter", "newline in the composer"},
	{"a", "react to message"},
	{"e", "edit your message"},
	{"dd", "delete your message"},
	{"o", "open message links"},
	{"/ · n / N", "find in channel · next/prev"},
	{"s", "search the workspace"},
	{"y", "yank message to clipboard"},
	{"] / [", "next / prev unread"},
	{"ctrl+k", "command palette"},
	{",", "settings"},
	{"ctrl+r", "refresh"},
	{"esc", "close · dismiss"},
	{"q", "quit"},
}

func (m Model) helpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "?", "enter":
		m.helpOpen = false
	}
	return m, nil
}

// overlayHelp composites the keymap card over the frame.
func (m Model) overlayHelp(frame string) string {
	p := m.pal
	w := 52
	inner := w - 4

	keyStyle := lipgloss.NewStyle().Foreground(p.Accent).Width(17)
	descStyle := lipgloss.NewStyle().Foreground(p.Fg)

	body := []string{
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(" keys"), inner, p.Panel),
		theme.FillBg("", inner, p.Panel),
	}
	for _, r := range helpRows {
		line := " " + keyStyle.Render(r.keys) + descStyle.Render(r.desc)
		body = append(body, theme.FillBg(line, inner, p.Panel))
	}
	body = append(body,
		theme.FillBg("", inner, p.Panel),
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Dim2).Render(" esc close"), inner, p.Panel),
	)

	bs := lipgloss.NewStyle().Foreground(p.Accent)
	box := []string{bs.Render("┌" + strings.Repeat("─", w-2) + "┐")}
	for _, l := range body {
		box = append(box, bs.Render("│")+" "+l+" "+bs.Render("│"))
	}
	box = append(box, bs.Render("└"+strings.Repeat("─", w-2)+"┘"))

	x := (m.width - w) / 2
	y := (m.height - len(box)) / 2
	if y < 1 {
		y = 1
	}
	return overlay(frame, strings.Join(box, "\n"), x, y)
}
