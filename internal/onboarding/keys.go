package onboarding

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/auth"
	"github.com/kurenn/slack-tui/internal/config"
)

// ── option data ──────────────────────────────────────────────────────────────

type authOpt struct {
	id, key, mark, label, hint string
	primary                    bool
}

var authOpts = []authOpt{
	{id: "slack", key: "1", mark: "#", label: "Sign in with Slack", hint: "oauth", primary: true},
	{id: "sso", key: "2", mark: "⊞", label: "Single sign-on (SSO)", hint: "saml"},
	{id: "token", key: "3", mark: "⊟", label: "Paste an auth token", hint: "xoxp-…"},
	{id: "guest", key: "4", mark: "◇", label: "Continue as guest", hint: "demo"},
}

var themeOpts = []struct{ val, name string }{
	{"charcoal", "Charcoal"}, {"midnight", "Midnight"}, {"phosphor", "Phosphor"},
	{"solarized", "Solarized"}, {"paper", "Paper"},
}

var accentOpts = []struct{ val, name string }{
	{"auto", "Auto"}, {"cyan", "Cyan"}, {"green", "Green"},
	{"purple", "Purple"}, {"orange", "Orange"}, {"magenta", "Magenta"},
}

var densityOpts = []struct{ val, name string }{
	{"compact", "Compact"}, {"comfortable", "Comfortable"},
}

var statusOpts = []struct{ val, dot, label, desc string }{
	{"online", "online", "Active", "available and reachable"},
	{"away", "away", "Away", "idle — replies may be slow"},
	{"dnd", "dnd", "Do not disturb", "mutes mentions & pages"},
}

func (m Model) step() string { return wizSteps[m.stepIndex] }

func optionCount(step string) int {
	switch step {
	case "theme":
		return len(themeOpts)
	case "accent":
		return len(accentOpts)
	case "density":
		return len(densityOpts)
	case "status":
		return len(statusOpts)
	default:
		return 0
	}
}

// ── key routing ──────────────────────────────────────────────────────────────

func (m Model) onKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.phase {
	case phaseBoot:
		m.boot.fastForward()
		return m, tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg { return bootAdvanceMsg{} })
	case phaseOAuth:
		if m.oauthRunning {
			if k == "esc" { // cancel the wait, back to the menu
				m.oauthRunning = false
				m.phase = phaseAuth
			}
			return m, nil
		}
		if m.oauthErr != "" { // dismiss the error → back to the menu
			m.oauthErr = ""
			m.phase = phaseAuth
			return m, nil
		}
		m.phase = phaseIdentity
		return m, m.handle.Focus()
	case phaseAuth:
		return m.authKey(k)
	case phaseToken:
		return m.tokenKey(msg)
	case phaseIdentity:
		return m.identityKey(msg)
	case phaseWizard:
		return m.wizardKey(msg)
	case phaseLaunch:
		if k == "enter" {
			return m.finish()
		}
	}
	return m, nil
}

func (m Model) authKey(k string) (Model, tea.Cmd) {
	switch {
	case k == "j" || k == "down":
		m.authSel = clamp(m.authSel+1, 0, len(authOpts)-1)
	case k == "k" || k == "up":
		m.authSel = clamp(m.authSel-1, 0, len(authOpts)-1)
	case k == "enter":
		return m.chooseAuth(authOpts[m.authSel])
	case len(k) == 1 && k >= "1" && k <= "4":
		n := int(k[0] - '1')
		m.authSel = n
		return m.chooseAuth(authOpts[n])
	}
	return m, nil
}

func (m Model) chooseAuth(opt authOpt) (Model, tea.Cmd) {
	m.provider = opt.id
	switch opt.id {
	case "token":
		m.phase = phaseToken
		return m, m.focusToken(0)
	case "guest":
		m.phase = phaseIdentity
		return m, m.handle.Focus()
	case "slack":
		creds := config.LoadOAuthCreds()
		if !creds.Ready() {
			// No built-in app in this build and none configured — the paste-a-token
			// screen is the only way through, so say so instead of silently
			// swapping the screen out from under the choice just made.
			m.provider = "token"
			m.phase = phaseToken
			m.tokenNote = "No Slack app configured — quit and run `slack-tui setup` to create " +
				"one in ~2 min, or paste a token below."
			return m, m.focusToken(0)
		}
		m.phase = phaseOAuth
		m.oauthRunning = true
		m.oauthErr = ""
		m.oauth = newTypewriter([]tline{
			{text: "[ oauth ] sign in with Slack", class: "accent"},
			{text: "opening the authorization page in your browser…", class: "dim"},
			{text: "  ↳ " + auth.RedirectURIs()[0], class: "fill"},
			{text: "waiting for you to approve access in Slack…", class: "ok"},
		})
		return m, tea.Batch(tick(m.speedMS()), oauthCmd(creds))
	default: // sso (simulated)
		m.phase = phaseOAuth
		m.oauth = newTypewriter(oauthLines(opt.id))
		return m, tick(m.speedMS())
	}
}

