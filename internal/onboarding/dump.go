package onboarding

import (
	tea "github.com/charmbracelet/bubbletea"
)

// WithSize sizes the model (test/dev helper).
func WithSize(m Model, w, h int) Model { m.width, m.height = w, h; return m }

// Dump renders one frame at w×h.
func Dump(m Model, w, h int) string { m.width, m.height = w, h; return m.View() }

// Key feeds one key string through Update (test/dev helper).
func Key(m Model, key string) Model {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+k":
		msg = tea.KeyMsg{Type: tea.KeyCtrlK}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, _ := m.Update(msg)
	return next
}

// Tick advances typewriter animations one step (test/dev helper).
func Tick(m Model) Model { next, _ := m.Update(tickMsg{}); return next }

// Goto jumps straight to a phase (or "wizard:step") for headless rendering.
func Goto(m Model, target string) Model {
	step := ""
	phase := target
	for i, c := range target {
		if c == ':' {
			phase, step = target[:i], target[i+1:]
			break
		}
	}
	m.handle.SetValue("devon")
	switch phase {
	case phaseOAuth:
		m.oauth = newTypewriter(oauthLines("slack"))
		m.oauth.fastForward()
	case phaseWizard:
		m.phase = phaseWizard
		for i, s := range wizSteps {
			if s == step {
				m.stepIndex = i
			}
		}
		m = m.syncOpt()
		return m
	}
	m.phase = phase
	return m
}
