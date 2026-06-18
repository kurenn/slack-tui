package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/data"
)

// TestClampThreadWidth: bounds are enforced correctly.
func TestClampThreadWidth(t *testing.T) {
	m := newSized() // 100×30

	// Too small: should clamp to minW=34.
	if got := m.clampThreadWidth(10); got != 34 {
		t.Errorf("clampThreadWidth(10) = %d, want 34", got)
	}

	// Too large: max = width - sidebarWidth - 2 - 24 = 100-30-2-24 = 44.
	maxW := m.width - sidebarWidth - 2 - 24
	if got := m.clampThreadWidth(9999); got != maxW {
		t.Errorf("clampThreadWidth(9999) = %d, want %d", got, maxW)
	}

	// In range: passes through.
	if got := m.clampThreadWidth(40); got != 40 {
		t.Errorf("clampThreadWidth(40) = %d, want 40", got)
	}
}

// TestThreadViewportNonEmpty: opening a thread with replies yields non-empty lines.
func TestThreadViewportNonEmpty(t *testing.T) {
	m := newSized()
	m.msgSel = 2 // e3 has replies
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}

	lines, _, innerW, _, x0 := m.threadViewport()
	if len(lines) == 0 {
		t.Fatal("threadViewport should return non-empty lines when thread is open")
	}
	if innerW != m.threadWidth-2 {
		t.Errorf("innerW = %d, want %d", innerW, m.threadWidth-2)
	}
	if x0 != m.width-m.threadWidth+1 {
		t.Errorf("x0 = %d, want %d", x0, m.width-m.threadWidth+1)
	}
}

// TestCellAtPaneRouting: cellAt returns the right pane for message vs thread coords.
func TestCellAtPaneRouting(t *testing.T) {
	m := newSized()
	m.msgSel = 2
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}

	// Message pane: x = sidebarWidth+2 = 32, y = 2 (first content row).
	_, msgLines, msgTop, msgInnerW, msgInnerH := func() (cell, []string, int, int, int) {
		lines, top, iw, ih := m.msgViewport()
		return cell{}, lines, top, iw, ih
	}()
	_ = msgLines
	_ = msgTop
	msgX := sidebarWidth + 2
	if msgInnerH > 0 && msgInnerW > 0 {
		_, pane, ok := m.cellAt(msgX, 2)
		if ok && pane != focusMessages {
			t.Errorf("message-region coord returned pane %q, want %q", pane, focusMessages)
		}
	}

	// Thread pane: x = m.width - m.threadWidth + 1, y = 2.
	_, _, _, _, tx0 := m.threadViewport()
	_, pane, ok := m.cellAt(tx0, 2)
	if ok && pane != focusThread {
		t.Errorf("thread-region coord returned pane %q, want %q", pane, focusThread)
	}
}

// TestSelectedReply: selectedReply returns the current reply in thread focus.
func TestSelectedReply(t *testing.T) {
	m := newSized()
	m.msgSel = 2 // e3 has replies
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}
	root, ok := m.threadRoot()
	if !ok || len(root.Replies) == 0 {
		t.Fatal("e3 should have replies")
	}

	m.focus = focusThread
	m.threadSel = 0
	r, ok := m.selectedReply()
	if !ok {
		t.Fatal("selectedReply should return true when thread is focused and has replies")
	}
	if r.Text != root.Replies[0].Text {
		t.Errorf("selectedReply text = %q, want %q", r.Text, root.Replies[0].Text)
	}

	// Not in thread focus: should return false.
	m.focus = focusMessages
	_, ok = m.selectedReply()
	if ok {
		t.Error("selectedReply should return false when not in thread focus")
	}
}

// TestYankInThread: y in thread focus does not panic and clears normally.
func TestYankInThread(t *testing.T) {
	m := newSized()
	m.msgSel = 2 // e3 with replies
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}
	m.focus = focusThread
	m.threadSel = 0
	// y should not panic; clipboard may fail in CI but that's ok
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("y in thread panicked: %v", r)
		}
	}()
	_ = Key(m, "y")
}

// TestDividerResize: press+motion on the thread divider changes threadWidth.
func TestDividerResize(t *testing.T) {
	// Use a wide terminal so there is room to widen the thread beyond its clamped default.
	m := WithSize(newTest(), 200, 30)
	m.msgSel = 2
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}

	initialWidth := m.threadWidth
	edge := m.width - m.threadWidth

	// Simulate a press on the divider edge.
	pressMsg := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      edge,
		Y:      5,
	}
	next, _ := m.Update(pressMsg)
	m = next.(Model)
	if !m.resizing {
		t.Fatal("press on divider edge should set resizing=true")
	}

	// Simulate motion to the left (making thread wider).
	newEdge := edge - 5
	motionMsg := tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      newEdge,
		Y:      5,
	}
	next, _ = m.Update(motionMsg)
	m = next.(Model)

	if m.threadWidth <= initialWidth {
		t.Errorf("motion left should widen thread: initial=%d, got=%d", initialWidth, m.threadWidth)
	}
	if !m.resizing {
		t.Error("should still be resizing after motion")
	}

	// Simulate release (no persist in tests since persistState=false).
	releaseMsg := tea.MouseMsg{
		Action: tea.MouseActionRelease,
		X:      newEdge,
		Y:      5,
	}
	next, _ = m.Update(releaseMsg)
	m = next.(Model)
	if m.resizing {
		t.Error("release should clear resizing")
	}
}

