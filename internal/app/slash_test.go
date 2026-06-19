package app

import (
	"strings"
	"testing"

	"github.com/kurenn/slack-tui/internal/source"
)

// TestSlashUnknownNotSent: an unrecognised /command clears the draft, flashes,
// and is never sent as a message.
func TestSlashUnknownNotSent(t *testing.T) {
	m := newSized()
	before := len(m.curMsgs())
	m.draft.SetValue("/giphy cat")

	cmd, done := m.runSlash("/giphy cat")

	if !done {
		t.Fatal("runSlash should report done=true for a command-shaped input")
	}
	if m.draft.Value() != "" {
		t.Errorf("draft should be cleared, got %q", m.draft.Value())
	}
	if m.loadErr == nil {
		t.Error("runSlash should flash an error for an unknown command")
	}
	// The returned cmd is a tea.Tick (error auto-clear), not a send — message
	// count must not change.
	_ = cmd
	if len(m.curMsgs()) != before {
		t.Errorf("unknown slash command should not append a message (got %d, want %d)", len(m.curMsgs()), before)
	}
}

// TestSlashPathSendsLiterally: text that looks like a file path (/usr/local/bin)
// must not be treated as a command.
func TestSlashPathSendsLiterally(t *testing.T) {
	m := newSized()
	_, done := m.runSlash("/usr/local/bin")
	if done {
		t.Error("runSlash should return done=false for a non-command /path so it sends as normal text")
	}
}

// TestSlashShrug: /shrug hi transforms the draft to contain the shrug kaomoji
// and appends the message.
func TestSlashShrug(t *testing.T) {
	m := newSized()
	m.draft.SetValue("/shrug hi")
	before := len(m.curMsgs())

	_, done := m.runSlash("/shrug hi")

	if !done {
		t.Fatal("runSlash should return done=true for /shrug")
	}
	// sendMessage should have run (the draft was mutated before the call).
	if len(m.curMsgs()) != before+1 {
		t.Fatalf("shrug should append a message (got %d, want %d)", len(m.curMsgs()), before+1)
	}
	last := m.curMsgs()[len(m.curMsgs())-1].Text
	if !strings.Contains(last, "¯") {
		t.Errorf("shrug message should contain the kaomoji, got %q", last)
	}
}

// TestSlashDM: /dm @ada resolves the handle, calls OpenDM, and feeding the
// result through Update opens the DM and records the call in the mock.
func TestSlashDM(t *testing.T) {
	m := newSized()
	// "ada" is a real handle in data.Mock().
	cmd, done := m.runSlash("/dm @ada")
	if !done {
		t.Fatal("runSlash should return done=true for /dm")
	}
	if m.draft.Value() != "" {
		t.Error("draft should be cleared after /dm")
	}
	if cmd == nil {
		t.Fatal("runSlash /dm should return a non-nil cmd")
	}

	// Execute the cmd — it calls src.OpenDM and returns dmOpenedMsg.
	result := cmd()
	opened, ok := result.(dmOpenedMsg)
	if !ok {
		t.Fatalf("cmd should return dmOpenedMsg, got %T", result)
	}
	if opened.err != nil {
		t.Fatalf("OpenDM should succeed on the mock, got %v", opened.err)
	}

	// Feed the message through Update.
	next, _ := m.Update(opened)
	m = next.(Model)

	// Mock should have recorded the call.
	mock := m.src.(*source.Mock)
	if len(mock.Opened()) == 0 {
		t.Fatal("src.OpenDM was not called")
	}
	if mock.Opened()[0] != "ada" {
		t.Errorf("OpenDM called with %q, want ada", mock.Opened()[0])
	}
	// The DM should now be active.
	if m.activeID != opened.conv.ID {
		t.Errorf("activeID = %q, want %q", m.activeID, opened.conv.ID)
	}
}

