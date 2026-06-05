package onboarding

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/abrahamkuri/slack-tui/internal/ui/pane"
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
	stage := lipgloss.Place(m.width, stageH, lipgloss.Center, lipgloss.Center, m.stage(p),
		lipgloss.WithWhitespaceBackground(p.Bg))
	return strings.Join([]string{m.titlebar(p), stage, m.statusbar(p)}, "\n")
}

// panelDims returns the centered card's outer width/height.
func (m Model) panelDims() (int, int) {
	pw := m.width - 8
	if pw > 66 {
		pw = 66
	}
	if pw < 50 {
		pw = m.width - 4
	}
	maxPH := 17
	if m.phase == phaseWizard {
		maxPH = 21 // the keyboard trainer step is the tallest
	}
	ph := m.height - 4
	if ph > maxPH {
		ph = maxPH
	}
	return pw, ph
}

// contentW is the usable text width inside the card (border + 2-space pad each side).
func (m Model) contentW() int {
	pw, _ := m.panelDims()
	return pw - 6
}

// frame renders a fixed-size centered card: title (+ right label), body top-aligned
// under a blank line, and footer pinned to the last row.
func (m Model) frame(p theme.Palette, title, right, body, footer string) string {
	pw, ph := m.panelDims()
	innerW, innerH := pw-2, ph-2
	pad := "  "

	inner := make([]string, innerH)
	row := 1 // one blank line of top padding
	for _, l := range strings.Split(body, "\n") {
		if row >= innerH-1 {
			break
		}
		inner[row] = pad + l
		row++
	}
	if footer != "" {
		inner[innerH-1] = pad + footer
	}
	box := pane.Render(p, pane.Options{
		Title: title, Right: right, Focused: true,
		Width: pw, Height: ph, Body: strings.Join(inner, "\n"),
	})
	_ = innerW
	return box
}

func (m Model) stage(p theme.Palette) string {
	switch m.phase {
	case phaseBoot:
		return m.boot.render(p)
	case phaseOAuth:
		return m.oauth.render(p)
	case phaseAuth:
		return m.frame(p, "sign in", "", m.viewAuth(p), "")
	case phaseToken:
		body := m.viewPrompt(p, "token", m.token.View(), []tline{
			{"authenticate with an access token", "fg"},
			{"paste a workspace token — begins with xoxp- or xoxb-.", "dim"},
		}, "↵ continue · the token never leaves this machine")
		return m.frame(p, "authenticate", "", body, "")
	case phaseIdentity:
		var lines []tline
		if m.provider == "guest" {
			lines = []tline{{"guest session", "fg"}, {"pick a display handle for this demo.", "dim"}}
		} else {
			lines = []tline{{"authenticated ✓  ·  @monospace-labs", "ok"}, {"choose how teammates will see you.", "dim"}}
		}
		body := m.viewPrompt(p, "handle", m.handle.View(), lines, "↵ continue")
		return m.frame(p, "identity", "", body, "")
	case phaseWizard:
		var body string
		if m.step() == "keyboard" {
			body = lipgloss.JoinVertical(lipgloss.Left, m.wizHeading(p), "", m.viewTrainer(p, m.contentW()))
		} else {
			body = m.viewWizardBody(p)
		}
		return m.frame(p, "setup", m.stepRail(p), body, m.viewFooter(p, m.contentW()))
	case phaseLaunch:
		return m.frame(p, "ready", "", m.viewLaunch(p), m.viewLaunchFooter(p))
	}
	return ""
}

// ── chrome ───────────────────────────────────────────────────────────────────

func (m Model) titlebar(p theme.Palette) string {
	bg := lipgloss.NewStyle().Background(p.TitlebarBg)
	dim := bg.Foreground(p.Dim)
	dot := func(hex string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Background(p.TitlebarBg).Render("●")
	}
	lights := " " + dot("#ff5f57") + " " + dot("#febc2e") + " " + dot("#28c840") + "  "
	title := dim.Render("slack-tui — ") + bg.Foreground(p.Fg).Bold(true).Render("onboarding")
	right := "monospace-labs"
	if h := m.handle.Value(); h != "" {
		right = "@" + h
	}
	r := dim.Render(right)
	left := lights + title
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(r) - 1
	if gap < 1 {
		gap = 1
	}
	return left + bg.Render(strings.Repeat(" ", gap)) + r + bg.Render(" ")
}

