package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/source"
	"github.com/kurenn/slack-tui/internal/theme"
)

// This file drives end-to-end UX flows through the real Bubble Tea Update
// loop — keys in, model/frame assertions out — for paths app_test.go doesn't
// already cover. Helpers here are prefixed "fl" to avoid colliding with
// identifiers other agents add to this shared package.

// flNextModel runs one Update and returns the concrete Model plus the cmd,
// for flows that need the returned command (Key() in dump.go discards it).
func flNextModel(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// flTryRun runs a cmd with a short deadline and reports whether it produced
// a message in time. Some batches mix fast mock-backed cmds with a real
// tea.Tick re-arming a poll timer (multiple seconds); calling every cmd in a
// batch naively would block the test on that timer. The abandoned goroutine
// completes harmlessly in the background (buffered channel, nothing reads
// it) once its real interval elapses.
func flTryRun(cmd tea.Cmd) (tea.Msg, bool) {
	if cmd == nil {
		return nil, false
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		return msg, true
	case <-time.After(200 * time.Millisecond):
		return nil, false
	}
}

// ── send: optimistic append then ack ────────────────────────────────────

// TestSendMessageOptimisticThenAck: enter posts a pending-* message
// immediately; the backend's ack (sentMsg) swaps it for the real message
// without changing the count.
func TestSendMessageOptimisticThenAck(t *testing.T) {
	m := newSized()
	before := len(m.curMsgs())
	m = Key(m, "i")
	for _, r := range "hello ack" {
		m = Key(m, string(r))
	}
	m, cmd := flNextModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("sending should return a command")
	}
	pendingID := m.curMsgs()[len(m.curMsgs())-1].ID
	if !isPending(pendingID) {
		t.Fatalf("message should be optimistic before the ack, id=%q", pendingID)
	}
	sm, ok := cmd().(sentMsg)
	if !ok || sm.err != nil {
		t.Fatalf("send should succeed against the mock, got %T %+v", sm, sm)
	}
	m, _ = flNextModel(m, sm)
	if got := len(m.curMsgs()); got != before+1 {
		t.Fatalf("message count after ack = %d, want %d (no duplication)", got, before+1)
	}
	last := m.curMsgs()[len(m.curMsgs())-1]
	if isPending(last.ID) {
		t.Errorf("message should carry the real backend ID after ack, still %q", last.ID)
	}
	if last.Text != "hello ack" {
		t.Errorf("acked message text = %q, want %q", last.Text, "hello ack")
	}
}

// TestSendMessageAckDedupesAgainstPolledDuplicate: the 6s poll can deliver the
// real message before the ack does (applyHistory carries the pending copy
// forward until then). When the ack finally lands, applySent must drop the
// now-redundant optimistic copy instead of leaving both — the "already"
// branch documented in applySent's comment.
func TestSendMessageAckDedupesAgainstPolledDuplicate(t *testing.T) {
	m := newSized()
	before := len(m.curMsgs())
	m = Key(m, "i")
	for _, r := range "dup race" {
		m = Key(m, string(r))
	}
	m, cmd := flNextModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	sm, ok := cmd().(sentMsg)
	if !ok || sm.err != nil {
		t.Fatalf("send should succeed, got %T %+v", sm, sm)
	}

	// Simulate the poll delivering the real message first: applyHistory
	// carries the still-pending optimistic copy forward alongside it.
	snapshot := append([]data.Message(nil), m.curMsgs()[:len(m.curMsgs())-1]...)
	snapshot = append(snapshot, sm.msg) // the "real" message the poll saw
	m, _ = flNextModel(m, historyMsg{convID: m.activeID, msgs: snapshot})

	found := 0
	for _, msg := range m.curMsgs() {
		if msg.ID == sm.msg.ID {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("expected exactly one copy of the polled message, found %d", found)
	}
	hasPending := false
	for _, msg := range m.curMsgs() {
		if msg.ID == sm.pendingID {
			hasPending = true
		}
	}
	if !hasPending {
		t.Fatal("test setup: the pending copy should still be present before the ack lands")
	}

	// Now the ack arrives: it must remove the pending copy, not duplicate.
	m, _ = flNextModel(m, sm)
	if got := len(m.curMsgs()); got != before+1 {
		t.Fatalf("message count after dedupe = %d, want %d", got, before+1)
	}
	for _, msg := range m.curMsgs() {
		if msg.ID == sm.pendingID {
			t.Error("pending copy should have been removed once the real one was already present")
		}
	}
}

// ── thread: open, render, reply ─────────────────────────────────────────

// TestThreadReplyOpenAndRenderInFrame opens a thread with existing replies
// and checks the frame actually renders a reply's text and the pane divider
// — renderThread and dividerCol were previously exercised only through state
// assertions (thread_test.go), never through View().
func TestThreadReplyOpenAndRenderInFrame(t *testing.T) {
	m := WithSize(newTest(), 200, 30) // wide enough for sidebar+center+thread
	m.msgSel = 2                      // e3, which has replies in the mock
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("t should open the thread")
	}
	root, ok := m.threadRoot()
	if !ok || len(root.Replies) == 0 {
		t.Fatal("e3 should have replies in the mock fixture")
	}
	frame := ansi.Strip(m.View())
	// A short, single-word fragment: the pane is narrow enough that the full
	// reply sentence word-wraps across lines, which would break a substring
	// match on the whole text even though it is genuinely rendered.
	fragment := strings.Fields(root.Replies[0].Text)[2] // "coalesces"
	if !strings.Contains(frame, fragment) {
		t.Errorf("rendered frame should contain the reply fragment %q (full reply: %q)\n%s", fragment, root.Replies[0].Text, frame)
	}
	// The divider column is a run of "┊" between the center pane and the
	// thread pane — one full-height line of it should appear literally.
	if !strings.Contains(frame, "┊") {
		t.Error("rendered frame should contain the thread divider column")
	}
}

