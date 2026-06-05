package onboarding

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/abrahamkuri/slack-tui/internal/theme"
)

const trainerRailW = 22

var drillAside = []string{
	"Movement keys work in every pane — the sidebar, the message list, and threads.",
	"No send button to hunt for. Your hands never leave home row.",
	"Threads keep side-conversations out of the main flow.",
	"The palette jumps to any channel or runs a command — the fastest way around.",
}

// kc renders a keycap chip used inside instruction text.
func kc(p theme.Palette, s string) string {
	return lipgloss.NewStyle().Foreground(p.Accent).Render("[" + s + "]")
}

func drillInstruction(p theme.Palette, drill int) string {
	fg := lipgloss.NewStyle().Foreground(p.Fg)
	switch drill {
	case 0:
		return fg.Render("Press ") + kc(p, "j") + fg.Render(" and ") + kc(p, "k") + fg.Render(" to move the selection down and up.")
	case 1:
		return fg.Render("Press ") + kc(p, "i") + fg.Render(" to enter INSERT, type a reply, then ") + kc(p, "↵") + fg.Render(" to send.")
	case 2:
		return fg.Render("Select a message and press ") + kc(p, "t") + fg.Render(" to open its thread.")
	default:
		return fg.Render("Press ") + kc(p, "⌃K") + fg.Render(" to open the command palette, then ") + kc(p, "esc") + fg.Render(" to close.")
	}
}

// viewTrainer renders the two-column trainer box: drill rail | stage.
func (m Model) viewTrainer(p theme.Palette, w int) string {
	t := m.trainer
	stageW := w - trainerRailW - 3 // outer borders (2) + divider (1)

	rail := m.railColumn(p, t)
	stage := m.stageColumn(p, t, stageW)
	h := len(stage)
	if len(rail) > h {
		h = len(rail)
	}

	bs := lipgloss.NewStyle().Foreground(p.Border)
	var b strings.Builder
	b.WriteString(bs.Render("┌"+strings.Repeat("─", trainerRailW)+"┬"+strings.Repeat("─", stageW)+"┐") + "\n")
	for i := 0; i < h; i++ {
		rc, sc := "", ""
		if i < len(rail) {
			rc = rail[i]
		}
		if i < len(stage) {
			sc = stage[i]
		}
		b.WriteString(bs.Render("│") + padPlain(rc, trainerRailW) + bs.Render("│") + padPlain(sc, stageW) + bs.Render("│") + "\n")
	}
	b.WriteString(bs.Render("└" + strings.Repeat("─", trainerRailW) + "┴" + strings.Repeat("─", stageW) + "┘"))
	return b.String()
}

// railColumn renders the drill tabs.
func (m Model) railColumn(p theme.Palette, t trainerState) []string {
	out := []string{""}
	for i, d := range drills {
		cur := i == t.drill
		done := t.done[i]
		mark, markColor := "○", p.Dim2
		if done {
			mark, markColor = "◉", p.Green
		} else if cur {
			mark, markColor = "▸", p.Accent
		}
		labelColor := p.Dim
		if cur {
			labelColor = p.Fg
		}
		bar := " "
		if cur {
			bar = lipgloss.NewStyle().Foreground(p.Accent).Render("▌")
		}
		left := bar + " " + lipgloss.NewStyle().Foreground(markColor).Render(mark) + " " +
			lipgloss.NewStyle().Foreground(labelColor).Render(d.label)
		keys := lipgloss.NewStyle().Foreground(p.Dim2).Render(d.keys)
		gap := trainerRailW - lipgloss.Width(left) - lipgloss.Width(keys) - 1
		if gap < 1 {
			gap = 1
		}
		line := left + strings.Repeat(" ", gap) + keys + " "
		if cur {
			line = lipgloss.NewStyle().Background(p.SelBg).Render(padPlain(line, trainerRailW))
		}
		out = append(out, line, "")
	}
	return out
}

