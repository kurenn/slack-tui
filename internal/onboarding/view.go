package onboarding

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/kurenn/slack-tui/internal/theme"
)

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	p := m.pal()
	if m.width < 54 || m.height < 18 {
		return lipgloss.NewStyle().Foreground(p.Dim).Render("onboarding needs at least 54×18 — resize the terminal.")
	}
	stageH := m.height - 2
	block := m.stage(p)
	var stage string
	if m.phase == phaseWizard || m.phase == phaseLaunch {
		stage = lipgloss.Place(m.width, stageH, lipgloss.Center, lipgloss.Center, block)
	} else {
		stage = m.bootPlace(block, stageH) // boot-family: full-bleed, left/top
	}
	return strings.Join([]string{m.titlebar(p), stage, m.statusbar(p)}, "\n")
}

// bootPlace centers a boot-family block as a single unit — preserving its
// internal left alignment — both horizontally and vertically in the stage.
func (m Model) bootPlace(block string, stageH int) string {
	lines := strings.Split(block, "\n")
	blockW := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > blockW {
			blockW = w
		}
	}
	left := (m.width - blockW) / 2
	if left < 0 {
		left = 0
	}
	top := (stageH - len(lines)) / 2
	if top < 0 {
		top = 0
	}
	pad := strings.Repeat(" ", left)
	out := make([]string, 0, stageH)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	for _, l := range lines {
		out = append(out, pad+l)
	}
	for len(out) < stageH {
		out = append(out, "")
	}
	return strings.Join(out[:stageH], "\n")
}

// bootContentW is the text width for boot-family screens.
func (m Model) bootContentW() int {
	w := m.width - 12
	if w > 74 {
		w = 74
	}
	if w < 40 {
		w = m.width - 8
	}
	return w
}

// bootScreen prepends the shared banner to a boot-family screen's content.
func bootScreen(p theme.Palette, content string) string {
	return banner(p) + "\n\n" + content
}

// panelDims returns the centered card's outer width/height. The wizard is wider
// to fit the card grids; the other phases use a slimmer column.
func (m Model) panelDims() (int, int) {
	maxW, maxH := 66, 17
	if m.phase == phaseWizard {
		maxW, maxH = 104, 24
	}
	pw := m.width - 8
	if pw > maxW {
		pw = maxW
	}
	if pw < 50 {
		pw = m.width - 4
	}
	ph := m.height - 4
	if ph > maxH {
		ph = maxH
	}
	return pw, ph
}

// contentW is the usable text width inside the card (border + 2-space pad each side).
func (m Model) contentW() int {
	pw, _ := m.panelDims()
	return pw - 6
}

// frame renders a fixed-size centered card with a thin dim border and a flat
// (transparent) interior — matching the design. Title is accent; an optional
// right label (e.g. the step rail) sits on the top rule. Body is top-aligned
// under a blank line; footer is pinned to the bottom row.
func (m Model) frame(p theme.Palette, title, right, body, footer string) string {
	pw, ph := m.panelDims()
	innerW, innerH := pw-2, ph-2
	bs := lipgloss.NewStyle().Foreground(p.Dim2)
	ts := lipgloss.NewStyle().Foreground(p.Accent).Bold(true)

	// top rule: ┌─ title ─────── right ─┐
	fill := innerW - (3 + lipgloss.Width(title))
	if right != "" {
		fill -= 3 + lipgloss.Width(right)
	}
	if fill < 0 {
		fill = 0
	}
	top := bs.Render("┌─ ") + ts.Render(title) + bs.Render(" ") + bs.Render(strings.Repeat("─", fill))
	if right != "" {
		top += bs.Render(" ") + right + bs.Render(" ─")
	}
	top += bs.Render("┐")

	inner := make([]string, innerH)
	row := 1 // one blank line of top padding
	for _, l := range strings.Split(body, "\n") {
		if row >= innerH-1 {
			break
		}
		inner[row] = "  " + l
		row++
	}
	if footer != "" {
		inner[innerH-1] = "  " + footer
	}

	var b strings.Builder
	b.WriteString(top + "\n")
	for _, l := range inner {
		b.WriteString(bs.Render("│") + padPlain(l, innerW) + bs.Render("│") + "\n")
	}
	b.WriteString(bs.Render("└" + strings.Repeat("─", innerW) + "┘"))
	return b.String()
}

// padPlain pads s to width w with plain (no-background) spaces, truncating if needed.
func padPlain(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return ansi.Truncate(s, w, "")
}