// ── react / unreact via the reaction picker ─────────────────────────────

// TestReactThenUnreactViaPicker drives the full picker UX (not just the
// resulting reactMsg): 'a' opens it, the overlay lists the curated choices,
// picking one reacts, and picking the same one again removes it.
func TestReactThenUnreactViaPicker(t *testing.T) {
	m := newSized()
	m.msgSel = 0
	msgID := m.curMsgs()[0].ID

	m = Key(m, "a")
	if !m.picker.open || m.picker.kind != "react" {
		t.Fatalf("a should open the reaction picker, open=%v kind=%q", m.picker.open, m.picker.kind)
	}
	overlay := ansi.Strip(m.View())
	if !strings.Contains(overlay, "thumbsup") {
		t.Errorf("reaction picker overlay should list choices, got:\n%s", overlay)
	}

	m, cmd := flNextModel(m, tea.KeyMsg{Type: tea.KeyEnter}) // picks index 0: thumbsup
	rm, ok := cmd().(reactMsg)
	if !ok || rm.err != nil || rm.name != "thumbsup" || !rm.added {
		t.Fatalf("picking the first choice should react with thumbsup, got %T %+v", rm, rm)
	}
	m, _ = flNextModel(m, rm)
	found := false
	for _, r := range m.curMsgs()[0].Reactions {
		if r.Emoji == "👍" && r.Count == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("reaction should be applied, got %+v", m.curMsgs()[0].Reactions)
	}
	if frame := ansi.Strip(m.View()); !strings.Contains(frame, "👍") {
		t.Error("the reacted message row should render the emoji")
	}

	// React again with the same emoji: toggles it off.
	m = Key(m, "a")
	m, cmd = flNextModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	rm2, ok := cmd().(reactMsg)
	if !ok || rm2.added {
		t.Fatalf("reacting again should toggle off, got %+v", rm2)
	}
	m, _ = flNextModel(m, rm2)
	for _, r := range m.curMsgs()[0].Reactions {
		if r.Emoji == "👍" {
			t.Fatalf("reaction should be removed, still %+v", m.curMsgs()[0].Reactions)
		}
	}
	if msgID != m.curMsgs()[0].ID {
		t.Fatal("test setup invariant broken: selected message identity changed")
	}
}

// ── delete: confirm overlay, cancel paths ───────────────────────────────

// TestDeleteConfirmCancelPaths extends TestDeleteConfirm (app_test.go, which
// only exercises the 'y' path): the overlay renders its prompt text, and both
// 'n' and 'esc' cancel without touching the message.
func TestDeleteConfirmCancelPaths(t *testing.T) {
	for _, cancelKey := range []string{"n", "esc"} {
		t.Run(cancelKey, func(t *testing.T) {
			m := newSized()
			m.messages[m.activeID] = append(m.messages[m.activeID],
				data.Message{ID: "mine-" + cancelKey, UserID: "me", Time: "10:00", Text: "keep me"})
			m.msgSel = len(m.curMsgs()) - 1
			before := len(m.curMsgs())
			m = Key(m, "d")
			m = Key(m, "d")
			if !m.confirm.open {
				t.Fatal("dd should open the confirm")
			}
			overlay := ansi.Strip(m.View())
			if !strings.Contains(overlay, "delete this message?") {
				t.Errorf("confirm overlay should show its prompt, got:\n%s", overlay)
			}
			m = Key(m, cancelKey)
			if m.confirm.open {
				t.Errorf("%q should close the confirm", cancelKey)
			}
			if len(m.curMsgs()) != before {
				t.Errorf("%q should not delete the message, len=%d want %d", cancelKey, len(m.curMsgs()), before)
			}
		})
	}
}

// ── yank: clipboard stub ────────────────────────────────────────────────

// TestYankMessageWritesClipboardStub: y copies the selected message's exact
// text via the stubbed writeClipboard seam (main_test.go), never the real
// clipboard.
func TestYankMessageWritesClipboardStub(t *testing.T) {
	clipboardStub = ""
	m := newSized()
	m.msgSel = 0
	want := m.curMsgs()[0].Text
	_ = Key(m, "y")
	if clipboardStub != want {
		t.Errorf("clipboardStub = %q, want %q", clipboardStub, want)
	}
}

// TestYankThreadReplyWritesClipboardStub: y in the thread pane copies the
// selected reply's text, not the root message's.
func TestYankThreadReplyWritesClipboardStub(t *testing.T) {
	clipboardStub = ""
	m := newSized()
	m.msgSel = 2 // e3, has replies
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}
	m.focus = focusThread
	m.threadSel = 0
	root, ok := m.threadRoot()
	if !ok || len(root.Replies) == 0 {
		t.Fatal("e3 should have replies")
	}
	want := root.Replies[0].Text
	_ = Key(m, "y")
	if clipboardStub != want {
		t.Errorf("clipboardStub = %q, want the reply text %q", clipboardStub, want)
	}
}

// ── find: n/N wraps through matches ──────────────────────────────────────

