// Package components holds the presentational pieces of the app — sidebar,
// message list, thread rail, composer, title bar, status bar — ported from the
// handoff's tui-components.jsx. They are render helpers driven by the app model's
// navigation state; they own no state themselves.
package components

import (
	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

// PresenceColor maps a presence status to its dot color.
func PresenceColor(p theme.Palette, status string) lipgloss.Color {
	switch status {
	case "online":
		return p.Green
	case "away":
		return p.Yellow
	case "dnd":
		return p.Red
	default:
		return p.Dim2
	}
}

// PresenceDot renders a colored presence dot (offline shows a dim ring glyph).
func PresenceDot(p theme.Palette, status string) string {
	glyph := "●"
	if status == "offline" || status == "" {
		glyph = "○"
	}
	return lipgloss.NewStyle().Foreground(PresenceColor(p, status)).Render(glyph)
}

// Wrap word-wraps a styled line to width (ansi-aware), hard-breaking any single
// word that still overflows.
func Wrap(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, ln := range strings.Split(ansi.Wordwrap(s, width, ""), "\n") {
		for lipgloss.Width(ln) > width {
			out = append(out, ansi.Truncate(ln, width, ""))
			ln = ansi.TruncateLeft(ln, width, "")
		}
		out = append(out, ln)
	}
	return out
}