func (m Model) stage(p theme.Palette) string {
	switch m.phase {
	case phaseBoot:
		return bootScreen(p, m.boot.render(p))
	case phaseOAuth:
		body := m.oauth.render(p)
		switch {
		case m.oauthErr != "":
			body = lipgloss.JoinVertical(lipgloss.Left, body, "",
				lipgloss.NewStyle().Foreground(p.Red).Render("✗ sign-in failed: "+m.oauthErr),
				lipgloss.NewStyle().Foreground(p.Dim2).Render("press any key to go back · or choose “Paste a token” instead"))
		case m.oauthRunning:
			body = lipgloss.JoinVertical(lipgloss.Left, body, "",
				lipgloss.NewStyle().Foreground(p.Dim2).Render("approve access in your browser · esc to cancel"))
		}
		return bootScreen(p, body)
	case phaseAuth:
		return bootScreen(p, m.viewAuth(p))
	case phaseAppSetup:
		return bootScreen(p, m.viewAppSetup(p))
	case phaseToken:
		return bootScreen(p, m.viewTokenForm(p))
	case phaseIdentity:
		var lines []tline
		if m.provider == "guest" {
			lines = []tline{{text: "guest session — pick a display handle for this demo.", class: "dim"}}
		} else {
			lines = []tline{
				{text: "authenticated ✓  ·  workspace @" + m.workspaceLabel(), class: "ok"},
				{text: "choose how teammates will see you.", class: "dim"},
			}
		}
		return bootScreen(p, m.viewLogin(p, "display handle:", m.handle.View(), lines, "set your handle, then press ↵ enter to continue"))
	case phaseWizard:
		var body string
		if m.step() == "keyboard" {
			body = lipgloss.JoinVertical(lipgloss.Left, m.wizHeading(p), "", m.viewTrainer(p, m.contentW()))
		} else {
			body = m.viewWizardBody(p)
		}
		return m.frame(p, "setup", m.stepRail(p), body, m.viewFooter(p, m.contentW()))
	case phaseLaunch:
		return m.viewLaunch(p)
	}
	return ""
}

// ── chrome ───────────────────────────────────────────────────────────────────

// workspaceLabel is the real team name once OAuth has told us one, and the demo
// workspace otherwise — a live sign-in used to be greeted by the mock's name.
func (m Model) workspaceLabel() string {
	if m.teamName != "" {
		return m.teamName
	}
	return "monospace-labs"
}

func (m Model) titlebar(p theme.Palette) string {
	bg := lipgloss.NewStyle().Background(p.TitlebarBg)
	dim := bg.Foreground(p.Dim)
	title := dim.Render("slack-tui — ") + bg.Foreground(p.Fg).Bold(true).Render("onboarding")
	right := m.workspaceLabel()
	if h := m.handle.Value(); h != "" {
		right = "@" + h
	}
	r := dim.Render(right)
	left := "  " + title
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(r) - 1
	if gap < 1 {
		gap = 1
	}
	return left + bg.Render(strings.Repeat(" ", gap)) + r + bg.Render(" ")
}

func (m Model) statusbar(p theme.Palette) string {
	bg := lipgloss.NewStyle().Background(p.StatusBg)
	label := map[string]string{
		phaseBoot: "BOOT", phaseAuth: "AUTH", phaseOAuth: "OAUTH", phaseAppSetup: "APP", phaseToken: "TOKEN",
		phaseIdentity: "IDENTITY", phaseWizard: "SETUP", phaseLaunch: "READY",
	}[m.phase]
	mode := lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 1).Render(label)

	loc := "@" + m.workspaceLabel()
	if m.phase == phaseWizard {
		loc = fmt.Sprintf("step %d / %d · %s", m.stepIndex+1, len(wizSteps), m.step())
	} else if h := m.handle.Value(); h != "" && m.phase != phaseAuth && m.phase != phaseOAuth {
		loc = "@" + h
	}
	locCell := bg.Foreground(p.Fg).Padding(0, 1).Render(loc)

	var hb strings.Builder
	for _, h := range m.hints() {
		hb.WriteString(bg.Foreground(p.Fg).Bold(true).Render(h[0]))
		hb.WriteString(bg.Foreground(p.Dim).Render(" " + h[1] + "   "))
	}
	left := mode + locCell
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(hb.String())
	if gap < 1 {
		gap = 1
	}
	return left + bg.Render(strings.Repeat(" ", gap)) + hb.String()
}

