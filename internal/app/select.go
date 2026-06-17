package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// cell is a position in the rendered message body expressed as an absolute
// content-line index and a display column (both zero-based).
type cell struct{ line, col int }

// orderCells returns (a, b) sorted so a is at or before b in reading order.
func orderCells(a, b cell) (cell, cell) {
	if a.line < b.line || (a.line == b.line && a.col <= b.col) {
		return a, b
	}
	return b, a
}

// cellAt maps a screen coordinate (x, y) to a content cell in the message
// pane. Returns ok=false if the position is outside the message content
// region. Content starts at screen column sidebarWidth+2 (sidebar + 1 gap +
// 1 border) and screen row 2 (title row + pane top border).
func (m Model) cellAt(x, y int) (cell, bool) {
	lines, top, innerW, innerH := m.msgViewport()
	const contentX0 = sidebarWidth + 2
	const contentY0 = 2
	relX, relY := x-contentX0, y-contentY0
	if relX < 0 || relX >= innerW || relY < 0 || relY >= innerH {
		return cell{}, false
	}
	line := top + relY
	if line >= len(lines) {
		return cell{}, false
	}
	// snap the column to a grapheme boundary so wide runes (emoji/CJK) aren't split
	w := lipgloss.Width(lines[line])
	col := lipgloss.Width(ansi.Truncate(lines[line], clamp(relX, 0, w), ""))
	return cell{line: line, col: col}, true
}

// selectionText returns the visible text spanned by the current selection
// (selAnchor..selHead), ordered, ANSI-stripped, trailing-space-trimmed per
// line, and newline-joined. Returns "" when the range is degenerate.
func (m Model) selectionText() string {
	lines, _, _, _ := m.msgViewport()
	if len(lines) == 0 {
		return ""
	}
	a, b := orderCells(m.selAnchor, m.selHead)
	if a == b {
		return ""
	}
	clampLine := func(l int) int { return clamp(l, 0, len(lines)-1) }
	a.line, b.line = clampLine(a.line), clampLine(b.line)
	var segs []string
	for L := a.line; L <= b.line; L++ {
		from, to := 0, lipgloss.Width(lines[L])
		if L == a.line {
			from = a.col
		}
		if L == b.line {
			to = b.col
		}
		if to < from {
			to = from
		}
		seg := ansi.Strip(ansi.TruncateLeft(ansi.Truncate(lines[L], to, ""), from, ""))
		segs = append(segs, strings.TrimRight(seg, " "))
	}
	return strings.Join(segs, "\n")
}
