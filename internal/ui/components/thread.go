package components

import (
	"fmt"
	"strings"

	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

// ThreadScroll renders a thread's scrollable content: the root message, a
// "N replies" separator, then the replies. Returns the lines plus the starting
// line index of each reply (selIndex selects among replies when focused).
func ThreadScroll(p theme.Palette, ws *data.Workspace, root data.Message, selIndex int, focused bool, innerW int) (lines []string, starts []int) {
	// Root message — never selected, replies stripped (the affordance lives in
	// the main list, not the rail).
	rootMsg := root
	rootMsg.Replies = nil
	lines = append(lines, messageLines(p, ws, rootMsg, innerW, false, false, false)...)

	replies := root.Replies
	sep := fmt.Sprintf(" %d %s ", len(replies), plural(len(replies), "reply", "replies"))
	side := (innerW - lipgloss.Width(sep)) / 2
	if side < 0 {
		side = 0
	}
	rule := lipgloss.NewStyle().Foreground(p.Border).Render(strings.Repeat("─", side))
	lines = append(lines, rule+lipgloss.NewStyle().Foreground(p.Dim2).Render(sep)+rule)

	for i, r := range replies {
		starts = append(starts, len(lines))
		msg := data.Message{ID: r.ID, UserID: r.UserID, Time: r.Time, Text: r.Text}
		lines = append(lines, messageLines(p, ws, msg, innerW, i == selIndex && focused, false, false)...)
	}
	starts = append(starts, len(lines))
	return lines, starts
}
