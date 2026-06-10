package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kurenn/slack-tui/internal/theme"
)

// Custom status-message prompt (Slack status_text/status_emoji). Open it from the
// command palette ("Set status message"). A leading :emoji: becomes the emoji.

func (m *Model) openStatusText() tea.Cmd {
	m.statusTextOpen = true
	m.statusTextInput.SetValue("")
	return m.statusTextInput.Focus()
}

func (m Model) statusTextKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.statusTextOpen = false
		m.statusTextInput.Blur()
		return m, nil
	case "enter":
		text := m.statusTextInput.Value()
		m.statusTextOpen = false
		m.statusTextInput.Blur()
		return m, m.setStatusTextCmd(text)
	}
	var cmd tea.Cmd
	m.statusTextInput, cmd = m.statusTextInput.Update(msg)
	return m, cmd
}

func (m Model) setStatusTextCmd(text string) tea.Cmd {
	emoji, rest := parseStatus(text)
	src := m.src
	return func() tea.Msg { return presenceMsg{src.SetStatusText(rest, emoji)} }
}

// parseStatus splits a leading :emoji: from the rest of the status text.
func parseStatus(s string) (emoji, text string) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, ":") {
		if i := strings.Index(s[1:], ":"); i >= 0 {
			return s[:i+2], strings.TrimSpace(s[i+2:])
		}
	}
	return "", s
}

// overlayStatusText composites the status-message prompt over the frame.
func (m Model) overlayStatusText(frame string) string {
	p := m.pal
	w := 56
	inner := w - 4
	bs := lipgloss.NewStyle().Foreground(p.Accent)
	m.statusTextInput.Width = inner - 4

	rows := []string{
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(" status message"), inner, p.Panel),
		theme.FillBg("", inner, p.Panel),
		theme.FillBg(" "+lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Render("❯ ")+m.statusTextInput.View(), inner, p.Panel),
		theme.FillBg("", inner, p.Panel),
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Dim2).Render(" ↵ set · esc cancel · :emoji: prefix · empty clears"), inner, p.Panel),
	}
	box := []string{bs.Render("┌" + strings.Repeat("─", w-2) + "┐")}
	for _, r := range rows {
		box = append(box, bs.Render("│")+" "+r+" "+bs.Render("│"))
	}
	box = append(box, bs.Render("└"+strings.Repeat("─", w-2)+"┘"))

	x := (m.width - w) / 2
	y := m.height/2 - len(box)/2
	if y < 1 {
		y = 1
	}
	return overlay(frame, strings.Join(box, "\n"), x, y)
}
