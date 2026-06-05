package components

import (
	"strings"

	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Composer renders the input box: a bordered row with a prompt glyph, the text
// input view, and a right-aligned hint. The border turns accent in INSERT mode.
// Height is always 3 (top rule, input row, bottom rule).
func Composer(p theme.Palette, prompt, inputView string, insert bool, width int) string {
	borderColor := p.Border
	if insert {
		borderColor = p.Accent
	}
	bs := lipgloss.NewStyle().Foreground(borderColor).Background(p.Panel)
	innerW := width - 2

	promptStyled := lipgloss.NewStyle().Foreground(p.Accent).Background(p.Panel).Bold(true).Render(prompt)
	hint := "i to write"
	if insert {
		hint = "↵ send · esc normal"
	}
	hintStyled := lipgloss.NewStyle().Foreground(p.Dim2).Background(p.Panel).Render(hint)

	left := " " + promptStyled + " " + inputView
	right := hintStyled + " "
	gap := innerW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// drop the hint if there's no room
		right = " "
		gap = innerW - lipgloss.Width(left) - lipgloss.Width(right)
	}
	if gap < 0 {
		left = ansi.Truncate(left, innerW-lipgloss.Width(right), "")
		gap = 0
	}
	row := bs.Render("│") +
		lipgloss.NewStyle().Background(p.Panel).Render(left+strings.Repeat(" ", gap)+right) +
		bs.Render("│")

	top := bs.Render("┌" + strings.Repeat("─", innerW) + "┐")
	bottom := bs.Render("└" + strings.Repeat("─", innerW) + "┘")
	return strings.Join([]string{top, row, bottom}, "\n")
}
