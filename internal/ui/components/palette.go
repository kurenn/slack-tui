package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/abrahamkuri/slack-tui/internal/theme"
)

// PaletteItem is one command-palette row.
type PaletteItem struct {
	Icon  string
	Label string
	Hint  string
	Kind  string // channel | dm | cmd — drives the icon color
}

// Palette renders the command-palette overlay box: an input row plus a windowed
// list of items, with an accent border. width is the box's outer width; maxRows
// caps the visible list. The returned block is meant to be composited over the
// base frame by the caller.
func Palette(p theme.Palette, inputView string, items []PaletteItem, index, width, maxRows int) string {
	innerW := width - 2
	bs := lipgloss.NewStyle().Foreground(p.Accent).Background(p.Panel)
	fill := lipgloss.NewStyle().Background(p.Panel)

	row := func(s string) string {
		return bs.Render("│") + fill.Width(innerW).Render(ansi.Truncate(s, innerW, "")) + bs.Render("│")
	}

	var b strings.Builder
	b.WriteString(bs.Render("┌" + strings.Repeat("─", innerW) + "┐"))
	b.WriteByte('\n')

	// input row: ":" prompt + query + esc chip
	prompt := lipgloss.NewStyle().Foreground(p.Accent).Background(p.Panel).Bold(true).Render(" : ")
	esc := lipgloss.NewStyle().Foreground(p.Dim2).Background(p.Panel).Render(" esc ")
	mid := innerW - lipgloss.Width(prompt) - lipgloss.Width(esc)
	inputCell := fill.Width(mid).Render(ansi.Truncate(inputView, mid, ""))
	b.WriteString(bs.Render("│") + prompt + inputCell + esc + bs.Render("│"))
	b.WriteByte('\n')

	// separator
	b.WriteString(bs.Render("├" + strings.Repeat("─", innerW) + "┤"))
	b.WriteByte('\n')

	if len(items) == 0 {
		b.WriteString(row(lipgloss.NewStyle().Foreground(p.Dim2).Background(p.Panel).Render("  no matches")))
		b.WriteByte('\n')
	} else {
		start := windowStart(index, len(items), maxRows)
		end := start + maxRows
		if end > len(items) {
			end = len(items)
		}
		for i := start; i < end; i++ {
			b.WriteString(row(paletteRow(p, items[i], i == index, innerW)))
			b.WriteByte('\n')
		}
	}

	b.WriteString(bs.Render("└" + strings.Repeat("─", innerW) + "┘"))
	return b.String()
}

func paletteRow(p theme.Palette, it PaletteItem, selected bool, w int) string {
	iconColor := p.Purple
	switch it.Kind {
	case "channel":
		iconColor = p.Blue
	case "dm":
		iconColor = p.Green
	}
	bg := p.Panel
	if selected {
		bg = p.SelBg
	}
	icon := lipgloss.NewStyle().Foreground(iconColor).Background(bg).Render(it.Icon)
	labelColor := p.Fg
	if !selected {
		labelColor = p.Dim
	}
	label := lipgloss.NewStyle().Foreground(labelColor).Background(bg).Render(it.Label)
	hint := lipgloss.NewStyle().Foreground(p.Dim2).Background(bg).Render(it.Hint)

	left := " " + icon + "  " + label
	gap := w - lipgloss.Width(left) - lipgloss.Width(hint) - 1
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + hint + " "
	return lipgloss.NewStyle().Width(w).Background(bg).Render(ansi.Truncate(line, w, ""))
}

// windowStart keeps the selected index visible within a maxRows window.
func windowStart(index, total, maxRows int) int {
	if total <= maxRows {
		return 0
	}
	start := index - maxRows/2
	if start < 0 {
		start = 0
	}
	if start > total-maxRows {
		start = total - maxRows
	}
	return start
}