func (m Model) statusbar(p theme.Palette) string {
	bg := lipgloss.NewStyle().Background(p.StatusBg)
	label := map[string]string{
		phaseBoot: "BOOT", phaseAuth: "AUTH", phaseOAuth: "OAUTH", phaseToken: "TOKEN",
		phaseIdentity: "IDENTITY", phaseWizard: "SETUP", phaseLaunch: "READY",
	}[m.phase]
	mode := lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 1).Render(label)

	loc := "@monospace-labs"
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

// ── auth ─────────────────────────────────────────────────────────────────────

func (m Model) viewAuth(p theme.Palette) string {
	w := m.contentW()
	head := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render("Sign in to your workspace")
	sub := wrapStyled(lipgloss.NewStyle().Foreground(p.Dim), "Choose how to authenticate.", w)

	var rows []string
	for i, o := range authOpts {
		sel := i == m.authSel
		key := lipgloss.NewStyle().Foreground(p.Dim2).Render(o.key)
		mark := lipgloss.NewStyle().Foreground(authColor(p, o.id)).Bold(true).Render(o.mark)
		labelColor := p.Dim
		if sel {
			labelColor = p.Fg
		}
		label := lipgloss.NewStyle().Foreground(labelColor).Bold(o.primary).Render(o.label)
		hint := lipgloss.NewStyle().Foreground(p.Dim2).Render(o.hint)
		rows = append(rows, selRow(p, w, sel, key+"  "+mark+"  "+label, hint))
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, sub, "", strings.Join(rows, "\n"))
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

// ── prompt (token / identity) ────────────────────────────────────────────────

func (m Model) viewPrompt(p theme.Palette, label, input string, lines []tline, hint string) string {
	w := m.contentW()
	var head []string
	for _, l := range lines {
		head = append(head, m.tlineRender(p, l))
	}
	prefix := lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Render("❯ ") +
		lipgloss.NewStyle().Foreground(p.Dim).Render(label+" ")
	field := lipgloss.NewStyle().Width(w - lipgloss.Width(prefix)).Background(p.SelBg).
		Foreground(p.Fg).Render(input)
	hintLine := lipgloss.NewStyle().Foreground(p.Dim2).Render(hint)
	return lipgloss.JoinVertical(lipgloss.Left, strings.Join(head, "\n"), "", prefix+field, "", hintLine)
}

func (m Model) tlineRender(p theme.Palette, l tline) string {
	s := lipgloss.NewStyle()
	switch l.class {
	case "accent":
		s = s.Foreground(p.Accent).Bold(true)
	case "ok":
		s = s.Foreground(p.Green)
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

func (m Model) viewLaunch(p theme.Palette) string {
	w := m.contentW()
	mark := lipgloss.NewStyle().Foreground(p.Green).Render("✓ session configured")
	head := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render("You're all set, @" + orDefault(m.handle.Value(), "you"))
	row := func(k, v string) string {
		return lipgloss.NewStyle().Foreground(p.Dim2).Width(10).Render(k) +
			lipgloss.NewStyle().Foreground(p.Fg).Render(v)
	}
	summary := lipgloss.JoinVertical(lipgloss.Left,
		row("theme", themeName(m.themeName)),
		row("accent", accentName(m.accent)),
		row("density", m.density),
		row("status", statusLabel(m.status)),
	)
	_ = w
	return lipgloss.JoinVertical(lipgloss.Left, mark, "", head, "", summary)
}

func (m Model) viewLaunchFooter(p theme.Palette) string {
	cta := lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 2).Render("enter workspace ↵")
	w := m.contentW()
	gap := w - lipgloss.Width(cta)
	if gap < 0 {
		gap = 0
	}
	return strings.Repeat(" ", gap) + cta
}

// selRow renders a selectable list row: a leading cursor bar, left content, and
// a right-aligned meta, filled to width w with the selection background.
func selRow(p theme.Palette, w int, selected bool, left, right string) string {
	bar := "  "
	bgc := p.Bg
	if selected {
		bar = lipgloss.NewStyle().Foreground(p.Accent).Render("▌") + " "
		bgc = p.SelBg
	}
	lead := bar + left
	gap := w - lipgloss.Width(lead) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := lead + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().Width(w).Background(bgc).Render(line)
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