func (m Model) hints() [][2]string {
	switch m.phase {
	case phaseBoot, phaseOAuth:
		return [][2]string{{"any key", "skip"}}
	case phaseAuth:
		return [][2]string{{"j/k", "choose"}, {"1-4", "jump"}, {"↵", "authenticate"}}
	case phaseAppSetup:
		return [][2]string{{"^y", "copy manifest"}, {"^o", "open slack"}, {"↵", "sign in"}, {"esc", "back"}}
	case phaseToken, phaseIdentity:
		return [][2]string{{"↵", "continue"}}
	case phaseLaunch:
		return [][2]string{{"↵", "enter workspace"}}
	default:
		if m.step() == "keyboard" {
			h := [][2]string{{"follow", "the drills"}}
			if m.kbDone {
				h = append(h, [2]string{"↵", "continue"})
			}
			return h
		}
		h := [][2]string{{"j/k", "choose"}, {"↵", "next"}}
		if m.stepIndex > 0 {
			h = append(h, [2]string{"esc", "back"})
		}
		return h
	}
}

// ── auth (full-bleed bordered rows) ──────────────────────────────────────────

func (m Model) viewAuth(p theme.Palette) string {
	w := m.bootContentW()
	ready := bootColor(p, "ok").Render("session ready.")
	prompt := lipgloss.NewStyle().Foreground(p.Dim).Render("connect a workspace to continue, or pick another method.")

	var rows []string
	for i, o := range authOpts {
		if i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, m.authRow(p, o, i == m.authSel, w))
	}
	return lipgloss.JoinVertical(lipgloss.Left, ready, "", prompt, "", lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m Model) authRow(p theme.Palette, o authOpt, sel bool, w int) string {
	contentW := w - 4 // border (2) + padding (2)

	keyColor := p.Dim2
	border := p.Border
	if o.primary {
		border = p.Dim2
	}
	if sel {
		keyColor, border = p.Accent, p.Accent
	}
	key := lipgloss.NewStyle().Foreground(keyColor).Render("[" + o.key + "]")
	mark := lipgloss.NewStyle().Foreground(authColor(p, o.id)).Render(o.mark)
	labelColor := p.Dim
	if sel {
		labelColor = p.Fg
	}
	label := lipgloss.NewStyle().Foreground(labelColor).Bold(true).Render(o.label)
	hint := lipgloss.NewStyle().Foreground(p.Dim2).Render(o.hint)
	enter := " "
	if sel {
		enter = lipgloss.NewStyle().Foreground(p.Accent).Render("↵")
	}

	left := key + "  " + mark + "  " + label
	right := hint + "  " + enter
	gap := contentW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right

	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).
		Width(w-2).Padding(0, 1)
	if sel {
		box = box.Background(p.SelBg)
	}
	return box.Render(line)
}

func authColor(p theme.Palette, id string) lipgloss.Color {
	switch id {
	case "slack":
		return p.Purple
	case "sso":
		return p.Blue
	case "token":
		return p.Green
	default:
		return p.Dim
	}
}

// ── login (token / identity) ─────────────────────────────────────────────────

// viewTokenForm renders the three token inputs (user required, app/bot optional).
func (m Model) viewTokenForm(p theme.Palette) string {
	head := []string{
		m.tlineRender(p, tline{text: "authenticate with access tokens", class: "fg"}),
		m.tlineRender(p, tline{text: "user token is required; app + bot tokens enable live channel unread (optional).", class: "dim"}),
	}
	if m.tokenNote != "" {
		head = append(head, m.tlineRender(p, tline{text: m.tokenNote, class: "warn"}))
	}
	labels := []string{"user token (xoxp):", "app token  (xapp):", "bot token  (xoxb):"}
	views := []string{m.token.View(), m.appToken.View(), m.botToken.View()}
	var rows []string
	for i, label := range labels {
		labelColor := p.Dim
		if i == m.tokenField {
			labelColor = p.Green
		}
		row := lipgloss.NewStyle().Foreground(labelColor).Render(label+" ") +
			lipgloss.NewStyle().Foreground(p.Fg).Render(views[i])
		if i == m.tokenField {
			row += lipgloss.NewStyle().Foreground(p.Accent).Render("▋")
		}
		rows = append(rows, row)
	}
	hint := lipgloss.NewStyle().Foreground(p.Dim2).Render("tab switches fields · ↵ enter continues · stays on this machine")
	return lipgloss.JoinVertical(lipgloss.Left, strings.Join(head, "\n"), "", strings.Join(rows, "\n"), "", hint)
}

