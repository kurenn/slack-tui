package components

import (
	"fmt"
	"strings"

	"github.com/kurenn/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

// TitleBar renders the top chrome: the workspace title and the current user's
// presence + handle on the right.
func TitleBar(p theme.Palette, workspace, meHandle, meStatus string, panes, width int) string {
	bg := lipgloss.NewStyle().Background(p.TitlebarBg)
	dim := bg.Foreground(p.Dim)
	title := dim.Render("slack-tui — ") +
		bg.Foreground(p.Fg).Bold(true).Render(workspace) +
		dim.Render(fmt.Sprintf(" — %d panes", panes))
	me := bg.Foreground(PresenceColor(p, meStatus)).Render("●") + dim.Render(" @"+meHandle)

	left := "  " + title
	gap := width - lipgloss.Width(left) - lipgloss.Width(me) - 1
	if gap < 1 {
		gap = 1
	}
	return left + bg.Render(strings.Repeat(" ", gap)) + me + bg.Render(" ")
}