// TestSlashDnd: /dnd 45 records 45 minutes in the mock and sets myStatus to "dnd".
// /dnd with no argument defaults to 120.
func TestSlashDnd(t *testing.T) {
	m := newSized()

	// /dnd 45
	cmd, done := m.runSlash("/dnd 45")
	if !done {
		t.Fatal("runSlash should return done=true for /dnd")
	}
	if cmd == nil {
		t.Fatal("runSlash /dnd should return a non-nil cmd")
	}
	// Execute the cmd — it calls src.Snooze and returns presenceMsg.
	_ = cmd()
	mock := m.src.(*source.Mock)
	if len(mock.Snoozed()) == 0 {
		t.Fatal("Snooze was not called")
	}
	if mock.Snoozed()[0] != 45 {
		t.Errorf("Snooze called with %d, want 45", mock.Snoozed()[0])
	}
	if m.myStatus != "dnd" {
		t.Errorf("myStatus = %q, want \"dnd\"", m.myStatus)
	}

	// /dnd with no arg defaults to 120
	m2 := newSized()
	cmd2, done2 := m2.runSlash("/dnd")
	if !done2 {
		t.Fatal("runSlash should return done=true for /dnd (no arg)")
	}
	_ = cmd2()
	mock2 := m2.src.(*source.Mock)
	if len(mock2.Snoozed()) == 0 {
		t.Fatal("Snooze was not called for /dnd with no arg")
	}
	if mock2.Snoozed()[0] != 120 {
		t.Errorf("Snooze default = %d, want 120", mock2.Snoozed()[0])
	}
}

// TestSlashShrugNestedNotExecuted: /shrug /leave should NOT execute /leave —
// it must send a message containing "/leave" and the kaomoji.
func TestSlashShrugNestedNotExecuted(t *testing.T) {
	m := newSized()
	// Confirm we start in a channel.
	var inChannels bool
	for _, c := range m.ws.Channels {
		if c.ID == m.activeID {
			inChannels = true
			break
		}
	}
	if !inChannels {
		t.Skipf("active conversation %q is not a channel", m.activeID)
	}
	beforeLen := len(m.ws.Channels)

	cmd, done := m.runSlash("/shrug /leave")
	if !done {
		t.Fatal("runSlash should return done=true for /shrug")
	}

	// The cmd is a send (closure), not a leaveCmd. Execute it to resolve.
	if cmd != nil {
		_ = cmd()
	}

	// Channel list must be unchanged — /leave must not have run.
	if len(m.ws.Channels) != beforeLen {
		t.Errorf("channel count changed from %d to %d — /leave was incorrectly executed", beforeLen, len(m.ws.Channels))
	}

	// The appended message text must contain both "/leave" and the kaomoji.
	msgs := m.curMsgs()
	if len(msgs) == 0 {
		t.Fatal("no message was appended by /shrug")
	}
	last := msgs[len(msgs)-1].Text
	if !strings.Contains(last, "/leave") {
		t.Errorf("message %q should contain \"/leave\"", last)
	}
	if !strings.Contains(last, "¯") {
		t.Errorf("message %q should contain the shrug kaomoji", last)
	}
}

// TestSlashLeave: /leave on the active channel removes it from ws.Channels and
// records the call on the mock.
func TestSlashLeave(t *testing.T) {
	m := newSized()
	// Make sure the active conversation is a channel (not a DM).
	target := m.activeID
	var inChannels bool
	for _, c := range m.ws.Channels {
		if c.ID == target {
			inChannels = true
			break
		}
	}
	if !inChannels {
		t.Skipf("active conversation %q is not a channel — test requires a channel", target)
	}
	beforeLen := len(m.ws.Channels)

	cmd, done := m.runSlash("/leave")
	if !done {
		t.Fatal("runSlash should return done=true for /leave")
	}
	if cmd == nil {
		t.Fatal("runSlash /leave should return a non-nil cmd")
	}

	// Execute the cmd — returns leftMsg.
	result := cmd()
	left, ok := result.(leftMsg)
	if !ok {
		t.Fatalf("cmd should return leftMsg, got %T", result)
	}
	if left.err != nil {
		t.Fatalf("Leave should succeed on the mock, got %v", left.err)
	}

	// Feed through Update.
	next, _ := m.Update(left)
	m = next.(Model)

	// Mock should have recorded the call.
	mock := m.src.(*source.Mock)
	if len(mock.Left()) == 0 {
		t.Fatal("src.Leave was not called")
	}
	if mock.Left()[0] != target {
		t.Errorf("Leave called with %q, want %q", mock.Left()[0], target)
	}
	// The channel should be gone.
	if len(m.ws.Channels) != beforeLen-1 {
		t.Errorf("channel count = %d, want %d", len(m.ws.Channels), beforeLen-1)
	}
	for _, c := range m.ws.Channels {
		if c.ID == target {
			t.Errorf("left channel %q still in ws.Channels", target)
		}
	}
}
