package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kurenn/slack-tui/internal/theme"
)

// confirm is a small y/n modal guarding destructive actions (message delete).

type confirmState struct {
	open   bool
	text   string
	action func(m *Model) tea.Cmd
}

func (m *Model) openConfirm(text string, action func(*Model) tea.Cmd) {
	m.confirm = confirmState{open: true, text: text, action: action}
}

func (m Model) confirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "enter":
		action := m.confirm.action
		m.confirm = confirmState{}
		return m, action(&m)
	case "n", "esc", "q":
		m.confirm = confirmState{}
	}
	return m, nil
}

// overlayConfirm composites the confirm card over the frame.
func (m Model) overlayConfirm(frame string) string {
	p := m.pal
	w := max(36, lipgloss.Width(m.confirm.text)+8)
	if w > m.width-8 {
		w = m.width - 8
	}
	inner := w - 4

	rows := []string{
		theme.FillBg(" "+lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(m.confirm.text), inner, p.Panel),
		theme.FillBg("", inner, p.Panel),
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Dim2).Render(" y confirm · n cancel"), inner, p.Panel),
	}
	bs := lipgloss.NewStyle().Foreground(p.Red)
	box := []string{bs.Render("┌" + strings.Repeat("─", w-2) + "┐")}
	for _, r := range rows {
		box = append(box, bs.Render("│")+" "+r+" "+bs.Render("│"))
	}
	box = append(box, bs.Render("└"+strings.Repeat("─", w-2)+"┘"))

	x := (m.width - w) / 2
	y := (m.height - len(box)) / 2
	if y < 1 {
		y = 1
	}
	return overlay(frame, strings.Join(box, "\n"), x, y)
}