// TestFindNextPrevWrap: '/' jumps to the first match after the cursor, 'n'
// advances to the next, 'N' returns to the previous. The engineering
// fixture's e1/e2/e5 contain "the" (verified independently below, not by
// calling jumpFind); msgSel starts on e5 (index 4, itself a match), so a
// forward search from there must move to the NEXT match after it, wrapping
// to e1 (index 0).
func TestFindNextPrevWrap(t *testing.T) {
	m := newSized()
	matches := map[int]bool{}
	for i, msg := range m.curMsgs() {
		hay := strings.ToLower(msg.Text + " " + m.ws.Users[msg.UserID].Name)
		if strings.Contains(hay, "the") {
			matches[i] = true
		}
	}
	if len(matches) < 2 {
		t.Fatalf("test needs at least 2 matching messages in the fixture, found %v", matches)
	}
	if m.msgSel != len(m.curMsgs())-1 {
		t.Fatalf("test assumes msgSel starts at the last message, got %d", m.msgSel)
	}

	m = Key(m, "/")
	if !m.findOpen {
		t.Fatal("/ should open the find prompt")
	}
	for _, r := range "the" {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	if m.findOpen {
		t.Fatal("enter should close the find prompt")
	}
	firstHit := m.msgSel
	if !matches[firstHit] {
		t.Fatalf("cursor landed on non-matching message %d", firstHit)
	}

	m = Key(m, "n")
	secondHit := m.msgSel
	if !matches[secondHit] {
		t.Fatalf("n moved to a non-matching message %d", secondHit)
	}
	if secondHit == firstHit {
		t.Error("n should move the cursor to a different match")
	}

	m = Key(m, "N")
	if m.msgSel != firstHit {
		t.Errorf("N should return to the previous match %d, got %d", firstHit, m.msgSel)
	}
}

// ── mark all as read via the command palette ────────────────────────────

// TestMarkAllReadViaPalette: the "Mark all as read" palette command clears
// every conversation's badge locally and calls markAllReadCmd — this exercises
// runPalette's "cmd:read" case, previously untested.
func TestMarkAllReadViaPalette(t *testing.T) {
	m := newSized()
	// "incidents" and "design" carry unread in the mock fixture.
	if m.meta["incidents"].Unread == 0 || m.meta["design"].Unread == 0 {
		t.Fatal("test fixture assumption broken: incidents/design should start unread")
	}
	m, cmd := flNextModel(m, tea.KeyMsg{Type: tea.KeyCtrlK})
	m = Key(m, "mark all")
	m, cmd = flNextModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Mark all as read should return a command")
	}
	if m.meta["incidents"].Unread != 0 || m.meta["design"].Unread != 0 {
		t.Errorf("unread badges should clear immediately, got incidents=%+v design=%+v",
			m.meta["incidents"], m.meta["design"])
	}
	// The follow-up is tea.Batch(markAllReadCmd, titleCmd) — bubbletea's Batch
	// collapses to the single non-nil cmd directly when the other is nil
	// (titleCmd is nil here: the unread total was already 0 pre-run), so
	// accept either shape and find the markedMsg either way.
	result := cmd()
	sawMarked := false
	check := func(msg tea.Msg) {
		if mm, isMarked := msg.(markedMsg); isMarked {
			sawMarked = true
			if mm.err != nil {
				t.Errorf("markAllReadCmd should succeed against the mock, got %v", mm.err)
			}
		}
	}
	if batch, ok := result.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				check(c())
			}
		}
	} else {
		check(result)
	}
	if !sawMarked {
		t.Errorf("expected a markedMsg result, got %T", result)
	}
}

// ── ] / [ jump to next/previous unread ──────────────────────────────────

// TestPrevUnreadJump: '[' moves to the previous unread conversation and
// marks it read — the ']' half is already covered by TestJumpToNextUnread.
func TestPrevUnreadJump(t *testing.T) {
	m := newSized() // engineering active; design/incidents/dm_ada unread
	m = Key(m, "[")
	if m.activeID == "engineering" {
		t.Errorf("[ should move to an unread conversation, still %q", m.activeID)
	}
	if m.meta[m.activeID].Unread != 0 {
		t.Errorf("the opened conversation should be marked read, meta=%+v", m.meta[m.activeID])
	}
}

// ── help overlay ─────────────────────────────────────────────────────────

// TestHelpOverlayOpenAndClose: '?' opens the keymap cheat sheet with the
// documented rows; esc/q/enter all close it, ctrl+c quits from inside it.
func TestHelpOverlayOpenAndClose(t *testing.T) {
	// A tall terminal: the help card has ~30 body rows and overlay() clips
	// anything past the frame's own height, so a merely-wide 100×30 (as
	// newSized gives) would silently drop the bottom rows this test checks.
	m := WithSize(newTest(), 100, 50)
	m = Key(m, "?")
	if !m.helpOpen {
		t.Fatal("? should open help")
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"dd", "ctrl+k", "quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("help overlay missing row for %q", want)
		}
	}
	for _, closeKey := range []string{"esc", "q", "enter"} {
		mm := Key(m, closeKey)
		if mm.helpOpen {
			t.Errorf("%q should close help", closeKey)
		}
	}
	_, cmd := flNextModel(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("ctrl+c from help should quit (non-nil cmd)")
	}
}

// ── status text flow ─────────────────────────────────────────────────────

