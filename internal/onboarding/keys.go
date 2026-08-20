package onboarding

import (
	"context"
	"strings"
	"time"

	"github.com/atotto/clipboard"

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

// The old "Single sign-on (SSO)" entry was a typewriter animation that
// authenticated nothing and dropped the user in the mock workspace. Harmless in
// a demo, misleading in a tool strangers install — anyone at an SSO shop would
// reach for it first.
var authOpts = []authOpt{
	{id: "slack", key: "1", mark: "#", label: "Sign in with Slack", hint: "oauth", primary: true},
	{id: "token", key: "2", mark: "⊟", label: "Paste an auth token", hint: "xoxp-…"},
	{id: "guest", key: "3", mark: "◇", label: "Continue as guest", hint: "demo"},
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

func (m Model) step() string { return m.steps[m.stepIndex] }

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
	case phaseAppSetup:
		return m.appSetupKey(msg)
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
	case len(k) == 1 && k >= "1" && k <= "9":
		n := int(k[0] - '1')
		if n >= len(authOpts) {
			return m, nil
		}
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
			// No app configured yet. Rather than bouncing to the token screen — or
			// telling someone to quit the program they just launched — set the app
			// up here. With slack-tui not distributed through Slack, this is the
			// ordinary path for a new install, not an error case.
			m.phase = phaseAppSetup
			m.appSetupNote = ""
			return m, m.clientID.Focus()
		}
		return m.startOAuth(creds)
	}
	return m, nil
}

// startOAuth switches to the waiting screen and kicks off the browser flow.
func (m Model) startOAuth(creds config.OAuthCreds) (Model, tea.Cmd) {
	m.phase = phaseOAuth
	m.oauthRunning = true
	m.oauthErr = ""
	m.appSetupNote = ""
	m.oauth = newTypewriter([]tline{
		{text: "[ oauth ] sign in with Slack", class: "accent"},
		{text: "opening the authorization page in your browser…", class: "dim"},
		{text: "  ↳ " + auth.RedirectURIs()[0], class: "fill"},
		{text: "waiting for you to approve access in Slack…", class: "ok"},
	})
	return m, tea.Batch(tick(m.speedMS()), oauthCmd(creds))
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
	if m.stepIndex >= len(m.steps)-1 {
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

// appSetupKey drives the in-flow Slack app setup: copy the manifest, open the
// browser, paste the client ID. The two side-effecting actions are on explicit
// keys rather than fired on entry, so rendering this screen (tests, --dump-ob)
// never touches the user's clipboard or launches a browser.
func (m Model) appSetupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.phase = phaseAuth
		m.appSetupNote = ""
		return m, nil
	case "ctrl+y":
		if AppManifest == "" {
			m.appSetupNote = "manifest unavailable in this build — copy it from the repo"
			return m, nil
		}
		if err := clipboard.WriteAll(AppManifest); err != nil {
			m.appSetupNote = "couldn't reach the clipboard — copy slack-app-manifest.json from the repo"
			return m, nil
		}
		m.appSetupNote = "✓ manifest copied — paste it into \"From an app manifest\""
		return m, nil
	case "ctrl+o":
		_ = auth.OpenBrowser("https://api.slack.com/apps")
		m.appSetupNote = "opened api.slack.com/apps in your browser"
		return m, nil
	case "enter":
		id := strings.TrimSpace(m.clientID.Value())
		if id == "" {
			return m, nil
		}
		if !config.ValidClientID(id) {
			if strings.HasPrefix(id, "A") {
				m.appSetupNote = "that's the App ID — you want the Client ID just below it"
			} else {
				m.appSetupNote = "doesn't look like a Client ID (e.g. 1234567890.9876543210)"
			}
			return m, nil
		}
		if err := config.SaveOAuthCreds(config.OAuthCreds{ClientID: id}); err != nil {
			m.appSetupNote = "couldn't save: " + err.Error()
			return m, nil
		}
		m.clientID.Blur()
		return m.startOAuth(config.OAuthCreds{ClientID: id})
	}
	var cmd tea.Cmd
	m.clientID, cmd = m.clientID.Update(msg)
	return m, cmd
}
