package components

import (
	"strings"

	"github.com/kurenn/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Composer renders the input box: a bordered block with a prompt glyph, the
// (possibly multi-line) input view, and a right-aligned hint on the first row.
// The border turns accent in INSERT mode. Height is 2 + input rows.
func Composer(p theme.Palette, prompt, inputView string, insert bool, width int) string {
	borderColor := p.Border
	if insert {
		borderColor = p.Accent
	}
	bs := lipgloss.NewStyle().Foreground(borderColor)
	innerW := width - 2

	promptStyled := lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Render(prompt)
	hint := "i to write"
	if insert {
		hint = "↵ send · esc normal"
	}
	hintStyled := lipgloss.NewStyle().Foreground(p.Dim2).Render(hint)

	row := func(left, right string) string {
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
		return bs.Render("│") + left + strings.Repeat(" ", gap) + right + bs.Render("│")
	}

	inputRows := strings.Split(inputView, "\n")
	rows := []string{bs.Render("┌" + strings.Repeat("─", innerW) + "┐")}
	rows = append(rows, row(" "+promptStyled+" "+inputRows[0], hintStyled+" "))
	for _, r := range inputRows[1:] { // continuation rows align under the input
		rows = append(rows, row("   "+r, " "))
	}
	rows = append(rows, bs.Render("└"+strings.Repeat("─", innerW)+"┘"))
	return strings.Join(rows, "\n")
}
