package onboarding

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/abrahamkuri/slack-tui/internal/theme"
)

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	p := m.pal()
	if m.width < 50 || m.height < 16 {
		return lipgloss.NewStyle().Foreground(p.Dim).Render("onboarding needs at least 50×16 — resize the terminal.")
	}
	stageH := m.height - 2
	stage := lipgloss.Place(m.width, stageH, lipgloss.Center, lipgloss.Center, m.stage(p),
		lipgloss.WithWhitespaceBackground(p.Bg))
	return strings.Join([]string{m.titlebar(p), stage, m.statusbar(p)}, "\n")
}

func (m Model) stageW() int {
	w := m.width - 6
	if w > 72 {
		w = 72
	}
	return w
}

func (m Model) stage(p theme.Palette) string {
	switch m.phase {
	case phaseBoot:
		return m.boot.render(p)
	case phaseOAuth:
		return m.oauth.render(p)
	case phaseAuth:
		return m.viewAuth(p)
	case phaseToken:
		return m.viewPrompt(p, "token:", m.token.View(), []tline{
			{"[ token ] authenticate with an access token", "accent"},
			{"paste a workspace token (begins with xoxp- or xoxb-).", "dim"},
		}, "paste your token, then press ↵ enter · we never send it anywhere")
	case phaseIdentity:
		var lines []tline
		if m.provider == "guest" {
			lines = []tline{{"guest session — pick a display handle for this demo.", "dim"}}
		} else {
			lines = []tline{
				{"authenticated ✓  ·  workspace @monospace-labs", "ok"},
				{"choose how teammates will see you.", "dim"},
			}
		}
		return m.viewPrompt(p, "display handle:", m.handle.View(), lines, "set your handle, then press ↵ enter to continue")
	case phaseWizard:
		return m.viewWizard(p)
	case phaseLaunch:
		return m.viewLaunch(p)
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
		loc = fmt.Sprintf("step %d / %d", m.stepIndex+1, len(wizSteps))
	} else if h := m.handle.Value(); h != "" && m.phase != phaseAuth && m.phase != phaseOAuth {
		loc = "@" + h
	}
	locCell := bg.Foreground(p.Fg).Padding(0, 1).Render(loc)

	hints := m.hints()
	var hb strings.Builder
	for _, h := range hints {
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
		h := [][2]string{{"j/k", "choose"}, {"1-9", "jump"}, {"↵", "next"}}
		if m.stepIndex > 0 {
			h = append(h, [2]string{"esc", "back"})
		}
		return h
	}
}

// ── auth ─────────────────────────────────────────────────────────────────────

func (m Model) viewAuth(p theme.Palette) string {
	w := m.stageW()
	head := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render("Sign in to your workspace")
	sub := lipgloss.NewStyle().Foreground(p.Dim).Render("Choose how to authenticate. Use j/k or 1–4, then ↵.")

	var rows []string
	for i, o := range authOpts {
		sel := i == m.authSel
		mark := lipgloss.NewStyle().Foreground(authColor(p, o.id)).Bold(true).Render(o.mark)
		key := lipgloss.NewStyle().Foreground(p.Dim2).Render(o.key)
		labelColor := p.Dim
		if sel {
			labelColor = p.Fg
		}
		label := lipgloss.NewStyle().Foreground(labelColor).Bold(o.primary).Render(o.label)
		hint := lipgloss.NewStyle().Foreground(p.Dim2).Render(o.hint)
		bar := "  "
		if sel {
			bar = lipgloss.NewStyle().Foreground(p.Accent).Render("▌") + " "
		}
		left := bar + key + "  " + mark + "  " + label
		gap := w - lipgloss.Width(left) - lipgloss.Width(hint)
		if gap < 1 {
			gap = 1
		}
		line := left + strings.Repeat(" ", gap) + hint
		bgc := p.Bg
		if sel {
			bgc = p.SelBg
		}
		rows = append(rows, lipgloss.NewStyle().Width(w).Background(bgc).Render(line))
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
	w := m.stageW()
	var head []string
	for _, l := range lines {
		head = append(head, m.tlineRender(p, l))
	}
	prompt := lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Render(label+" ") +
		lipgloss.NewStyle().Foreground(p.Fg).Render(input)
	box := lipgloss.NewStyle().Width(w).Foreground(p.Fg).
		Border(lipgloss.NormalBorder()).BorderForeground(p.Accent).Padding(0, 1).Render(prompt)
	hintLine := lipgloss.NewStyle().Foreground(p.Dim2).Render(hint)
	return lipgloss.JoinVertical(lipgloss.Left, strings.Join(head, "\n"), "", box, "", hintLine)
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
		s = s.Foreground(p.Fg)
	}
	return s.Render(l.text)
}

// ── launch ───────────────────────────────────────────────────────────────────

func (m Model) viewLaunch(p theme.Palette) string {
	mark := lipgloss.NewStyle().Foreground(p.Green).Render("✓ session configured — welcome aboard")
	head := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render("You're all set, @" + orDefault(m.handle.Value(), "you"))
	val := func(s string) string { return lipgloss.NewStyle().Foreground(p.Accent).Render(s) }
	sep := lipgloss.NewStyle().Foreground(p.Dim2).Render(" · ")
	sum := lipgloss.NewStyle().Foreground(p.Dim).Render(
		val(themeName(m.themeName)) + " theme" + sep + val(accentName(m.accent)) + " accent" + sep +
			val(m.density) + " density" + sep + val(statusLabel(m.status)))
	cta := lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 2).Render("enter workspace ↵")
	return lipgloss.JoinVertical(lipgloss.Center, mark, "", head, "", sum, "", cta)
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