// viewAppSetup walks the user through creating their own Slack app without
// leaving the program. slack-tui isn't distributed through Slack, so this is the
// ordinary path for a new install rather than a fallback.
func (m Model) viewAppSetup(p theme.Palette) string {
	head := []string{
		m.tlineRender(p, tline{text: "connect your Slack workspace", class: "fg"}),
		m.tlineRender(p, tline{text: "slack-tui signs in through an app you own — it takes about two minutes,", class: "dim"}),
		m.tlineRender(p, tline{text: "and there's no client secret involved.", class: "dim"}),
		"",
		m.tlineRender(p, tline{text: "  1. ^y copies the app manifest to your clipboard", class: "fill"}),
		m.tlineRender(p, tline{text: "  2. ^o opens api.slack.com/apps — Create New App → From an app manifest", class: "fill"}),
		m.tlineRender(p, tline{text: "  3. paste it, create the app, then copy the Client ID below", class: "fill"}),
	}
	if m.appSetupNote != "" {
		head = append(head, "", m.tlineRender(p, tline{text: m.appSetupNote, class: "warn"}))
	}
	label := lipgloss.NewStyle().Foreground(p.Green).Render("Client ID: ")
	field := lipgloss.NewStyle().Foreground(p.Fg).Render(m.clientID.View()) +
		lipgloss.NewStyle().Foreground(p.Accent).Render("▋")
	hint := lipgloss.NewStyle().Foreground(p.Dim2).Render(
		"Basic Information → App Credentials · ↵ signs in · esc goes back")
	return lipgloss.JoinVertical(lipgloss.Left, strings.Join(head, "\n"), "", label+field, "", hint)
}

func (m Model) viewLogin(p theme.Palette, label, input string, lines []tline, hint string) string {
	var head []string
	for _, l := range lines {
		head = append(head, m.tlineRender(p, l))
	}
	cursor := lipgloss.NewStyle().Foreground(p.Accent).Render("▋")
	loginLine := lipgloss.NewStyle().Foreground(p.Green).Render(label+" ") +
		lipgloss.NewStyle().Foreground(p.Fg).Render(input) + cursor
	hintLine := lipgloss.NewStyle().Foreground(p.Dim2).Render(hint)
	return lipgloss.JoinVertical(lipgloss.Left, strings.Join(head, "\n"), "", loginLine, "", hintLine)
}

func (m Model) tlineRender(p theme.Palette, l tline) string {
	s := lipgloss.NewStyle()
	switch l.class {
	case "accent":
		s = s.Foreground(p.Accent).Bold(true)
	case "ok":
		s = s.Foreground(p.Green)
	case "warn":
		s = s.Foreground(p.Yellow)
	case "fill":
		s = s.Foreground(p.Dim2)
	case "dim":
		s = s.Foreground(p.Dim)
	default:
		s = s.Foreground(p.Fg).Bold(true)
	}
	return s.Render(l.text)
}

// ── launch ───────────────────────────────────────────────────────────────────

// viewLaunch is the full-bleed, centered finale (no card): a green confirmation,
// the heading, an inline preference summary, and the enter-workspace CTA.
func (m Model) viewLaunch(p theme.Palette) string {
	mark := lipgloss.NewStyle().Foreground(p.Green).Render("✓ session configured — welcome aboard")
	head := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render("You're all set, @" + orDefault(m.handle.Value(), "you"))

	val := func(s string) string { return lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(s) }
	lbl := func(s string) string { return lipgloss.NewStyle().Foreground(p.Dim).Render(s) }
	sep := lipgloss.NewStyle().Foreground(p.Dim2).Render(" · ")
	summary := val(themeName(m.themeName)) + " " + lbl("theme") + sep +
		val(accentName(m.accent)) + " " + lbl("accent") + sep +
		val(m.density) + " " + lbl("density") + sep + val(statusLabel(m.status))

	cta := lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 3).Render("enter workspace ↵")
	return lipgloss.JoinVertical(lipgloss.Center, mark, "", head, "", summary, "", "", cta)
}

// selRow renders a selectable list row: a leading accent cursor bar, left
// content, and a right-aligned meta. Flat — no background fill.
func selRow(p theme.Palette, w int, selected bool, left, right string) string {
	bar := "  "
	if selected {
		bar = lipgloss.NewStyle().Foreground(p.Accent).Render("▌") + " "
	}
	lead := bar + left
	gap := w - lipgloss.Width(lead) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return lead + strings.Repeat(" ", gap) + right
}

func themeName(v string) string {
	for _, o := range themeOpts {
		if o.val == v {
			return o.name
		}
	}
	return v
}
func accentName(v string) string {
	for _, o := range accentOpts {
		if o.val == v {
			return o.name
		}
	}
	return v
}
func statusLabel(v string) string {
	for _, o := range statusOpts {
		if o.val == v {
			return o.label
		}
	}
	return v
}