// TestStatusTextFlowSuccessAndError opens the status-message prompt from the
// palette, sets an emoji-prefixed status, and separately checks the error
// path surfaces in the frame when SetStatusText fails.
func TestStatusTextFlowSuccessAndError(t *testing.T) {
	m := newSized()
	m, _ = flNextModel(m, tea.KeyMsg{Type: tea.KeyCtrlK})
	m = Key(m, "status message")
	m, cmd := flNextModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.statusTextOpen {
		t.Fatal("palette command should open the status-text prompt")
	}
	for _, r := range ":coffee: brb" {
		m = Key(m, string(r))
	}
	m, cmd = flNextModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.statusTextOpen {
		t.Error("enter should close the status-text prompt")
	}
	if cmd == nil {
		t.Fatal("submitting should return a command")
	}
	pm, ok := cmd().(presenceMsg)
	if !ok || pm.err != nil {
		t.Fatalf("mock SetStatusText should succeed, got %T %+v", pm, pm)
	}

	// esc closes without a command.
	m = newSized()
	m = Key(m, "ctrl+k")
	m = Key(m, "status message")
	m = Key(m, "enter")
	m = Key(m, "esc")
	if m.statusTextOpen {
		t.Error("esc should close the status-text prompt")
	}

	// Error path: wrap the mock so SetStatusText fails; the error must reach
	// the frame after Update processes the resulting presenceMsg.
	errSrc := &flErrStatusTextSource{Source: source.NewMock()}
	em := NewWith(errSrc, config.Defaults())
	em = WithSize(em, 100, 30)
	em, _ = flNextModel(em, tea.KeyMsg{Type: tea.KeyCtrlK})
	em = Key(em, "status message")
	em, cmd = flNextModel(em, tea.KeyMsg{Type: tea.KeyEnter})
	em = Key(em, "boom")
	em, cmd = flNextModel(em, tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	em, _ = flNextModel(em, msg)
	if em.loadErr == nil {
		t.Fatal("a failing SetStatusText should surface an error banner")
	}
	if frame := ansi.Strip(em.View()); !strings.Contains(frame, "status set failed") {
		t.Errorf("error banner should mention the failure, got:\n%s", frame)
	}
}

// flErrStatusTextSource wraps the mock, failing only SetStatusText — the
// critique-mandated pattern (embed the real mock, override one method)
// instead of hand-rolling a full Source recorder.
type flErrStatusTextSource struct{ source.Source }

func (flErrStatusTextSource) SetStatusText(text, emoji string) error {
	return errFlStatusText
}

var errFlStatusText = flStatusTextErr("status set failed")

type flStatusTextErr string

func (e flStatusTextErr) Error() string { return string(e) }

// ── settings: theme cycle persists to disk ──────────────────────────────

// TestSettingsThemeCyclePersists: cycling the theme in the settings overlay
// and closing it must write the new theme to prefs.json — TestSettingsPanel
// (app_test.go) checks the live cycle but never the persisted file.
func TestSettingsThemeCyclePersists(t *testing.T) {
	isolateConfigDir(t)
	m := newSized()
	m.openSettings()
	m.settingsSel = 0 // theme row
	before := m.prefs.Theme
	m.cycleSetting(1)
	if m.prefs.Theme == before {
		t.Fatal("cycling should change the in-memory theme")
	}
	want := m.prefs.Theme
	m = Key(m, "esc") // closeSettings saves prefs
	if m.settingsOpen {
		t.Fatal("esc should close settings")
	}
	// config.Load's bool return is saved.Onboarded, not read-success — a test
	// model is never onboarded, so only the theme value is asserted here.
	loaded, _ := config.Load()
	if loaded.Theme != want {
		t.Errorf("theme not persisted: loaded=%q want=%q", loaded.Theme, want)
	}
}

// ── view: narrow terminal, error banner, copy toast ─────────────────────

// TestNarrowTerminalMessage: below the minimum usable size, View shows only
// the "needs a larger terminal" message.
func TestNarrowTerminalMessage(t *testing.T) {
	m := WithSize(newTest(), 40, 20)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "needs a larger terminal") {
		t.Errorf("narrow terminal should show the resize hint, got:\n%s", view)
	}
}

// TestLoadErrBannerTruncated: a set loadErr renders as a banner, and an
// over-width message is truncated with an ellipsis rather than wrapping or
// overflowing the frame.
func TestLoadErrBannerTruncated(t *testing.T) {
	m := WithSize(newTest(), 60, 30)
	m.loadErr = errFlBanner(strings.Repeat("x", 200))
	view := ansi.Strip(m.View())
	lines := strings.Split(view, "\n")
	found := false
	for _, l := range lines {
		if strings.Contains(l, "slack:") {
			found = true
			if lipglossWidthFl(l) > m.width {
				t.Errorf("banner line wider than the frame: %d > %d", lipglossWidthFl(l), m.width)
			}
			if !strings.HasSuffix(strings.TrimRight(l, " "), "…") {
				t.Errorf("over-width banner should end with an ellipsis, got %q", l)
			}
		}
	}
	if !found {
		t.Fatal("loadErr should render a banner line")
	}
}

type errFlBanner string

func (e errFlBanner) Error() string { return string(e) }

func lipglossWidthFl(s string) int { return len([]rune(s)) }

// TestCopyToastClampedInFrame: an extreme copyToastX must be clamped so the
// toast stays inside the frame instead of being cut off or overflowing.
func TestCopyToastClampedInFrame(t *testing.T) {
	m := WithSize(newTest(), 80, 30)
	m.copyToast = "copied"
	m.copyToastX = 9999
	m.copyToastY = 1
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) <= m.copyToastY {
		t.Fatal("frame should have a line at copyToastY")
	}
	if got := lipgloss.Width(lines[m.copyToastY]); got > m.width {
		t.Errorf("toast line width %d exceeds frame width %d", got, m.width)
	}
	if !strings.Contains(ansi.Strip(view), "copied") {
		t.Error("copy toast text should be visible somewhere in the frame")
	}
}

// TestDumpWidthMatchesRequested guards the --dump dev flag: the first
// rendered line's printable width equals the requested width, for two sizes.
func TestDumpWidthMatchesRequested(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{100, 30}, {140, 40}} {
		out := Dump(newTest(), sz.w, sz.h)
		first := strings.SplitN(out, "\n", 2)[0]
		if got := lipgloss.Width(first); got != sz.w {
			t.Errorf("Dump(%d,%d) first line width = %d, want %d", sz.w, sz.h, got, sz.w)
		}
	}
}

// ── attach-file overlay ───────────────────────────────────────────────────

