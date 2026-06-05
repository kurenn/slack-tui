package onboarding

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/abrahamkuri/slack-tui/internal/theme"
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

// bootPlace lays a boot-family block at the top-left with a left margin, padded
// to fill the stage height.
func (m Model) bootPlace(block string, stageH int) string {
	out := []string{"", ""} // top margin
	for _, l := range strings.Split(block, "\n") {
		out = append(out, "    "+l)
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
		return bootScreen(p, m.oauth.render(p))
	case phaseAuth:
		return bootScreen(p, m.viewAuth(p))
	case phaseToken:
		return bootScreen(p, m.viewLogin(p, "token:", m.token.View(), []tline{
			{text: "authenticate with an access token", class: "fg"},
			{text: "paste a workspace token — begins with xoxp- or xoxb-.", class: "dim"},
		}, "paste your token, then press ↵ enter · it never leaves this machine"))
	case phaseIdentity:
		var lines []tline
		if m.provider == "guest" {
			lines = []tline{{text: "guest session — pick a display handle for this demo.", class: "dim"}}
		} else {
			lines = []tline{
				{text: "authenticated ✓  ·  workspace @monospace-labs", class: "ok"},
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