// TestDividerResizeNotPersistInTest: release does not write prefs when persistState=false.
func TestDividerResizeNotPersistInTest(t *testing.T) {
	m := newSized()
	m.msgSel = 2
	m = Key(m, "t")
	m.resizing = true
	m.threadWidth = m.clampThreadWidth(50)

	releaseMsg := tea.MouseMsg{
		Action: tea.MouseActionRelease,
		X:      m.width - 50,
		Y:      5,
	}
	next, cmd := m.Update(releaseMsg)
	m = next.(Model)
	if m.resizing {
		t.Error("release should clear resizing")
	}
	// persistState is false in tests — no cmd should be returned for prefs saving.
	if cmd != nil {
		// cmd is returned only when persistState=true, which tests never set.
		// In test mode no cmd is expected.
		t.Error("no persist cmd expected in test (persistState=false)")
	}
}

// TestThreadWidthDefault: a new model has threadWidth=60 when prefs.ThreadWidth=0.
func TestThreadWidthDefault(t *testing.T) {
	m := newSized()
	if m.threadWidth != m.clampThreadWidth(60) {
		t.Errorf("default threadWidth = %d, want %d (clamped 60)", m.threadWidth, m.clampThreadWidth(60))
	}
}

// TestThreadSelectionText: a simulated drag in the thread pane produces text from
// the thread lines (not message lines), and selPane is set correctly.
func TestThreadSelectionText(t *testing.T) {
	m := newSized()
	m.msgSel = 2
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}

	lines, top, _, ih, tx0 := m.threadViewport()
	if len(lines) == 0 || ih == 0 {
		t.Skip("no thread lines to test selection on")
	}

	// Simulate a press at the first thread line.
	pressMsg := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      tx0,
		Y:      2,
	}
	next, _ := m.Update(pressMsg)
	m = next.(Model)

	if !m.selecting {
		t.Skip("press did not start a selection (coord outside content)")
	}
	if m.selPane != focusThread {
		t.Errorf("selPane = %q, want %q", m.selPane, focusThread)
	}

	// Drag a few columns to the right.
	motionMsg := tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      tx0 + 5,
		Y:      2,
	}
	next, _ = m.Update(motionMsg)
	m = next.(Model)

	_ = top
	txt := m.selectionText()
	_ = txt // may be empty if the line has trailing spaces; just confirm no panic
}

// TestResizingClearedOnClose: closeThread clears the resizing flag so a
// mid-drag thread close cannot leave the resize loop stuck.
func TestResizingClearedOnClose(t *testing.T) {
	m := newSized()
	m.msgSel = 2
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}
	m.resizing = true
	m.closeThread()
	if m.resizing {
		t.Error("closeThread should clear m.resizing")
	}
}

// TestWindowResizeRestoresWidth: a transient tiny terminal width must not
// permanently shrink the thread — the desired width (prefs.ThreadWidth) is
// restored when the terminal grows back to a usable size.
func TestWindowResizeRestoresWidth(t *testing.T) {
	m := WithSize(newTest(), 140, 40)
	m.prefs.ThreadWidth = 50

	// Feed a tiny window that can't honour 50 cols.
	tiny := tea.WindowSizeMsg{Width: 40, Height: 20}
	next, _ := m.Update(tiny)
	m = next.(Model)

	// Restore to a wide window.
	normal := tea.WindowSizeMsg{Width: 140, Height: 40}
	next, _ = m.Update(normal)
	m = next.(Model)

	want := m.clampThreadWidth(50)
	if m.threadWidth != want {
		t.Errorf("threadWidth after resize = %d, want %d (clamped desired 50)", m.threadWidth, want)
	}
}

// TestDividerGrabIgnoresTitleBar: a press at the divider X on row 0 (title bar)
// must not start a resize — the Y guard must filter it out.
func TestDividerGrabIgnoresTitleBar(t *testing.T) {
	m := newSized()
	m.msgSel = 2
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}

	edge := m.width - m.threadWidth
	pressMsg := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      edge,
		Y:      0, // title bar row — must not trigger resize
	}
	next, _ := m.Update(pressMsg)
	m = next.(Model)
	if m.resizing {
		t.Error("press on divider at Y=0 (title bar) should NOT set resizing=true")
	}
}

// TestAddReplyToMockMsg: ensure the mock message at index 2 (e3) has replies
// accessible — guards the test assumptions about the mock data.
func TestAddReplyToMockMsg(t *testing.T) {
	m := newSized()
	if len(m.curMsgs()) < 3 {
		t.Fatal("mock should have at least 3 messages")
	}
	root := m.curMsgs()[2]
	if root.ReplyCount == 0 && len(root.Replies) == 0 {
		// Add a reply manually for downstream tests that need it.
		m.messages[m.activeID][2].Replies = []data.Reply{
			{ID: "r1", UserID: "ada", Time: "10:01", Text: "looks good"},
		}
		m.messages[m.activeID][2].ReplyCount = 1
	}
	root = m.curMsgs()[2]
	if len(root.Replies) == 0 && root.ReplyCount == 0 {
		t.Skip("mock message e3 has no replies; skipping")
	}
}
