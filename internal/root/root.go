// Package root is the top-level Bubble Tea model. It decides between the
// onboarding flow and the main app based on saved prefs, and swaps onboarding
// out for the app when onboarding finishes (the prefs.json hand-off seam).
package root

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/app"
	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/onboarding"
)

type mode int

const (
	modeOnboarding mode = iota
	modeLoading
	modeApp
)

// Model wraps either the onboarding model, a connecting screen, or the app.
type Model struct {
	mode mode
	ob   onboarding.Model
	app  app.Model

	width, height int
}

// New builds the root, routing to onboarding on first run. The app itself is
// constructed inside a tea.Cmd (appReadyMsg) so its initial network load never
// blocks the UI thread.
func New() Model {
	_, onboarded := config.Load()
	if onboarded {
		return Model{mode: modeLoading}
	}
	return Model{mode: modeOnboarding, ob: onboarding.New()}
}

// appReadyMsg delivers the fully-loaded app model.
type appReadyMsg struct{ app app.Model }

// loadApp builds the app off the UI thread — app.New talks to Slack.
func loadApp() tea.Msg { return appReadyMsg{app.New()} }

func (m Model) Init() tea.Cmd {
	if m.mode == modeOnboarding {
		return m.ob.Init()
	}
	return loadApp
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
	}

	if _, ok := msg.(onboarding.FinishedMsg); ok {
		m.mode = modeLoading
		return m, loadApp // re-reads the prefs onboarding just saved
	}
	if ready, ok := msg.(appReadyMsg); ok {
		m.app = app.WithSize(ready.app, m.width, m.height)
		m.mode = modeApp
		return m, m.app.Init()
	}

	switch m.mode {
	case modeOnboarding:
		var cmd tea.Cmd
		m.ob, cmd = m.ob.Update(msg)
		return m, cmd
	case modeLoading:
		if key, ok := msg.(tea.KeyMsg); ok && (key.String() == "ctrl+c" || key.String() == "q") {
			return m, tea.Quit
		}
		return m, nil
	default:
		next, cmd := m.app.Update(msg)
		m.app = next.(app.Model)
		return m, cmd
	}
}

func (m Model) View() string {
	switch m.mode {
	case modeOnboarding:
		return m.ob.View()
	case modeLoading:
		return "\n  connecting to slack…"
	default:
		return m.app.View()
	}
}
