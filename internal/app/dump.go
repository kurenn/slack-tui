package app

import tea "github.com/charmbracelet/bubbletea"

// Dump renders a single frame at the given size — used by the `--dump` dev flag
// and tests for headless verification.
func Dump(m Model, w, h int) string {
	m.width, m.height = w, h
	return m.View()
}

// WithSize returns a copy of the model sized to w×h (test helper).
func WithSize(m Model, w, h int) Model {
	m.width, m.height = w, h
	return m
}

// Key feeds a single key string through Update and returns the new model
// (test helper for exercising the keyboard engine headlessly).
func Key(m Model, key string) Model {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "ctrl+k":
		msg = tea.KeyMsg{Type: tea.KeyCtrlK}
	case "ctrl+n":
		msg = tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+p":
		msg = tea.KeyMsg{Type: tea.KeyCtrlP}
	case "alt+enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter, Alt: true}
	case "comma": // the dump replay splits on commas, so name it
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, _ := m.Update(msg)
	return next.(Model)
}
