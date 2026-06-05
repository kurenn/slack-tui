// Package root is the top-level Bubble Tea model. It decides between the
// onboarding flow and the main app based on saved prefs, and swaps onboarding
// out for the app when onboarding finishes (the prefs.json hand-off seam).
package root

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/abrahamkuri/slack-tui/internal/app"
	"github.com/abrahamkuri/slack-tui/internal/config"
	"github.com/abrahamkuri/slack-tui/internal/onboarding"
)

type mode int

const (
	modeOnboarding mode = iota
	modeApp
)

// Model wraps either the onboarding model or the app model.
type Model struct {
	mode mode
	ob   onboarding.Model
	app  app.Model

	width, height int
}

// New builds the root, routing to onboarding on first run.
func New() Model {
	_, onboarded := config.Load()
	if onboarded {
		return Model{mode: modeApp, app: app.New()}
	}
	return Model{mode: modeOnboarding, ob: onboarding.New()}
}

func (m Model) Init() tea.Cmd {
	if m.mode == modeOnboarding {
		return m.ob.Init()
	}
	return m.app.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
	}

	if fin, ok := msg.(onboarding.FinishedMsg); ok {
		_ = fin
		next := app.New() // re-reads the prefs onboarding just saved
		next = app.WithSize(next, m.width, m.height)
		m.app = next
		m.mode = modeApp
		return m, m.app.Init()
	}

	switch m.mode {
	case modeOnboarding:
		var cmd tea.Cmd
		m.ob, cmd = m.ob.Update(msg)
		return m, cmd
	default:
		next, cmd := m.app.Update(msg)
		m.app = next.(app.Model)
		return m, cmd
	}
}

func (m Model) View() string {
	if m.mode == modeOnboarding {
		return m.ob.View()
	}
	return m.app.View()
}
