package onboarding

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/abrahamkuri/slack-tui/internal/theme"
)

var drillInstr = []string{
	"Press j and k to move the selection down and up.",
	"Press i to enter INSERT, type a reply, then ↵ to send. esc cancels.",
	"Select a message and press t to open its thread.",
	"Press ⌃K to open the command palette, then esc to close.",
}

func (m Model) viewTrainer(p theme.Palette, w int) string {
	t := m.trainer

	// drill rail
	var rail []string
	for i, d := range drills {
		glyph, c := "○", p.Dim2
		if t.done[i] {
			glyph, c = "◉", p.Green
		} else if i == t.drill {
			glyph, c = "▸", p.Accent
		}
		rail = append(rail, lipgloss.NewStyle().Foreground(c).Render(glyph+" ")+
			lipgloss.NewStyle().Foreground(railLabelColor(p, i, t)).Render(d.label))
	}
	railLine := strings.Join(rail, lipgloss.NewStyle().Foreground(p.Dim2).Render("   "))
	instr := wrapStyled(lipgloss.NewStyle().Foreground(p.Dim), drillInstr[t.drill], w)

	// mini app box: box outer = innerW (w-4); content width after padding = innerW-2.
	innerW := w - 4
	cw := innerW - 2
	var lines []string
	for i, mm := range miniMsgs {
		tm := lipgloss.NewStyle().Foreground(p.Dim2).Render(mm.time + " ")
		user := lipgloss.NewStyle().Foreground(p.Token(mm.color)).Bold(true).Render(mm.user + " ")
		text := miniText(p, mm.text)
		line := tm + user + text
		if i == t.sel && t.drill == 0 {
			line = lipgloss.NewStyle().Foreground(p.Accent).Render("▌") +
				lipgloss.NewStyle().Width(cw-1).Background(p.SelBg).Render(ansi.Truncate(line, cw-1, ""))
		} else {
			line = " " + ansi.Truncate(line, cw-1, "")
		}
		lines = append(lines, line)
	}

	// composer
	var comp string
	if t.mode == "insert" && t.drill == 1 {
		comp = lipgloss.NewStyle().Foreground(p.Bg).Background(p.Green).Bold(true).Render(" INSERT ") +
			lipgloss.NewStyle().Foreground(p.Accent).Render(" ❯ ") + t.mini.View()
	} else {
		ph := "press i to write"
		if t.drill != 1 {
			ph = "message #engineering"
		}
		comp = lipgloss.NewStyle().Foreground(p.Accent).Render(" NORMAL ") +
			lipgloss.NewStyle().Foreground(p.Dim2).Render(" · "+ph)
	}
	lines = append(lines, "", comp)

	if t.paletteOpen {
		lines = append(lines, "",
			lipgloss.NewStyle().Foreground(p.Accent).Render(" : ")+lipgloss.NewStyle().Foreground(p.Dim).Render("jump to channel, run a command…"),
			lipgloss.NewStyle().Foreground(p.Blue).Render(" # ")+lipgloss.NewStyle().Foreground(p.Fg).Render("engineering"),
			lipgloss.NewStyle().Foreground(p.Dim2).Render("   press esc to close"))
	}

	box := lipgloss.NewStyle().Width(innerW).Padding(0, 1).
		Border(lipgloss.NormalBorder()).BorderForeground(p.Border).
		Render(strings.Join(lines, "\n"))

	out := []string{railLine, "", instr, "", box}
	if t.done[t.drill] {
		banner := "✓ " + drills[t.drill].label + " — got it"
		if t.drill == len(drills)-1 {
			banner += ". tutorial complete!"
		} else {
			banner += ", next drill…"
		}
		out = append(out, lipgloss.NewStyle().Foreground(p.Green).Render(banner))
	}
	return lipgloss.JoinVertical(lipgloss.Left, out...)
}

func railLabelColor(p theme.Palette, i int, t trainerState) lipgloss.Color {
	if t.done[i] {
		return p.Fg
	}
	if i == t.drill {
		return p.Fg
	}
	return p.Dim
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
