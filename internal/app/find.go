package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kurenn/slack-tui/internal/theme"
)

// Local find (`/`): search within the loaded history of the active channel.
// Enter jumps to the next match; n/N repeat forward/backward.

func (m *Model) openFind() tea.Cmd {
	m.findOpen = true
	m.findInput.SetValue(m.searchQuery)
	return m.findInput.Focus()
}

func (m Model) findKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, m.quit()
	case "esc":
		m.findOpen = false
		m.findInput.Blur()
		return m, nil
	case "enter":
		m.findOpen = false
		m.findInput.Blur()
		m.searchQuery = strings.TrimSpace(m.findInput.Value())
		if m.searchQuery == "" {
			return m, nil
		}
		m.focus = focusMessages
		return m, m.jumpFind(1)
	}
	var cmd tea.Cmd
	m.findInput, cmd = m.findInput.Update(msg)
	return m, cmd
}

// jumpFind moves the selection to the next/previous message matching the query
// (case-insensitive, text or author), wrapping around.
func (m *Model) jumpFind(dir int) tea.Cmd {
	msgs := m.curMsgs()
	q := strings.ToLower(m.searchQuery)
	if q == "" || len(msgs) == 0 {
		return nil
	}
	n := len(msgs)
	for k := 1; k <= n; k++ {
		i := ((m.msgSel+dir*k)%n + n) % n
		hay := strings.ToLower(msgs[i].Text + " " + m.ws.Users[msgs[i].UserID].Name)
		if strings.Contains(hay, q) {
			m.msgSel, m.msgExtra = i, 0
			return nil
		}
	}
	return m.flash(fmt.Errorf("no match for %q", m.searchQuery))
}

// overlayFind composites the find prompt over the frame.
func (m Model) overlayFind(frame string) string {
	p := m.pal
	w := 48
	inner := w - 4
	bs := lipgloss.NewStyle().Foreground(p.Accent)
	m.findInput.Width = inner - 4

	rows := []string{
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(" find in channel"), inner, p.Panel),
		theme.FillBg(" "+lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Render("/ ")+m.findInput.View(), inner, p.Panel),
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Dim2).Render(" ↵ jump · n/N next/prev · esc cancel"), inner, p.Panel),
	}
	box := []string{bs.Render("┌" + strings.Repeat("─", w-2) + "┐")}
	for _, r := range rows {
		box = append(box, bs.Render("│")+" "+r+" "+bs.Render("│"))
	}
	box = append(box, bs.Render("└"+strings.Repeat("─", w-2)+"┘"))

	x := (m.width - w) / 2
	y := m.height / 3
	if y < 1 {
		y = 1
	}
	return overlay(frame, strings.Join(box, "\n"), x, y)
}
