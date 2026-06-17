package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// TestCellAtRegion: cellAt maps screen coords to content cells, returning
// ok=false for positions outside the message content region.
func TestCellAtRegion(t *testing.T) {
	m := WithSize(newTest(), 120, 40)

	lines, top, innerW, innerH := m.msgViewport()
	if innerH <= 0 || len(lines) == 0 {
		t.Skip("no renderable message content")
	}

	// In-region: top-left corner of content area.
	c, ok := m.cellAt(sidebarWidth+2, 2)
	if !ok {
		t.Fatal("cellAt(sidebarWidth+2, 2) should be in-region, got ok=false")
	}
	if c.line != top {
		t.Errorf("cellAt in-region: c.line = %d, want %d (top)", c.line, top)
	}
	if c.col != 0 {
		t.Errorf("cellAt in-region: c.col = %d, want 0", c.col)
	}

	// Out left/up: (0, 0) is in the sidebar.
	_, ok = m.cellAt(0, 0)
	if ok {
		t.Error("cellAt(0, 0) should be out-of-region (left of content), got ok=true")
	}

	// Out right: x == sidebarWidth+2+innerW is one past the right edge.
	_, ok = m.cellAt(sidebarWidth+2+innerW, 2)
	if ok {
		t.Errorf("cellAt(sidebarWidth+2+innerW=%d, 2) should be out-of-region (right), got ok=true", sidebarWidth+2+innerW)
	}

	// Out below: y == 2+innerH is one past the bottom edge.
	_, ok = m.cellAt(sidebarWidth+2, 2+innerH)
	if ok {
		t.Errorf("cellAt(sidebarWidth+2, 2+innerH=%d) should be out-of-region (below), got ok=true", 2+innerH)
	}
}

// TestSelectionTextSingleLine: selectionText returns the correct strip of a
// single rendered line, and ordering is canonical (anchor after head works too).
func TestSelectionTextSingleLine(t *testing.T) {
	m := WithSize(newTest(), 120, 40)
	lines, _, _, _ := m.msgViewport()

	// Find a line wide enough to select a 3-column slice (cols 1..4).
	L := -1
	for i, l := range lines {
		if lipgloss.Width(l) >= 5 {
			L = i
			break
		}
	}
	if L < 0 {
		t.Skip("no rendered line with width >= 5")
	}

	m.selAnchor = cell{L, 1}
	m.selHead = cell{L, 4}
	m.selActive = true

	expected := strings.TrimRight(
		ansi.Strip(ansi.TruncateLeft(ansi.Truncate(lines[L], 4, ""), 1, "")),
		" ",
	)

	got := m.selectionText()
	if got != expected {
		t.Errorf("selectionText single-line:\n  got  %q\n  want %q", got, expected)
	}

	// Reversed anchor/head must produce the same result.
	m.selAnchor = cell{L, 4}
	m.selHead = cell{L, 1}
	got2 := m.selectionText()
	if got2 != expected {
		t.Errorf("selectionText reversed anchor/head:\n  got  %q\n  want %q", got2, expected)
	}
}

// TestSelectionTextMultiLine: a selection spanning two lines yields the tail of
// the first line and the head of the second, joined by "\n".
func TestSelectionTextMultiLine(t *testing.T) {
	m := WithSize(newTest(), 120, 40)
	lines, _, _, _ := m.msgViewport()

	// Find consecutive lines L, L+1 both with width >= 4.
	L := -1
	for i := 0; i+1 < len(lines); i++ {
		if lipgloss.Width(lines[i]) >= 4 && lipgloss.Width(lines[i+1]) >= 4 {
			L = i
			break
		}
	}
	if L < 0 {
		t.Skip("no consecutive rendered lines with width >= 4")
	}

	m.selAnchor = cell{L, 2}
	m.selHead = cell{L + 1, 3}
	m.selActive = true

	firstSeg := strings.TrimRight(
		ansi.Strip(ansi.TruncateLeft(lines[L], 2, "")),
		" ",
	)
	secondSeg := strings.TrimRight(
		ansi.Strip(ansi.Truncate(lines[L+1], 3, "")),
		" ",
	)
	expected := firstSeg + "\n" + secondSeg

	got := m.selectionText()
	if got != expected {
		t.Errorf("selectionText multi-line:\n  got  %q\n  want %q", got, expected)
	}
}

// TestSelectionEmptyWhenDegenerate: a selection where anchor == head returns "".
func TestSelectionEmptyWhenDegenerate(t *testing.T) {
	m := WithSize(newTest(), 120, 40)
	lines, _, _, _ := m.msgViewport()
	if len(lines) == 0 {
		t.Skip("no rendered lines")
	}
	m.selAnchor = cell{0, 2}
	m.selHead = cell{0, 2}
	m.selActive = true
	if got := m.selectionText(); got != "" {
		t.Errorf("degenerate selection should return \"\", got %q", got)
	}
}

// TestMousePressMotionUpdatesSelection: a left-button press in the content
// region starts a selection; a motion advances selHead; a release at the
// same position as the anchor (empty selection) finishes without writing to
// the clipboard.
func TestMousePressMotionUpdatesSelection(t *testing.T) {
	m := WithSize(newTest(), 120, 40)

	lines, _, _, innerH := m.msgViewport()
	if innerH <= 0 || len(lines) == 0 {
		t.Skip("no renderable message content")
	}

	x0, y0 := sidebarWidth+2, 2

	// --- Press ---
	next, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x0,
		Y:      y0,
	})
	m = next.(Model)

	if !m.selecting {
		t.Fatal("after press: m.selecting should be true")
	}
	if !m.selActive {
		t.Fatal("after press: m.selActive should be true")
	}
	if m.selAnchor != m.selHead {
		t.Errorf("after press: anchor (%+v) should equal head (%+v)", m.selAnchor, m.selHead)
	}

	anchor := m.selAnchor

	// --- Motion: move 3 columns to the right ---
	next, _ = m.Update(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
		X:      x0 + 3,
		Y:      y0,
	})
	m = next.(Model)

	if !m.selecting {
		t.Fatal("after motion: m.selecting should still be true")
	}
	if m.selHead.line != anchor.line {
		t.Errorf("after motion: selHead.line = %d, want %d", m.selHead.line, anchor.line)
	}
	if m.selHead.col <= anchor.col {
		// col should have advanced (or been clamped to the line width)
		t.Errorf("after motion: selHead.col = %d, want > %d (anchor col)", m.selHead.col, anchor.col)
	}

	// --- Release at anchor position → empty selection, no clipboard write ---
	// The release handler maps the release coordinates via cellAt, so releasing
	// at (x0, y0) sets selHead back to the anchor cell. Because anchor == head,
	// selectionText() returns "" and selActive is cleared as well.
	next, _ = m.Update(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      x0,
		Y:      y0,
	})
	m = next.(Model)

	if m.selecting {
		t.Error("after release: m.selecting should be false")
	}
	if m.selActive {
		t.Error("after release at anchor: m.selActive should be false (empty selection)")
	}
}