// TestAttachFileFlow: 'A' opens the attach prompt; a real, existing path
// stages the file and enters INSERT; a non-existent path flashes an error
// instead; esc cancels without staging anything.
func TestAttachFileFlow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newSized()
	m = Key(m, "A")
	if !m.attachOpen {
		t.Fatal("A should open the attach prompt")
	}
	overlay := ansi.Strip(m.View())
	if !strings.Contains(overlay, "attach file") {
		t.Errorf("attach overlay should show its prompt, got:\n%s", overlay)
	}
	for _, r := range path {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	if m.attachOpen {
		t.Error("enter with a valid path should close the attach prompt")
	}
	if len(m.pendingFiles) != 1 || m.pendingFiles[0] != path {
		t.Errorf("valid path should be staged, got %v", m.pendingFiles)
	}
	if !m.insert {
		t.Error("staging a file should enter INSERT to write a comment")
	}

	m2 := newSized()
	m2 = Key(m2, "A")
	for _, r := range "/no/such/file-really" {
		m2 = Key(m2, string(r))
	}
	m2 = Key(m2, "enter")
	if m2.attachOpen {
		t.Error("a bad path should still close the prompt")
	}
	if len(m2.pendingFiles) != 0 {
		t.Errorf("a non-existent path should not be staged, got %v", m2.pendingFiles)
	}
	if m2.loadErr == nil || !strings.Contains(m2.loadErr.Error(), "no such file") {
		t.Errorf("a bad path should flash an error, got %v", m2.loadErr)
	}

	m3 := newSized()
	m3 = Key(m3, "A")
	m3 = Key(m3, "esc")
	if m3.attachOpen {
		t.Error("esc should close the attach prompt")
	}
	if len(m3.pendingFiles) != 0 {
		t.Error("esc should not stage anything")
	}
}

// ── overlays that were never rendered through View() ─────────────────────

// TestFindOverlayShowsPromptAndQuery: overlayFind (0% covered) renders its
// label and the query typed so far.
func TestFindOverlayShowsPromptAndQuery(t *testing.T) {
	m := newSized()
	m = Key(m, "/")
	for _, r := range "dirty" {
		m = Key(m, string(r))
	}
	overlay := ansi.Strip(m.View())
	if !strings.Contains(overlay, "find in channel") {
		t.Errorf("find overlay missing its label, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "dirty") {
		t.Errorf("find overlay should show the typed query, got:\n%s", overlay)
	}
}

// TestStatusTextOverlayShowsPromptAndInput: overlayStatusText (0% covered)
// renders its label and the text typed so far.
func TestStatusTextOverlayShowsPromptAndInput(t *testing.T) {
	m := newSized()
	m, _ = flNextModel(m, tea.KeyMsg{Type: tea.KeyCtrlK})
	m = Key(m, "status message")
	m, _ = flNextModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	for _, r := range "brb" {
		m = Key(m, string(r))
	}
	overlay := ansi.Strip(m.View())
	if !strings.Contains(overlay, "status message") {
		t.Errorf("status-text overlay missing its label, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "brb") {
		t.Errorf("status-text overlay should show the typed text, got:\n%s", overlay)
	}
}

// TestSuggestOverlayRendersPopup: overlaySuggest (0% covered) lists the live
// mention suggestions above the composer while typing "@ad".
func TestSuggestOverlayRendersPopup(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	for _, r := range "hey @ad" {
		m = Key(m, string(r))
	}
	if !m.suggestActive() {
		t.Fatal("@ad should activate the suggestion popup")
	}
	overlay := ansi.Strip(m.View())
	if !strings.Contains(overlay, "ada") {
		t.Errorf("suggest overlay should list the matching handle, got:\n%s", overlay)
	}
}

// TestPaletteOverlayRendersItems: overlayPalette (0% covered) lists commands
// once ctrl+k opens it.
func TestPaletteOverlayRendersItems(t *testing.T) {
	m := newSized()
	m, _ = flNextModel(m, tea.KeyMsg{Type: tea.KeyCtrlK})
	// The full list is windowed to paletteMaxRows and "Cycle theme" sorts well
	// past the channel/DM entries at the top, so filter down to it first —
	// otherwise it's simply scrolled out of the visible box.
	m = Key(m, "cycle theme")
	overlay := ansi.Strip(m.View())
	if !strings.Contains(overlay, "Cycle theme") {
		t.Errorf("palette overlay should list its commands, got:\n%s", overlay)
	}
}

// ── mouse-drag selection highlight ────────────────────────────────────────

// TestHighlightVisibleAppliesSelectionStyle: an active drag selection must
// restyle exactly the selected row's raw rendering (the highlight), while
// leaving its visible text unchanged — compared row-for-row against the
// identical model with no selection, rather than searching the whole frame
// for the Fg-on-SelBg escape (which also appears, unrelated to text
// selection, on the sidebar's active-conversation row).
func TestHighlightVisibleAppliesSelectionStyle(t *testing.T) {
	base := func() Model { return WithSize(newTest(), 120, 40) }

	mOff := base()
	lines, top, _, _ := mOff.msgViewport()
	L := -1
	for i, l := range lines {
		if lipgloss.Width(l) >= 5 {
			L = i
			break
		}
	}
	if L < 0 {
		t.Skip("no rendered line wide enough for a selection")
	}

	mOn := base()
	mOn.selAnchor = cell{L, 1}
	mOn.selHead = cell{L, 4}
	mOn.selActive = true
	mOn.selPane = focusMessages

	// y0 in cellAt is 2 (title bar + pane top border); the message pane isn't
	// scrolled here (a handful of short messages), so top should be 0.
	row := 2 + (L - top)
	frameOff := strings.Split(mOff.View(), "\n")
	frameOn := strings.Split(mOn.View(), "\n")
	if row >= len(frameOff) || row >= len(frameOn) {
		t.Fatalf("computed row %d out of range (off=%d on=%d lines)", row, len(frameOff), len(frameOn))
	}
	if frameOff[row] == frameOn[row] {
		t.Error("the selected row's raw rendering should change once highlighted")
	}
	if ansi.Strip(frameOff[row]) != ansi.Strip(frameOn[row]) {
		t.Errorf("the selected row's visible text should be unchanged — only styling should differ\noff: %q\non:  %q",
			ansi.Strip(frameOff[row]), ansi.Strip(frameOn[row]))
	}
}

// ── gg / G in every pane (app_test.go's TestMessageNavigation only covers
//    the messages pane) ────────────────────────────────────────────────────

// TestJumpTopBottomSidebarAndThread: gg/G in the sidebar move to the first
// and last selectable row (never a header); in an open thread they move to
// the first and last reply.
func TestJumpTopBottomSidebarAndThread(t *testing.T) {
	m := newSized()
	m.focus = focusSidebar
	m.sideSel = 3
	m.jumpTop()
	items := m.sideItems()
	if m.sideSel >= len(items) || items[m.sideSel].Header {
		t.Fatalf("jumpTop in the sidebar should land on a conversation row, sideSel=%d", m.sideSel)
	}
	first := m.sideSel
	m.jumpBottom()
	if m.sideSel >= len(items) || items[m.sideSel].Header {
		t.Fatalf("jumpBottom in the sidebar should land on a conversation row, sideSel=%d", m.sideSel)
	}
	if m.sideSel == first {
		t.Error("jumpBottom should move away from jumpTop's position (multiple conversations exist)")
	}

	m.focus = focusMessages // 't' only opens a thread from the messages pane
	m.msgSel = 2            // e3, which has 3 replies in the mock
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}
	root, ok := m.threadRoot()
	if !ok || len(root.Replies) < 2 {
		t.Fatal("e3 should have at least 2 replies")
	}
	m.focus = focusThread
	m.threadSel = 1
	m.jumpTop()
	if m.threadSel != 0 {
		t.Errorf("jumpTop in an open thread should select reply 0, got %d", m.threadSel)
	}
	m.jumpBottom()
	if want := len(root.Replies) - 1; m.threadSel != want {
		t.Errorf("jumpBottom in an open thread should select the last reply %d, got %d", want, m.threadSel)
	}
}

// TestMoveSelInThread: j/k while the thread pane is focused move threadSel
// (the messages-pane branch is already covered elsewhere; sidebar's is
// covered by TestHideKeepsSideSelValid and friends).
func TestMoveSelInThread(t *testing.T) {
	m := newSized()
	m.msgSel = 2 // e3, has replies
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}
	m.focus = focusThread
	m.threadSel = 0
	m = Key(m, "j")
	if m.threadSel != 1 {
		t.Errorf("j in the thread pane should advance threadSel, got %d", m.threadSel)
	}
	m = Key(m, "k")
	if m.threadSel != 0 {
		t.Errorf("k in the thread pane should retreat threadSel, got %d", m.threadSel)
	}
}

