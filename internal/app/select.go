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

// cellIn maps (x,y) to a cell within one pane's viewport.
// x0 is the screen column of the first content column; y0 is always 2.
func cellIn(lines []string, top, x0, y0, innerW, innerH, x, y int) (cell, bool) {
	relX, relY := x-x0, y-y0
	if relX < 0 || relX >= innerW || relY < 0 || relY >= innerH {
		return cell{}, false
	}
	line := top + relY
	if line >= len(lines) {
		return cell{}, false
	}
	// snap to grapheme boundary so wide runes (emoji/CJK) are not split
	w := lipgloss.Width(lines[line])
	col := lipgloss.Width(ansi.Truncate(lines[line], clamp(relX, 0, w), ""))
	return cell{line: line, col: col}, true
}

// cellAt maps a screen coordinate (x, y) to a content cell, returning the
// pane it belongs to ("messages" or "thread"). Tests the message pane first,
// then the thread pane when it is open. Returns ok=false if outside content.
func (m Model) cellAt(x, y int) (cell, string, bool) {
	const y0 = 2
	lines, top, innerW, innerH := m.msgViewport()
	if c, ok := cellIn(lines, top, sidebarWidth+2, y0, innerW, innerH, x, y); ok {
		return c, focusMessages, true
	}
	if m.threadOpen() {
		tl, ttop, tiw, tih, tx0 := m.threadViewport()
		if c, ok := cellIn(tl, ttop, tx0, y0, tiw, tih, x, y); ok {
			return c, focusThread, true
		}
	}
	return cell{}, "", false
}

// selectionText returns the visible text spanned by the current selection
// (selAnchor..selHead), ordered, ANSI-stripped, trailing-space-trimmed per
// line, and newline-joined. Returns "" when the range is degenerate.
func (m Model) selectionText() string {
	var lines []string
	if m.selPane == focusThread {
		lines, _, _, _, _ = m.threadViewport()
	} else {
		lines, _, _, _ = m.msgViewport()
	}
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

// highlightVisible paints the selection range onto a visible slice of lines.
// visible is the already-sliced window; top is the content line index of
// visible[0]. a and b must be pre-ordered (a <= b). The slice is modified in
// place and returned. Used by both renderCenter and renderThread.
func highlightVisible(visible []string, top int, a, b cell, sel lipgloss.Style) []string {
	for j := range visible {
		L := top + j
		if L < a.line || L > b.line {
			continue
		}
		from, to := 0, lipgloss.Width(visible[j])
		if L == a.line {
			from = a.col
		}
		if L == b.line {
			to = b.col
		}
		if to <= from {
			continue
		}
		seg := ansi.Strip(ansi.TruncateLeft(ansi.Truncate(visible[j], to, ""), from, ""))
		visible[j] = spliceLine(visible[j], sel.Render(seg), from)
	}
	return visible
}
