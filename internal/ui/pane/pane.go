// Package pane renders the core visual primitive: a single-line box-drawing
// border with the title embedded in the top rule and an optional right-label,
// ported from the handoff's Pane component. A focused pane borders + titles in
// the accent color. Everything in the app is built out of these.
package pane

import (
	"strings"

	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Options configures a pane render. Width/Height are the OUTER dimensions in
// cells, including the border rules. Body is the pre-rendered inner content;
// it's clipped to the inner box (it does not scroll — callers manage that).
type Options struct {
	Title   string
	Right   string
	Focused bool
	Width   int
	Height  int
	Body    string
}

// Render draws the pane and returns it as a multi-line string of exactly
// Options.Height lines, each Options.Width cells wide.
func Render(p theme.Palette, o Options) string {
	if o.Width < 2 || o.Height < 2 {
		return ""
	}
	innerW := o.Width - 2
	bodyH := o.Height - 2

	borderColor := p.Border
	titleColor := p.Dim
	if o.Focused {
		borderColor = p.Accent
		titleColor = p.Accent
	}
	bs := lipgloss.NewStyle().Foreground(borderColor).Background(p.Panel)
	ts := lipgloss.NewStyle().Foreground(titleColor).Background(p.Panel).Bold(o.Focused)
	rs := lipgloss.NewStyle().Foreground(p.Dim2).Background(p.Panel)

	var b strings.Builder
	b.WriteString(topRule(bs, ts, rs, o.Title, o.Right, innerW))
	b.WriteByte('\n')

	lines := strings.Split(o.Body, "\n")
	fill := lipgloss.NewStyle().Width(innerW).Background(p.Panel)
	for i := 0; i < bodyH; i++ {
		var content string
		if i < len(lines) {
			content = ansi.Truncate(lines[i], innerW, "")
		}
		b.WriteString(bs.Render("│"))
		b.WriteString(fill.Render(content))
		b.WriteString(bs.Render("│"))
		b.WriteByte('\n')
	}

	b.WriteString(bs.Render("└" + strings.Repeat("─", innerW) + "┘"))
	return b.String()
}

// topRule builds the title rule: ┌─ title ──────── right ─┐
// The title is load-bearing, so when title and right-label can't both fit the
// right-label is dropped rather than truncating the title.
func topRule(bs, ts, rs lipgloss.Style, title, right string, innerW int) string {
	const minFill = 2 // keep at least a short rule between title and right
	titleW := lipgloss.Width(title)
	rlen := 0
	rightVis := ""
	// "─ " + title + " " is 3+titleW; " " + right + " ─" is 3+rlen.
	if right != "" && 3+titleW+3+lipgloss.Width(right)+minFill <= innerW {
		right = ansi.Truncate(right, max(0, innerW-6), "")
		rlen = lipgloss.Width(right)
		rightVis = bs.Render(" ") + rs.Render(right) + bs.Render(" ─")
	}
	maxTitle := innerW - 4 // "┌─ " + title + " " minimum
	if rlen > 0 {
		maxTitle = innerW - (rlen + 3) - 4
	}
	if maxTitle < 0 {
		maxTitle = 0
	}
	title = ansi.Truncate(title, maxTitle, "…")
	tlen := lipgloss.Width(title)

	leftVis := 3 + tlen // "─ " + title + " "
	rightWidth := 0
	if right != "" {
		rightWidth = rlen + 3 // " " + right + " ─"
	}
	fill := innerW - leftVis - rightWidth
	if fill < 0 {
		fill = 0
	}

	var b strings.Builder
	b.WriteString(bs.Render("┌"))
	b.WriteString(bs.Render("─ "))
	b.WriteString(ts.Render(title))
	b.WriteString(bs.Render(" "))
	b.WriteString(bs.Render(strings.Repeat("─", fill)))
	b.WriteString(rightVis)
	b.WriteString(bs.Render("┐"))
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