// ── applySentReply: failure rolls back the optimistic reply ──────────────

// TestApplySentReplyErrorRollsBack: a failed reply send removes the pending
// copy and restores the draft — applySentReply's error branch, which
// TestThreadReply (app_test.go) never exercises (it only sends successfully).
func TestApplySentReplyErrorRollsBack(t *testing.T) {
	m := newSized()
	root := data.Message{ID: "root2", UserID: "ada", Text: "q"}
	m.messages[m.activeID] = append(m.messages[m.activeID], root)
	m.threadRootID = "root2"
	pendingID := pendingPrefix + "99"
	m.eachRootMsg("root2", func(r *data.Message) {
		r.Replies = append(r.Replies, data.Reply{ID: pendingID, UserID: "me", Text: "will fail"})
	})
	cmd := m.applySentReply(sentReplyMsg{convID: m.activeID, rootID: "root2", pendingID: pendingID, text: "will fail", err: errFlSendReply("boom")})
	if cmd == nil {
		t.Fatal("a failed reply send should flash an error (non-nil cmd)")
	}
	if m.loadErr == nil {
		t.Error("a failed reply send should set loadErr")
	}
	got, _ := m.eachReplyRoot("root2")
	for _, r := range got.Replies {
		if r.ID == pendingID {
			t.Error("the failed pending reply should have been rolled back")
		}
	}
	if m.threadDraft.Value() != "will fail" {
		t.Errorf("failed reply should restore the thread draft, got %q", m.threadDraft.Value())
	}
}

type errFlSendReply string

func (e errFlSendReply) Error() string { return string(e) }

// ── settings: h/l change a setting, ctrl+c quits from inside it ──────────

// TestSettingsKeyHLNavigationAndQuit: 'l' and 'h' cycle the selected row in
// opposite directions (settingsKey's branches, not just cycleSetting called
// directly as TestSettingsPanel does), and ctrl+c quits.
func TestSettingsKeyHLNavigationAndQuit(t *testing.T) {
	m := newSized()
	m.openSettings()
	m.settingsSel = 1 // accent
	before := m.prefs.Accent
	m = Key(m, "l")
	afterL := m.prefs.Accent
	if afterL == before {
		t.Fatal("l should cycle the accent forward")
	}
	m = Key(m, "h")
	if m.prefs.Accent != before {
		t.Errorf("h should cycle back to the original accent, got %q want %q", m.prefs.Accent, before)
	}
	_, cmd := flNextModel(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("ctrl+c from settings should quit (non-nil cmd)")
	}
}

// ── picker: arrow navigation and the free-pick fallback ──────────────────

// TestPickerArrowNavigation: down/up move the picker's selected index within
// an open picker with several items.
func TestPickerArrowNavigation(t *testing.T) {
	m := newSized()
	m = Key(m, "a") // reaction picker: many curated items
	if !m.picker.open {
		t.Fatal("a should open the reaction picker")
	}
	start := m.picker.index
	m = Key(m, "down")
	if m.picker.index != start+1 {
		t.Errorf("down should advance the picker index, got %d want %d", m.picker.index, start+1)
	}
	m = Key(m, "up")
	if m.picker.index != start {
		t.Errorf("up should retreat the picker index, got %d want %d", m.picker.index, start)
	}
}

