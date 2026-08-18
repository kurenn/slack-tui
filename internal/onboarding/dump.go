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

// KeyC feeds a key and returns the resulting model plus the command Update
// emitted (test helper for exercising the real cmd path).
func KeyC(m Model, key string) (Model, tea.Cmd) {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+k":
		msg = tea.KeyMsg{Type: tea.KeyCtrlK}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	return m.Update(msg)
}

// Pump runs a command and feeds its message back through Update (test helper;
// blocks for tea.Tick delays).
func Pump(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	next, _ := m.Update(cmd())
	return next
}

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
	case phaseBoot:
		m.boot.fastForward()
	case phaseOAuth:
		m.oauth = newTypewriter(oauthLines("slack"))
		m.oauth.fastForward()
	case phaseAppSetup:
		m.phase = phaseAppSetup
	case phaseWizard:
		m.phase = phaseWizard
		// "keyboard:N" jumps the trainer to drill N (prior drills marked done).
		drillTarget := ""
		if i := indexByte(step, ':'); i >= 0 {
			step, drillTarget = step[:i], step[i+1:]
		}
		for i, s := range wizSteps {
			if s == step {
				m.stepIndex = i
			}
		}
		if drillTarget != "" {
			n := int(drillTarget[0] - '0')
			m.trainer.drill = n
			for i := 0; i < n; i++ {
				m.trainer.done[i] = true
			}
		}
		m = m.syncOpt()
		return m
	}
	m.phase = phase
	return m
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