type oauthDoneMsg struct {
	toks config.Tokens
	team auth.Team
	err  error
}

func oauthCmd(creds config.OAuthCreds) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		toks, team, err := auth.Login(ctx, creds, nil)
		return oauthDoneMsg{toks: toks, team: team, err: err}
	}
}

func (m Model) tokenKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if len(strings.TrimSpace(m.token.Value())) >= 4 {
			m.tokenInputs()[m.tokenField].Blur()
			m.phase = phaseIdentity
			return m, m.handle.Focus()
		}
		return m, nil // user token required to continue (Tab moves between fields)
	case "tab", "down":
		return m, m.focusToken((m.tokenField + 1) % 3)
	case "shift+tab", "up":
		return m, m.focusToken((m.tokenField + 2) % 3)
	case "esc":
		return m, nil
	}
	var cmd tea.Cmd
	in := m.tokenInputs()[m.tokenField]
	*in, cmd = in.Update(msg)
	return m, cmd
}

// tokenInputs returns the three token inputs in field order.
func (m *Model) tokenInputs() []*textinput.Model {
	return []*textinput.Model{&m.token, &m.appToken, &m.botToken}
}

func (m *Model) focusToken(field int) tea.Cmd {
	for _, in := range m.tokenInputs() {
		in.Blur()
	}
	m.tokenField = field
	return m.tokenInputs()[field].Focus()
}

func (m Model) identityKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.String() == "enter" {
		if strings.TrimSpace(m.handle.Value()) != "" {
			m.phase = phaseWizard
			m.stepIndex = 0
			m = m.syncOpt()
			m.handle.Blur()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.handle, cmd = m.handle.Update(msg)
	return m, cmd
}

func (m Model) wizardKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := msg.String()
	if m.step() == "keyboard" {
		// Esc / h navigate back unless the trainer needs Esc for the active drill.
		if (k == "esc" || k == "h") && !m.trainer.needsEsc() {
			return m.back()
		}
		var cmd tea.Cmd
		m, cmd = m.trainerKey(msg)
		if m.kbDone && k == "enter" {
			return m.next()
		}
		return m, cmd
	}

	switch {
	case k == "j" || k == "down":
		m.optSel = clamp(m.optSel+1, 0, optionCount(m.step())-1)
		m = m.applyOpt(m.optSel)
	case k == "k" || k == "up":
		m.optSel = clamp(m.optSel-1, 0, optionCount(m.step())-1)
		m = m.applyOpt(m.optSel)
	case k == "enter":
		return m.next()
	case k == "esc" || (k == "h" && m.stepIndex > 0):
		return m.back()
	case len(k) == 1 && k >= "1" && k <= "9":
		n := int(k[0] - '1')
		if n < optionCount(m.step()) {
			m.optSel = n
			m = m.applyOpt(n)
		}
	}
	return m, nil
}

func (m Model) next() (Model, tea.Cmd) {
	if m.stepIndex >= len(wizSteps)-1 {
		m.phase = phaseLaunch
		return m, nil
	}
	m.stepIndex++
	m = m.syncOpt()
	return m, nil
}

func (m Model) back() (Model, tea.Cmd) {
	if m.stepIndex > 0 {
		m.stepIndex--
		m = m.syncOpt()
	}
	return m, nil
}

// applyOpt sets the previewed value for the current step from an option index.
func (m Model) applyOpt(idx int) Model {
	switch m.step() {
	case "theme":
		m.themeName = themeOpts[idx].val
	case "accent":
		m.accent = accentOpts[idx].val
	case "density":
		m.density = densityOpts[idx].val
	case "status":
		m.status = statusOpts[idx].val
	}
	return m
}

// syncOpt aligns optSel to the current value when entering a step.
func (m Model) syncOpt() Model {
	find := func(get func(int) string, n int, want string) int {
		for i := 0; i < n; i++ {
			if get(i) == want {
				return i
			}
		}
		return 0
	}
	switch m.step() {
	case "theme":
		m.optSel = find(func(i int) string { return themeOpts[i].val }, len(themeOpts), m.themeName)
	case "accent":
		m.optSel = find(func(i int) string { return accentOpts[i].val }, len(accentOpts), m.accent)
	case "density":
		m.optSel = find(func(i int) string { return densityOpts[i].val }, len(densityOpts), m.density)
	case "status":
		m.optSel = find(func(i int) string { return statusOpts[i].val }, len(statusOpts), m.status)
	}
	return m
}