// TestPickerFreePickAcceptsUnlistedEmoji: the reaction picker's free=true
// mode lets a query with no matching item still be picked as-is (any Slack
// emoji name works, not just the curated list).
func TestPickerFreePickAcceptsUnlistedEmoji(t *testing.T) {
	m := newSized()
	m.msgSel = 0
	m = Key(m, "a")
	for _, r := range "zzz_not_curated" {
		m = Key(m, string(r))
	}
	if len(m.pickerVisible()) != 0 {
		t.Fatalf("test needs a query matching nothing in the curated list, got %d matches", len(m.pickerVisible()))
	}
	m, cmd := flNextModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.picker.open {
		t.Error("enter on a free pick should close the picker")
	}
	if cmd == nil {
		t.Fatal("a free pick should still run the react command")
	}
	rm, ok := cmd().(reactMsg)
	if !ok || rm.name != "zzz_not_curated" {
		t.Fatalf("free pick should react with the typed name, got %T %+v", rm, rm)
	}
}

// ── settings: the rows TestSettingsPanel/TestStatusChangePushesPresence
//    don't touch (density, group DMs, notifications, key hints) ──────────

// TestSettingsCycleDensityGroupDMsNotifyHints exercises cycleSetting's
// remaining rows directly, each against an independent, observable effect.
func TestSettingsCycleDensityGroupDMsNotifyHints(t *testing.T) {
	m := newSized()

	// Both directions against named, independently-specified values. Comparing
	// prefs.Density to m.density.String() would compare two outputs of the same
	// operation: a regression writing an unsupported density into both would
	// still pass.
	m.settingsSel = 2 // density
	m.density = theme.ParseDensity("comfortable")
	m.prefs.Density = "comfortable"
	m.cycleSetting(1)
	if got := m.density.String(); got != "compact" {
		t.Errorf("comfortable + 1 = %q, want compact", got)
	}
	if m.prefs.Density != "compact" {
		t.Errorf("prefs.Density = %q, want compact persisted alongside", m.prefs.Density)
	}
	m.cycleSetting(-1)
	if got := m.density.String(); got != "comfortable" {
		t.Errorf("compact - 1 = %q, want comfortable", got)
	}
	if m.prefs.Density != "comfortable" {
		t.Errorf("prefs.Density = %q, want comfortable", m.prefs.Density)
	}

	m.settingsSel = 4 // group DMs — the mock isn't a *source.Slack, so no reload cmd
	if m.prefs.GroupDMs {
		t.Fatal("test needs GroupDMs to start false")
	}
	cmd := m.cycleSetting(1)
	if !m.prefs.GroupDMs {
		t.Error("cycling group DMs should flip the preference regardless of source type")
	}
	if cmd != nil {
		t.Error("with a mock source (not *source.Slack), no reload command should be returned")
	}

	m.settingsSel = 5 // notifications
	if !m.prefs.Notifications() {
		t.Fatal("test needs notifications to start on")
	}
	m.cycleSetting(1)
	if m.prefs.Notifications() {
		t.Error("cycling notifications should turn them off")
	}
	m.cycleSetting(1)
	if !m.prefs.Notifications() {
		t.Error("cycling again should turn them back on")
	}

	m.settingsSel = 6 // key hints
	beforeHints := m.showHints
	m.cycleSetting(1)
	if m.showHints == beforeHints {
		t.Error("cycling key hints should toggle showHints")
	}
}

// TestAccentChipStatusNameTitleCase: the small settings-overlay label
// helpers, tested directly with hand-written expectations from the
// documented mapping (not by re-deriving them from the same switch).
func TestAccentChipStatusNameTitleCase(t *testing.T) {
	p := newSized().pal
	seen := map[string]bool{}
	for _, val := range []string{"cyan", "green", "purple", "orange", "magenta", "blue"} {
		out := accentChip(p, val)
		if seen[out] {
			t.Errorf("accentChip(%q) collides with an earlier color", val)
		}
		seen[out] = true
	}
	want := lipgloss.NewStyle().Foreground(p.Accent).Render("██")
	if got := accentChip(p, "not-a-real-accent"); got != want {
		t.Errorf("an unrecognized accent should fall back to the palette's own Accent, got %q want %q", got, want)
	}

	if got := statusName("away"); got != "Away" {
		t.Errorf(`statusName("away") = %q, want "Away"`, got)
	}
	if got := statusName("dnd"); got != "Do not disturb" {
		t.Errorf(`statusName("dnd") = %q, want "Do not disturb"`, got)
	}
	if got := statusName("weird"); got != "weird" {
		t.Errorf(`statusName("weird") should pass through unknown values, got %q`, got)
	}

	if got := titleCase("dnd"); got != "Dnd" {
		t.Errorf(`titleCase("dnd") = %q, want "Dnd"`, got)
	}
	if got := titleCase(""); got != "" {
		t.Errorf(`titleCase("") = %q, want ""`, got)
	}
}

// TestNotifyLabelBranches: "off" and "unavailable" are both reachable
// hermetically — testenv.Pin empties PATH, so notify.Available() is always
// false here, which means the third branch ("Mentions & DMs", a real
// notifier present) genuinely cannot be exercised without a real notify-send
// on PATH; that branch is left untested rather than faked.
func TestNotifyLabelBranches(t *testing.T) {
	if got := notifyLabel(config.Prefs{Notify: config.NotifyOff}); got != "Off" {
		t.Errorf(`notifyLabel(off) = %q, want "Off"`, got)
	}
	if got := notifyLabel(config.Prefs{Notify: config.NotifyMentions}); got != "Unavailable" {
		t.Errorf(`notifyLabel(on, no notifier on PATH) = %q, want "Unavailable"`, got)
	}
}

// ── mouse release: clipboard copy, divider persistence ────────────────────