// stageColumn renders the instruction, aside, and the mini app.
func (m Model) stageColumn(p theme.Palette, t trainerState, stageW int) []string {
	pad := func(s string) string { return " " + s }
	textW := stageW - 2

	var out []string
	out = append(out, "")
	for _, l := range strings.Split(lipgloss.NewStyle().Width(textW).Render(drillInstruction(p, t.drill)), "\n") {
		out = append(out, pad(l))
	}
	for _, l := range strings.Split(lipgloss.NewStyle().Width(textW).Foreground(p.Dim).Render(drillAside[t.drill]), "\n") {
		out = append(out, pad(l))
	}
	out = append(out, "")
	for _, l := range m.miniApp(p, t, stageW-2) {
		out = append(out, pad(l))
	}
	out = append(out, "")
	if t.done[t.drill] {
		banner := "✓ " + drills[t.drill].label + " — got it"
		if t.drill == len(drills)-1 {
			banner += ". tutorial complete!"
		} else {
			banner += ", next drill…"
		}
		out = append(out, pad(lipgloss.NewStyle().Foreground(p.Green).Render(banner)))
	}
	return out
}

// miniApp renders the bordered miniature message app (messages + composer).
func (m Model) miniApp(p theme.Palette, t trainerState, w int) []string {
	bs := lipgloss.NewStyle().Foreground(p.Border)
	innerW := w - 2
	var rows []string
	for i, mm := range miniMsgs {
		rows = append(rows, m.miniMessage(p, t, i, mm, innerW)...)
	}

	// composer
	var chip string
	if t.mode == "insert" {
		chip = lipgloss.NewStyle().Background(p.Green).Foreground(p.Bg).Bold(true).Render(" INSERT ")
	} else {
		chip = lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Render(" NORMAL ")
	}
	var composer string
	if t.mode == "insert" && t.drill == 1 {
		composer = chip + lipgloss.NewStyle().Foreground(p.Accent).Render("  ❯ ") + t.mini.View()
	} else {
		ph := "message #engineering"
		if t.drill == 1 {
			ph = "press i to write"
		}
		composer = chip + lipgloss.NewStyle().Foreground(p.Dim2).Render("  · "+ph)
	}

	var out []string
	out = append(out, bs.Render("┌"+strings.Repeat("─", innerW)+"┐"))
	for _, r := range rows {
		out = append(out, bs.Render("│")+padPlain(" "+r, innerW)+bs.Render("│"))
	}
	out = append(out, bs.Render("├"+strings.Repeat("─", innerW)+"┤"))
	out = append(out, bs.Render("│")+padPlain(" "+composer, innerW)+bs.Render("│"))
	out = append(out, bs.Render("└"+strings.Repeat("─", innerW)+"┘"))
	return out
}

// miniMessage renders one mini message (possibly multi-line), with selection,
// reactions, and the thread affordance.
func (m Model) miniMessage(p theme.Palette, t trainerState, i int, mm miniMsg, w int) []string {
	sel := i == t.sel
	const gutter = 6
	bodyW := w - gutter - 1
	if bodyW < 8 {
		bodyW = 8
	}

	tm := lipgloss.NewStyle().Foreground(p.Dim2).Width(gutter).Render(mm.time)
	user := lipgloss.NewStyle().Foreground(p.Token(mm.color)).Bold(true).Render(mm.user + " ")
	head := tm + user + miniText(p, mm.text)

	var lines []string
	for _, l := range strings.Split(lipgloss.NewStyle().Width(w-1).Render(head), "\n") {
		lines = append(lines, l)
	}
	replies := mm.replies
	if i == 0 && t.sent {
		replies++
	}
	if mm.react != "" {
		lines = append(lines, strings.Repeat(" ", gutter)+lipgloss.NewStyle().Background(p.SelBg).Render(" "+mm.react+" "))
	}
	if replies > 0 {
		aff := lipgloss.NewStyle().Foreground(p.Dim).Render("└─ ") +
			lipgloss.NewStyle().Foreground(p.Accent).Render(fmt.Sprintf("%d replies", replies))
		if t.drill == 2 && !t.threadOpen {
			aff += lipgloss.NewStyle().Foreground(p.Dim2).Render("  ↵ press t")
		}
		lines = append(lines, strings.Repeat(" ", gutter)+aff)
	}

	if sel {
		bar := lipgloss.NewStyle().Foreground(p.Accent).Render("▌")
		for j, l := range lines {
			lines[j] = bar + lipgloss.NewStyle().Width(w-1).Background(p.SelBg).Render(ansi.Truncate(l, w-1, ""))
		}
	}
	return lines
}

// miniText renders a mini message body, highlighting @you.
func miniText(p theme.Palette, text string) string {
	fg := lipgloss.NewStyle().Foreground(p.Fg)
	me := lipgloss.NewStyle().Foreground(p.Orange).Bold(true)
	parts := strings.Split(text, "@you")
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteString(me.Render("@you"))
		}
		b.WriteString(fg.Render(part))
	}
	return b.String()
}
