// Package onboarding is the first-run flow: a typewriter boot sequence, auth,
// a 5-step setup wizard with an interactive keyboard trainer, and a launch
// hand-off. It mirrors onboarding.jsx. On completion it persists prefs and emits
// FinishedMsg so the root can swap in the main app. The whole screen re-themes
// live as the user previews themes/accents.
package onboarding

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/abrahamkuri/slack-tui/internal/config"
	"github.com/abrahamkuri/slack-tui/internal/theme"
)

// Phases of the flow.
const (
	phaseBoot     = "boot"
	phaseAuth     = "auth"
	phaseOAuth    = "oauth"
	phaseToken    = "token"
	phaseIdentity = "identity"
	phaseWizard   = "wizard"
	phaseLaunch   = "launch"
)

// Wizard steps.
var wizSteps = []string{"theme", "accent", "density", "keyboard", "status"}

// FinishedMsg is emitted when the user enters the workspace; it carries the
// prefs to persist and hand to the app.
type FinishedMsg struct{ Prefs config.Prefs }

// tickMsg drives typewriter animations (boot + oauth).
type tickMsg time.Time

// bootAdvanceMsg fires after the post-boot pause to move into auth.
type bootAdvanceMsg struct{}

// advanceDrillMsg is sent ~900ms after a drill is cleared to auto-advance.
type advanceDrillMsg struct{}

// Model is the onboarding state.
type Model struct {
	phase string

	// live-preview tweaks
	bootSpeed string // slow | normal | fast
	themeName string
	accent    string
	density   string
	status    string

	handle textinput.Model
	token  textinput.Model

	authSel  int
	provider string

	stepIndex int
	optSel    int
	kbDone    bool

	boot    typewriter
	oauth   typewriter
	trainer trainerState

	width, height int
}

// New builds the onboarding model seeded from any existing prefs.
func New() Model {
	prefs, _ := config.Load()
	mk := func(ph string) textinput.Model {
		ti := textinput.New()
		ti.Prompt = ""
		return ti
	}
	m := Model{
		phase:     phaseBoot,
		bootSpeed: "normal",
		themeName: orDefault(prefs.Theme, "charcoal"),
		accent:    orDefault(prefs.Accent, "auto"),
		density:   orDefault(prefs.Density, "comfortable"),
		status:    orDefault(prefs.Status, "online"),
		handle:    mk("handle"),
		token:     mk("token"),
		trainer:   newTrainer(),
	}
	m.boot = newTypewriter(bootLines())
	return m
}

func (m Model) pal() theme.Palette { return theme.Resolve(m.themeName, m.accent) }

func (m Model) Init() tea.Cmd { return tick(m.speedMS()) }

func (m Model) speedMS() int {
	switch m.bootSpeed {
	case "slow":
		return 26
	case "fast":
		return 4
	default:
		return 12
	}
}

func tick(ms int) tea.Cmd {
	return tea.Tick(time.Duration(ms)*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// SetSize is called by the root on resize.
func (m Model) SetSize(w, h int) Model { m.width, m.height = w, h; return m }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		return m.onTick()
	case bootAdvanceMsg:
		m.phase = phaseAuth
		return m, nil
	case advanceDrillMsg:
		return m.advanceDrill()
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) onTick() (Model, tea.Cmd) {
	switch m.phase {
	case phaseBoot:
		m.boot.step()
		if m.boot.done {
			return m, tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg { return bootAdvanceMsg{} })
		}
		return m, tick(m.speedMS())
	case phaseOAuth:
		m.oauth.step()
		if m.oauth.done {
			m.phase = phaseIdentity
			return m, m.handle.Focus()
		}
		return m, tick(m.speedMS())
	}
	return m, nil
}

// persist writes prefs and returns the finishing command.
func (m Model) finish() (Model, tea.Cmd) {
	prefs := config.Prefs{
		Handle:    orDefault(m.handle.Value(), "you"),
		Theme:     m.themeName,
		Accent:    m.accent,
		Font:      "JetBrains Mono",
		Density:   m.density,
		Status:    m.status,
		Onboarded: true,
		TS:        time.Now().Unix(),
	}
	_ = config.Save(prefs)
	// Persist a pasted user token so the app connects to real Slack on launch.
	if tok := strings.TrimSpace(m.token.Value()); tok != "" {
		saved, _ := config.LoadTokens()
		saved.User = tok
		_ = config.SaveTokens(saved)
	}
	return m, func() tea.Msg { return FinishedMsg{Prefs: prefs} }
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