// TestMouseReleaseWithRealSelectionCopiesToClipboard: releasing after a drag
// that actually spans text must write it via the clipboard seam and show the
// "copied" toast — TestMousePressMotionUpdatesSelection (select_test.go)
// only exercises the degenerate (empty) release.
func TestMouseReleaseWithRealSelectionCopiesToClipboard(t *testing.T) {
	clipboardStub = ""
	m := WithSize(newTest(), 120, 40)
	lines, _, _, innerH := m.msgViewport()
	if innerH <= 0 || len(lines) == 0 {
		t.Skip("no renderable message content")
	}
	x0, y0 := sidebarWidth+2, 2
	next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x0, Y: y0})
	m = next.(Model)
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, X: x0 + 3, Y: y0})
	m = next.(Model)
	next, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: x0 + 3, Y: y0})
	m = next.(Model)
	if clipboardStub == "" {
		t.Fatal("releasing a real span should copy text via the clipboard seam")
	}
	if m.copyToast != "copied" {
		t.Errorf("copyToast = %q, want \"copied\"", m.copyToast)
	}
	if cmd == nil {
		t.Error("a real copy should schedule the toast's auto-dismiss")
	}
}

// TestDividerReleasePersistsWhenEnabled: releasing a divider drag with
// persistState on must write the new thread width to prefs.json —
// TestDividerResizeNotPersistInTest (thread_test.go) only covers the
// persistState=false (test-mode) branch.
func TestDividerReleasePersistsWhenEnabled(t *testing.T) {
	isolateConfigDir(t)
	m := WithSize(newTest(), 200, 30)
	m.msgSel = 2
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}
	m.resizing = true
	m.persistState = true
	newWidth := m.clampThreadWidth(50)
	m.threadWidth = newWidth
	_, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, X: m.width - newWidth, Y: 5})
	if cmd == nil {
		t.Fatal("releasing with persistState on should return a save command")
	}
	cmd() // runs config.Save synchronously against the isolated dir
	loaded, _ := config.Load()
	if loaded.ThreadWidth != newWidth {
		t.Errorf("persisted ThreadWidth = %d, want %d", loaded.ThreadWidth, newWidth)
	}
}

// ── small pure helpers, table-tested directly ─────────────────────────────

// TestNormalizeDropPathVariants: file:// URLs (with and without a host, with
// percent-encoding) and ~ expansion, against hand-computed expectations.
func TestNormalizeDropPathVariants(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	cases := []struct{ in, want string }{
		{"", ""},
		{"/plain/path", "/plain/path"},
		{"file:///tmp/a%20b.txt", "/tmp/a b.txt"},     // no host: leading "/" kept, %20 decoded
		{"file://host/tmp/file.txt", "/tmp/file.txt"}, // host stripped
		{"~", home}, // bare ~ → home
		{"~/docs/a.txt", filepath.Join(home, "docs/a.txt")},
	}
	for _, c := range cases {
		if got := normalizeDropPath(c.in); got != c.want {
			t.Errorf("normalizeDropPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLocNameChannelVsDM: the composer placeholder prefix is "#" for
// channels, "@" for DMs.
func TestLocNameChannelVsDM(t *testing.T) {
	m := newSized()
	ch, _ := m.ws.Conversation("engineering")
	if got := m.locName(ch); got != "#engineering" {
		t.Errorf("locName(channel) = %q, want %q", got, "#engineering")
	}
	dm, _ := m.ws.Conversation("dm_ada")
	if got := m.locName(dm); got != "@ada.k" {
		t.Errorf("locName(dm) = %q, want %q", got, "@ada.k")
	}
}

// TestFindCtrlCQuits: ctrl+c while the find prompt is open still quits, like
// every other overlay's ctrl+c path.
func TestFindCtrlCQuits(t *testing.T) {
	m := newSized()
	m = Key(m, "/")
	if !m.findOpen {
		t.Fatal("/ should open find")
	}
	_, cmd := flNextModel(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("ctrl+c from the find prompt should quit (non-nil cmd)")
	}
}

// TestPersistStateCmdBothBranches: persistStateCmd is a no-op when
// persistState is false (the case in every other test in this suite) and
// actually writes state.json when it's true.
func TestPersistStateCmdBothBranches(t *testing.T) {
	m := newSized()
	m.persistState = false
	if cmd := m.persistStateCmd(); cmd != nil {
		t.Error("persistStateCmd with persistState=false should return nil")
	}

	isolateConfigDir(t)
	m.persistState = true
	m.draft.SetValue("wip")
	cmd := m.persistStateCmd()
	if cmd == nil {
		t.Fatal("persistStateCmd with persistState=true should return a command")
	}
	cmd()
	st, err := config.LoadState()
	if err != nil {
		t.Fatalf("state should have been saved: %v", err)
	}
	if st.Drafts[m.activeID] != "wip" {
		t.Errorf("saved draft = %q, want %q", st.Drafts[m.activeID], "wip")
	}
}

// TestApplyJoinableErrorClosesPickerAndFlashes: a failed Joinable fetch must
// close the (now-useless) empty picker and surface the error, rather than
// leaving an open picker with nothing to show.
func TestApplyJoinableErrorClosesPickerAndFlashes(t *testing.T) {
	m := newSized()
	m.picker = pickerState{open: true, kind: "join"}
	cmd := m.applyJoinable(joinableMsg{err: maErr("joinable failed")})
	if cmd == nil {
		t.Fatal("a failed Joinable fetch should flash (non-nil cmd)")
	}
	if m.picker.open {
		t.Error("the picker should close on a failed fetch")
	}
	if m.loadErr == nil || !strings.Contains(m.loadErr.Error(), "joinable failed") {
		t.Errorf("error should surface in loadErr, got %v", m.loadErr)
	}
}

// TestSelectedMsgAndReplyFalseBranches: both accessors report ok=false when
// the cursor isn't actually in their pane — selectedMsg outside the messages
// pane, selectedReply when no thread is open at all.
func TestSelectedMsgAndReplyFalseBranches(t *testing.T) {
	m := newSized()
	m.focus = focusSidebar
	if _, ok := m.selectedMsg(); ok {
		t.Error("selectedMsg should be false when the sidebar is focused")
	}

	m2 := newSized()
	m2.focus = focusThread // no thread actually open
	if _, ok := m2.selectedReply(); ok {
		t.Error("selectedReply should be false with no open thread")
	}
}
