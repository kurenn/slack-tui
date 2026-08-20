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

	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/theme"
)

// AppManifest is the Slack app manifest shown on the app-setup screen, injected
// by package main (go:embed can only reach files inside its own package tree,
// and the manifest is a repo-root file the README links to). Empty in tests and
// --dump-ob renders, which simply offer nothing to copy.
var AppManifest string

// Phases of the flow.
const (
	phaseBoot     = "boot"
	phaseAuth     = "auth"
	phaseOAuth    = "oauth"
	phaseAppSetup = "appsetup"
	phaseToken    = "token"
	phaseIdentity = "identity"
	phaseWizard   = "wizard"
	phaseLaunch   = "launch"
)

// Wizard steps. Colour steps are dropped when the desktop already decides the
// palette — see wizStepsFor.
var allWizSteps = []string{"theme", "accent", "density", "keyboard", "status"}

// wizStepsFor omits the theme/accent pickers when slack-tui is following the
// Omarchy theme. Asking someone to choose colours that are then overridden by
// their desktop is a question with no answer, so the steps go away rather than
// becoming inert.
func wizStepsFor(followsDesktop bool) []string {
	if !followsDesktop {
		return allWizSteps
	}
	out := make([]string, 0, len(allWizSteps))
	for _, s := range allWizSteps {
		if s == "theme" || s == "accent" {
			continue
		}
		out = append(out, s)
	}
	return out
}

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

	handle     textinput.Model
	clientID   textinput.Model // Slack app client ID, for the in-flow app setup
	token      textinput.Model // user token (xoxp)
	appToken   textinput.Model // app-level token (xapp) — Socket Mode
	botToken   textinput.Model // bot token (xoxb) — Socket Mode
	tokenField int             // which token input is focused (0=user,1=app,2=bot)

	authSel      int
	provider     string
	tokenNote    string // why we landed on the paste-a-token screen, when we redirected here
	appSetupNote string // feedback on the app-setup screen (copied / opened / bad id)
	teamName     string // real workspace from OAuth; empty for the demo/mock path

	oauthRunning bool   // real browser OAuth in flight
	oauthErr     string // last OAuth failure, shown on the oauth screen

	steps     []string // wizard steps for this run (colour steps may be omitted)
	signedIn  bool     // a usable token already exists — skip the auth screen
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
	// On a desktop that publishes a palette, follow it outright rather than
	// honouring a stored theme — config.Load() always returns a concrete theme
	// name ("charcoal" by default), so deferring to it here would mean the
	// desktop never won.
	followsDesktop := theme.OmarchyAvailable()
	themeName := orDefault(prefs.Theme, "charcoal")
	if followsDesktop {
		themeName = theme.OmarchyName
	}
	m := Model{
		phase:     phaseBoot,
		bootSpeed: "normal",
		steps:     wizStepsFor(followsDesktop),
		themeName: themeName,
		accent:    orDefault(prefs.Accent, "auto"),
		density:   orDefault(prefs.Density, "comfortable"),
		status:    orDefault(prefs.Status, "online"),
		handle:    mk("handle"),
		clientID:  mk("clientid"),
		token:     mk("token"),
		appToken:  mk("app"),
		botToken:  mk("bot"),
		trainer:   newTrainer(),
	}
	// `slack-tui setup` and `slack-tui login` sign in and save tokens without
	// touching prefs, so the first launch afterwards still lands here. Asking
	// someone to authenticate again, minutes after they just did, is the flow
	// contradicting itself — skip straight past auth and keep the preferences.
	if toks, err := config.LoadTokens(); err == nil && toks.Resolve().User != "" {
		m.signedIn = true
		m.provider = "slack"
		if _, active, err := config.LoadWorkspaces(); err == nil && active != "" {
			m.teamName = active
		}
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
		if m.signedIn {
			m.phase = phaseIdentity
			return m, m.handle.Focus()
		}
		m.phase = phaseAuth
		return m, nil
	case advanceDrillMsg:
		return m.advanceDrill()
	case oauthDoneMsg:
		m.oauthRunning = false
		if msg.err != nil {
			m.oauthErr = msg.err.Error()
			return m, nil
		}
		m.teamName = msg.team.Name
		_ = config.SaveWorkspace(config.Workspace{
			Name:   workspaceName(msg.team.Name),
			TeamID: msg.team.ID,
			Tokens: msg.toks,
		}) // SaveWorkspace keeps any stored app (xapp) token for this team
		m.phase = phaseIdentity
		return m, m.handle.Focus()
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

// workspaceName slugifies a team name for the workspace list / --workspace flag.
func workspaceName(team string) string {
	s := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(team), " ", "-"))
	if s == "" {
		return "default"
	}
	return s
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
			if m.oauthRunning { // real flow: wait for the browser callback
				return m, nil
			}
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
	// Persist pasted tokens so the app connects to real Slack on launch.
	if tok := strings.TrimSpace(m.token.Value()); tok != "" {
		saved, _ := config.LoadTokens()
		saved.User = tok
		if a := strings.TrimSpace(m.appToken.Value()); a != "" {
			saved.App = a
		}
		if b := strings.TrimSpace(m.botToken.Value()); b != "" {
			saved.Bot = b
		}
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
