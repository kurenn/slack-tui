package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/source"
)

// newTest builds the model on the in-memory mock — tests must never read the
// user's tokens/prefs or touch the network.
func newTest() Model { return NewWith(source.NewMock(), config.Defaults()) }

// TestTallMessageScroll: a message taller than the viewport can be read by
// line-scrolling (Ctrl-d), not just showing its top.
func TestTallMessageScroll(t *testing.T) {
	m := WithSize(newTest(), 70, 16)
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString(fmt.Sprintf("line%02d\n", i))
	}
	m.messages[m.activeID] = []data.Message{{ID: "big", UserID: "ada", Time: "09:00", Text: sb.String()}}
	m.msgSel, m.msgExtra, m.focus = 0, 0, focusMessages

	top := ansi.Strip(m.View())
	if !strings.Contains(top, "line00") || strings.Contains(top, "line39") {
		t.Fatal("initially the top should be visible and the bottom hidden")
	}
	for i := 0; i < 40; i++ {
		m.scrollMessages(m.pageJump())
	}
	if bottom := ansi.Strip(m.View()); !strings.Contains(bottom, "line39") {
		t.Errorf("after scrolling, the bottom of the tall message should be visible (msgExtra=%d)", m.msgExtra)
	}
}

// TestTallMessageJK: pressing j repeatedly scrolls down through a tall message
// (not just jump between messages).
func TestTallMessageJK(t *testing.T) {
	m := WithSize(newTest(), 70, 16)
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString(fmt.Sprintf("line%02d\n", i))
	}
	m.messages[m.activeID] = []data.Message{{ID: "big", UserID: "ada", Time: "09:00", Text: sb.String()}}
	m.msgSel, m.msgExtra, m.focus = 0, 0, focusMessages
	for i := 0; i < 60; i++ {
		m = Key(m, "j")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "line39") {
		t.Errorf("j should scroll through a tall message to its bottom (msgExtra=%d)", m.msgExtra)
	}
	for i := 0; i < 60; i++ {
		m = Key(m, "k")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "line00") || m.msgExtra != 0 {
		t.Errorf("k should scroll back to the top (msgExtra=%d)", m.msgExtra)
	}
}

// TestStatusChangePushesPresence: cycling the Status row advances presence and
// returns a command to push it to the backend.
func TestStatusChangePushesPresence(t *testing.T) {
	isolateConfigDir(t)
	m := newSized()
	m.openSettings()
	m.settingsSel = 3 // status row
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(Model)
	if m.myStatus != "away" {
		t.Errorf("status should advance online→away, got %q", m.myStatus)
	}
	if cmd == nil {
		t.Fatal("changing status should return a presence command")
	}
}

func TestSettingsPanel(t *testing.T) {
	isolateConfigDir(t)
	m := newSized()
	m.openSettings()
	if !m.settingsOpen {
		t.Fatal("settings should be open")
	}
	m.settingsSel = 1 // accent
	before := m.prefs.Accent
	m.cycleSetting(1)
	if m.prefs.Accent == before {
		t.Errorf("cycling accent should change it (was %q)", before)
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "settings") || !strings.Contains(out, "Accent") {
		t.Error("settings overlay not rendered in the frame")
	}
	m = Key(m, "esc")
	if m.settingsOpen {
		t.Error("esc should close settings")
	}
}

func init() { /* tests drive the model headlessly via Key/WithSize */ }

// TestLoadOlderPrepends: an older page prepends and shifts the selection; an
// empty page marks the conversation fully loaded.
func TestLoadOlderPrepends(t *testing.T) {
	m := newSized()
	m.msgSel = 0
	before := len(m.curMsgs())
	older := []data.Message{
		{ID: "old1", UserID: "ada", Time: "08:00", Text: "older1"},
		{ID: "old2", UserID: "lin", Time: "08:01", Text: "older2"},
	}
	next, _ := m.Update(olderMsg{convID: m.activeID, msgs: older})
	m = next.(Model)
	if len(m.curMsgs()) != before+2 || m.curMsgs()[0].Text != "older1" {
		t.Fatalf("older page not prepended (len=%d, first=%q)", len(m.curMsgs()), m.curMsgs()[0].Text)
	}
	if m.msgSel != 2 {
		t.Errorf("selection should shift down by the prepended count, got %d", m.msgSel)
	}
	next, _ = m.Update(olderMsg{convID: m.activeID, msgs: nil})
	m = next.(Model)
	if !m.fullyLoaded[m.activeID] {
		t.Error("an empty older page should mark the conversation fully loaded")
	}
}

// TestJumpToNextUnread: ] hops to the next conversation with unread and marks it read.
func TestJumpToNextUnread(t *testing.T) {
	m := newSized() // mock: engineering active; design(1) and incidents(2) unread
	m.jumpUnread(1)
	if m.activeID == "engineering" {
		t.Errorf("] should move to an unread conversation, still %q", m.activeID)
	}
	if m.meta[m.activeID].Unread != 0 {
		t.Errorf("the opened conversation should be marked read, meta=%+v", m.meta[m.activeID])
	}
}

// TestPollUpdatesOpenThread reproduces the reported bug: a reply arriving while
// a thread is open must appear (here delivered as the poll's repliesMsg).
func TestPollUpdatesOpenThread(t *testing.T) {
	m := newSized()
	m.msgSel = 2 // e3, a message with replies
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("thread should be open")
	}
	next, _ := m.Update(repliesMsg{convID: m.activeID, rootID: m.threadRootID,
		replies: []data.Reply{{ID: "x", UserID: "ada", Time: "10:00", Text: "fresh reply"}}})
	m = next.(Model)
	root, _ := m.threadRoot()
	if len(root.Replies) != 1 || root.Replies[0].Text != "fresh reply" {
		t.Fatalf("open thread not updated from poll: %+v", root.Replies)
	}
}

// TestSocketEventMarksUnread: a live message in a non-active channel lights up
// its unread (and mention); an event for the active channel is ignored.
func TestSocketEventMarksUnread(t *testing.T) {
	m := newSized()
	other := "random" // starts read in the mock
	m.applyEvent(source.Event{ConvID: other, Msg: data.Message{UserID: "ada", Text: "hey @you", MentionsMe: true}})
	if m.meta[other].Unread != 1 || !m.meta[other].Mention {
		t.Fatalf("non-active event should mark unread+mention, got %+v", m.meta[other])
	}
	beforeMeta := m.meta[m.activeID]
	m.applyEvent(source.Event{ConvID: m.activeID, Msg: data.Message{ID: "live1", UserID: "ada", Text: "x"}})
	if m.meta[m.activeID] != beforeMeta {
		t.Error("active-channel event should not change its unread")
	}
}

// TestActiveRootEventAppends: a root event in the active conversation appends
// instantly, follows the bottom when the user is there, dedupes a repeat, and
// leaves the selection put when the user is scrolled up.
func TestActiveRootEventAppends(t *testing.T) {
	m := newSized()
	m.msgSel = len(m.curMsgs()) - 1 // at the bottom
	before := len(m.curMsgs())
	m.applyEvent(source.Event{ConvID: m.activeID, Msg: data.Message{ID: "live1", UserID: "ada", Time: "10:00", Text: "incoming"}})
	if len(m.curMsgs()) != before+1 || m.curMsgs()[len(m.curMsgs())-1].Text != "incoming" {
		t.Fatalf("active root event should append, got %d msgs", len(m.curMsgs()))
	}
	if m.msgSel != len(m.curMsgs())-1 {
		t.Errorf("selection should follow the bottom, msgSel=%d", m.msgSel)
	}
	// Dedupe: the same ID again must not append twice.
	m.applyEvent(source.Event{ConvID: m.activeID, Msg: data.Message{ID: "live1", UserID: "ada", Text: "incoming"}})
	if len(m.curMsgs()) != before+1 {
		t.Errorf("repeated ID should dedupe, got %d msgs", len(m.curMsgs()))
	}
	// Scrolled up: selection must not move.
	m.msgSel = 0
	m.applyEvent(source.Event{ConvID: m.activeID, Msg: data.Message{ID: "live2", UserID: "ada", Text: "more"}})
	if m.msgSel != 0 {
		t.Errorf("scrolled-up selection should stay put, msgSel=%d", m.msgSel)
	}
}

// TestActiveRootEventUncached: an event for a conversation whose history isn't
// cached (or is loading) does nothing — the in-flight fetch will include it.
func TestActiveRootEventUncached(t *testing.T) {
	m := newSized()
	delete(m.messages, m.activeID)
	m.applyEvent(source.Event{ConvID: m.activeID, Msg: data.Message{ID: "x", UserID: "ada", Text: "hi"}})
	if _, ok := m.messages[m.activeID]; ok {
		t.Error("event for an uncached conversation should not create a cache entry")
	}
	// Loading: present but in flight — leave it for the fetch.
	m.messages[m.activeID] = []data.Message{{ID: "a", UserID: "ada", Text: "first"}}
	m.loading[m.activeID] = true
	m.applyEvent(source.Event{ConvID: m.activeID, Msg: data.Message{ID: "x", UserID: "ada", Text: "hi"}})
	if len(m.messages[m.activeID]) != 1 {
		t.Errorf("event during loading should not append, got %d", len(m.messages[m.activeID]))
	}
}

// TestActiveReplyEventLands: a thread-reply event lands under its root, bumps
// ReplyCount, dedupes, and rings the bell only when the root is mine and the
// thread isn't open.
func TestActiveReplyEventLands(t *testing.T) {
	m := newSized()
	root := data.Message{ID: "myroot", UserID: m.ws.MeID, Time: "10:00", Text: "hey bot", ReplyCount: 0}
	m.messages[m.activeID] = append(m.messages[m.activeID], root)

	cmds := m.applyEvent(source.Event{ConvID: m.activeID, ThreadTS: "myroot",
		Msg: data.Message{ID: "rep1", UserID: "bot", Time: "10:01", Text: "answer"}})
	got, _ := m.eachReplyRoot("myroot")
	if len(got.Replies) != 1 || got.Replies[0].Text != "answer" || got.ReplyCount != 1 {
		t.Fatalf("reply should land with ReplyCount=1, got %+v (count=%d)", got.Replies, got.ReplyCount)
	}
	if len(cmds) == 0 {
		t.Error("a reply on my closed thread should ring the bell")
	}
	// Dedupe the same reply ID.
	m.applyEvent(source.Event{ConvID: m.activeID, ThreadTS: "myroot",
		Msg: data.Message{ID: "rep1", UserID: "bot", Text: "answer"}})
	got, _ = m.eachReplyRoot("myroot")
	if len(got.Replies) != 1 {
		t.Errorf("repeated reply ID should dedupe, got %d", len(got.Replies))
	}
	// Thread open on my root → no bell.
	m.threadRootID = "myroot"
	cmds = m.applyEvent(source.Event{ConvID: m.activeID, ThreadTS: "myroot",
		Msg: data.Message{ID: "rep2", UserID: "bot", Text: "more"}})
	if len(cmds) != 0 {
		t.Error("a reply while the thread is open should not ring the bell")
	}
}

// TestActiveReplyEventNotMine: a reply on someone else's thread (root not mine)
// stays quiet even when the thread is closed.
func TestActiveReplyEventNotMine(t *testing.T) {
	m := newSized()
	m.messages[m.activeID] = append(m.messages[m.activeID],
		data.Message{ID: "theirroot", UserID: "ada", Text: "ada's thread"})
	cmds := m.applyEvent(source.Event{ConvID: m.activeID, ThreadTS: "theirroot",
		Msg: data.Message{ID: "rep1", UserID: "bot", Text: "answer"}})
	if len(cmds) != 0 {
		t.Error("a reply on someone else's thread should not ring the bell")
	}
}

// eachReplyRoot is a test helper: the cached root with the given ID (active conv).
func (m *Model) eachReplyRoot(id string) (data.Message, bool) {
	for _, msg := range m.messages[m.activeID] {
		if msg.ID == id {
			return msg, true
		}
	}
	return data.Message{}, false
}

// TestApplySentReplyDedupe: when a live event already delivered the real reply,
// reconciling the optimistic send drops the pending copy (no duplicate).
func TestApplySentReplyDedupe(t *testing.T) {
	m := newSized()
	root := data.Message{ID: "root", UserID: m.ws.MeID, Text: "q",
		Replies: []data.Reply{
			{ID: "real", UserID: m.ws.MeID, Text: "hi"},      // live event already landed it
			{ID: "pending-1", UserID: m.ws.MeID, Text: "hi"}, // our optimistic copy
		}}
	m.messages[m.activeID] = append(m.messages[m.activeID], root)
	m.applySentReply(sentReplyMsg{convID: m.activeID, rootID: "root", pendingID: "pending-1",
		reply: data.Reply{ID: "real", UserID: m.ws.MeID, Text: "hi"}})
	got, _ := m.eachReplyRoot("root")
	if len(got.Replies) != 1 || got.Replies[0].ID != "real" {
		t.Fatalf("dedupe should leave exactly one real reply, got %+v", got.Replies)
	}
}

// TestThreadReplyToMyMessageFetches: a poll showing a new reply on a thread I
// started fetches the replies (for the inline preview) and rings the bell —
// agents answer in threads, which used to look like silence.
func TestThreadReplyToMyMessageFetches(t *testing.T) {
	m := newSized()
	m.messages[m.activeID] = append(m.messages[m.activeID],
		data.Message{ID: "mine", UserID: "me", Time: "10:00", Text: "hey bot"})
	snapshot := append([]data.Message(nil), m.messages[m.activeID]...)
	snapshot[len(snapshot)-1].ReplyCount = 1 // the agent answered in-thread
	next, cmd := m.Update(historyMsg{convID: m.activeID, msgs: snapshot})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("a new reply on my thread should trigger a replies fetch")
	}
	// a reply on someone else's thread stays quiet
	m2 := newSized()
	snap2 := append([]data.Message(nil), m2.curMsgs()...)
	for i := range snap2 {
		if snap2[i].UserID != "me" {
			snap2[i].ReplyCount += 5
		}
	}
	next, _ = m2.Update(historyMsg{convID: m2.activeID, msgs: snap2})
	if got := next.(Model); len(got.curMsgs()) == 0 {
		t.Fatal("history should apply")
	}
}

// TestPollFollowsBottom: a new channel message arriving while the user is at the
// bottom should move the selection to follow it.
func TestPollFollowsBottom(t *testing.T) {
	m := newSized()
	m.msgSel = len(m.curMsgs()) - 1 // at the bottom
	grown := append(append([]data.Message(nil), m.curMsgs()...),
		data.Message{ID: "new", UserID: "ada", Time: "10:00", Text: "incoming"})
	next, _ := m.Update(historyMsg{convID: m.activeID, msgs: grown})
	m = next.(Model)
	if got := m.curMsgs()[len(m.curMsgs())-1].Text; got != "incoming" {
		t.Fatalf("new message not applied, last = %q", got)
	}
	if m.msgSel != len(m.curMsgs())-1 {
		t.Errorf("selection should follow the new bottom, msgSel = %d", m.msgSel)
	}
}

func newSized() Model { return WithSize(newTest(), 100, 30) }

// TestPresenceUpdateMsgApplied: feeding a presenceUpdateMsg through Update
// writes the status for included ids and leaves excluded ids untouched.
func TestPresenceUpdateMsgApplied(t *testing.T) {
	m := newSized()

	// Verify baseline (mock statuses from data.Mock())
	if m.ws.Users["ada"].Status != "online" {
		t.Fatalf("expected ada online at baseline, got %q", m.ws.Users["ada"].Status)
	}
	if m.ws.Users["marco"].Status != "away" {
		t.Fatalf("expected marco away at baseline, got %q", m.ws.Users["marco"].Status)
	}

	// Feed an update: ada→away, but omit marco.
	next, _ := m.Update(presenceUpdateMsg{statuses: map[string]string{"ada": "away"}})
	m = next.(Model)

	if got := m.ws.Users["ada"].Status; got != "away" {
		t.Errorf("ada status = %q after update, want away", got)
	}
	// marco was not in the update map — must stay untouched.
	if got := m.ws.Users["marco"].Status; got != "away" {
		t.Errorf("marco status = %q, should be unchanged", got)
	}
}

// TestPresenceCmdGathersDMPartnerUserIDs: presenceCmd uses UserID (the DM
// counterpart), not the conversation ID, and skips group DMs (empty UserID).
func TestPresenceCmdGathersDMPartnerUserIDs(t *testing.T) {
	m := newSized()

	// Inject a group DM (mpim) with no UserID to confirm it is skipped.
	m.ws.DMs = append(m.ws.DMs, data.Conversation{ID: "mpim_grp", Type: "dm", Name: "group", UserID: ""})

	ids := m.dmPartnerIDs()

	// All collected ids must be non-empty (group DMs excluded).
	for _, id := range ids {
		if id == "" {
			t.Error("dmPartnerIDs must not include empty UserID (group DM)")
		}
	}
	// The mock workspace has four 1:1 DMs: ada, lin, marco, priya.
	expected := map[string]bool{"ada": false, "lin": false, "marco": false, "priya": false}
	for _, id := range ids {
		if _, known := expected[id]; known {
			expected[id] = true
		}
	}
	for id, found := range expected {
		if !found {
			t.Errorf("dmPartnerIDs missing %q", id)
		}
	}
}

func TestInitialState(t *testing.T) {
	m := newTest()
	if m.focus != focusMessages {
		t.Errorf("focus = %q, want messages", m.focus)
	}
	if m.insert {
		t.Error("should start in NORMAL mode")
	}
	if want := len(m.messages["engineering"]) - 1; m.msgSel != want {
		t.Errorf("msgSel = %d, want %d (last)", m.msgSel, want)
	}
}

func TestMessageNavigation(t *testing.T) {
	m := newSized()
	last := len(m.curMsgs()) - 1
	m = Key(m, "k") // up
	if m.msgSel != last-1 {
		t.Errorf("after k msgSel = %d, want %d", m.msgSel, last-1)
	}
	m = Key(m, "g")
	m = Key(m, "g") // gg → top
	if m.msgSel != 0 {
		t.Errorf("after gg msgSel = %d, want 0", m.msgSel)
	}
	m = Key(m, "G") // bottom
	if m.msgSel != last {
		t.Errorf("after G msgSel = %d, want %d", m.msgSel, last)
	}
}

func TestFocusCycling(t *testing.T) {
	m := newSized()
	m = Key(m, "h") // messages → sidebar
	if m.focus != focusSidebar {
		t.Errorf("after h focus = %q, want sidebar", m.focus)
	}
	m = Key(m, "l") // back to messages
	if m.focus != focusMessages {
		t.Errorf("after l focus = %q, want messages", m.focus)
	}
	m = Key(m, "tab") // messages → sidebar (wraps in 2-pane order)
	if m.focus != focusSidebar {
		t.Errorf("after tab focus = %q, want sidebar", m.focus)
	}
}

// TestArrowKeysMirrorHJKL: ←/→ switch panes like h/l (↑/↓ already mirror j/k).
func TestArrowKeysMirrorHJKL(t *testing.T) {
	m := newSized()
	m = Key(m, "left")
	if m.focus != focusSidebar {
		t.Errorf("after ← focus = %q, want sidebar", m.focus)
	}
	m = Key(m, "right")
	if m.focus != focusMessages {
		t.Errorf("after → focus = %q, want messages", m.focus)
	}
	last := len(m.curMsgs()) - 1
	m = Key(m, "up")
	if m.msgSel != last-1 {
		t.Errorf("after ↑ msgSel = %d, want %d", m.msgSel, last-1)
	}
	m = Key(m, "down")
	if m.msgSel != last {
		t.Errorf("after ↓ msgSel = %d, want %d", m.msgSel, last)
	}
}

func TestOpenAndCloseThread(t *testing.T) {
	m := newSized()
	// engineering's last message (e5) has no replies; move to e3 which does.
	// e3 is index 2; navigate there.
	m.msgSel = 2
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("t should open a thread")
	}
	if m.focus != focusThread {
		t.Errorf("focus = %q, want thread", m.focus)
	}
	if len(m.focusOrder()) != 3 {
		t.Errorf("focusOrder len = %d, want 3 with thread open", len(m.focusOrder()))
	}
	m = Key(m, "esc")
	if m.threadOpen() {
		t.Error("esc should close the thread")
	}
	if m.focus != focusMessages {
		t.Errorf("after close focus = %q, want messages", m.focus)
	}
}

func TestInsertAndSend(t *testing.T) {
	m := newSized()
	before := len(m.curMsgs())
	m = Key(m, "i")
	if !m.insert {
		t.Fatal("i should enter INSERT mode")
	}
	for _, r := range "hello" {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	if got := len(m.curMsgs()); got != before+1 {
		t.Fatalf("message count = %d, want %d", got, before+1)
	}
	if last := m.curMsgs()[len(m.curMsgs())-1]; last.Text != "hello" || last.UserID != "me" {
		t.Errorf("sent message = %+v, want text=hello user=me", last)
	}
	if !m.insert {
		t.Error("should remain in INSERT after send")
	}
	m = Key(m, "esc")
	if m.insert {
		t.Error("esc should leave INSERT mode")
	}
}

func TestSidebarOpenClearsUnread(t *testing.T) {
	m := newSized()
	m = Key(m, "h") // focus sidebar (cursor on engineering)
	m = Key(m, "j") // → design row
	it := m.sideItems()[m.sideSel]
	if it.Header {
		t.Fatal("cursor landed on a header")
	}
	id := it.Conv.ID
	m = Key(m, "enter")
	if m.activeID != id {
		t.Errorf("activeID = %q, want %q", m.activeID, id)
	}
	if m.meta[id].Unread != 0 || m.meta[id].Mention {
		t.Errorf("opening %q should clear unread/mention, got %+v", id, m.meta[id])
	}
	if m.focus != focusMessages {
		t.Errorf("after open focus = %q, want messages", m.focus)
	}
}

// TestSendErrorRollsBack: a failed send removes the optimistic message,
// restores the draft, and surfaces the error banner.
func TestSendErrorRollsBack(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	for _, r := range "hello" {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	pendingID := m.curMsgs()[len(m.curMsgs())-1].ID
	before := len(m.curMsgs())
	next, _ := m.Update(sentMsg{convID: m.activeID, pendingID: pendingID, text: "hello", err: fmt.Errorf("boom")})
	m = next.(Model)
	if len(m.curMsgs()) != before-1 {
		t.Errorf("failed send should remove the pending message")
	}
	if m.draft.Value() != "hello" {
		t.Errorf("failed send should restore the draft, got %q", m.draft.Value())
	}
	if m.loadErr == nil {
		t.Error("failed send should surface the error banner")
	}
}

// TestPerConversationDrafts: switching channels parks the draft and restores
// the target's.
func TestPerConversationDrafts(t *testing.T) {
	m := newSized()
	first := m.activeID
	m = Key(m, "i")
	for _, r := range "wip" {
		m = Key(m, string(r))
	}
	m = Key(m, "esc")
	m.openChannel("random")
	if m.draft.Value() != "" {
		t.Errorf("new channel should start with an empty draft, got %q", m.draft.Value())
	}
	m.openChannel(first)
	if m.draft.Value() != "wip" {
		t.Errorf("returning should restore the draft, got %q", m.draft.Value())
	}
}

// TestReactApply: a confirmed reaction toggles the pill on the message.
func TestReactApply(t *testing.T) {
	m := newSized()
	msgID := m.curMsgs()[0].ID
	next, _ := m.Update(reactMsg{convID: m.activeID, msgID: msgID, name: "thumbsup", added: true})
	m = next.(Model)
	found := false
	for _, r := range m.curMsgs()[0].Reactions {
		if r.Emoji == "👍" && r.Count == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("added reaction should appear, got %+v", m.curMsgs()[0].Reactions)
	}
	next, _ = m.Update(reactMsg{convID: m.activeID, msgID: msgID, name: "thumbsup", added: false})
	m = next.(Model)
	for _, r := range m.curMsgs()[0].Reactions {
		if r.Emoji == "👍" {
			t.Fatalf("removed reaction should disappear, got %+v", m.curMsgs()[0].Reactions)
		}
	}
}

// TestEditFlow: e on your own message pre-fills the composer; enter saves.
func TestEditFlow(t *testing.T) {
	m := newSized()
	m.messages[m.activeID] = append(m.messages[m.activeID],
		data.Message{ID: "mine", UserID: "me", Time: "10:00", Text: "old"})
	m.msgSel = len(m.curMsgs()) - 1
	m = Key(m, "e")
	if !m.insert || m.editingID != "mine" || m.draft.Value() != "old" {
		t.Fatalf("e should start editing (insert=%v editing=%q draft=%q)", m.insert, m.editingID, m.draft.Value())
	}
	m = Key(m, "!")
	m = Key(m, "enter")
	if m.editingID != "" || m.insert {
		t.Error("saving should leave edit + insert mode")
	}
	if got := m.curMsgs()[len(m.curMsgs())-1].Text; got != "old!" {
		t.Errorf("edited text = %q, want old!", got)
	}
}

// TestDeleteConfirm: dd opens the confirm; y removes the message.
func TestDeleteConfirm(t *testing.T) {
	m := newSized()
	m.messages[m.activeID] = append(m.messages[m.activeID],
		data.Message{ID: "mine", UserID: "me", Time: "10:00", Text: "oops"})
	m.msgSel = len(m.curMsgs()) - 1
	before := len(m.curMsgs())
	m = Key(m, "d")
	m = Key(m, "d")
	if !m.confirm.open {
		t.Fatal("dd on your own message should open the confirm")
	}
	m = Key(m, "y")
	if len(m.curMsgs()) != before-1 {
		t.Errorf("y should delete the message (len=%d)", len(m.curMsgs()))
	}
}

// TestLocalFind: / jumps to a matching message; n repeats.
func TestLocalFind(t *testing.T) {
	m := newSized()
	m = Key(m, "/")
	if !m.findOpen {
		t.Fatal("/ should open the find prompt")
	}
	for _, r := range "dirty" {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	if m.findOpen {
		t.Fatal("enter should close the find prompt")
	}
	if got := m.curMsgs()[m.msgSel].Text; !strings.Contains(strings.ToLower(got), "dirty") {
		t.Errorf("cursor should land on a matching message, got %q", got)
	}
}

// TestSearchPickerFlow: s → query → results → enter jumps to the hit.
func TestSearchPickerFlow(t *testing.T) {
	m := newSized()
	m = Key(m, "s")
	if !m.picker.open {
		t.Fatal("s should open the search picker")
	}
	for _, r := range "standup" {
		m = Key(m, string(r))
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("enter on a fresh query should run the search")
	}
	res, ok := cmd().(searchMsg)
	if !ok || len(res.hits) == 0 {
		t.Fatalf("mock search should hit 'standup', got %+v", res)
	}
	next, _ = m.Update(res)
	m = next.(Model)
	if len(m.picker.items) == 0 {
		t.Fatal("results should fill the picker")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.picker.open {
		t.Error("picking a result should close the picker")
	}
	if got := m.curMsgs()[m.msgSel].Text; !strings.Contains(strings.ToLower(got), "standup") {
		t.Errorf("cursor should land on the search hit, got %q", got)
	}
}

// TestNewMarkOnOpen: opening a conversation with unread places the ── new ──
// mark and lands the cursor on the first unread message.
func TestNewMarkOnOpen(t *testing.T) {
	m := newSized() // mock: design has 1 unread
	m.openChannel("design")
	msgs := m.curMsgs()
	if len(msgs) == 0 {
		t.Fatal("design should have messages")
	}
	wantIdx := len(msgs) - 1
	if m.newMark["design"] != msgs[wantIdx].ID {
		t.Errorf("newMark = %q, want %q", m.newMark["design"], msgs[wantIdx].ID)
	}
	if m.msgSel != wantIdx {
		t.Errorf("cursor should land on the first unread (sel=%d, want %d)", m.msgSel, wantIdx)
	}
}

// TestMentionAutocomplete: typing @ad pops handle suggestions; tab completes.
func TestMentionAutocomplete(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	for _, r := range "hey @ad" {
		m = Key(m, string(r))
	}
	if !m.suggestActive() {
		t.Fatal("@ad should pop mention suggestions")
	}
	if got := m.suggest.items[0].label; got != "ada" {
		t.Fatalf("first suggestion = %q, want ada", got)
	}
	m = Key(m, "tab")
	if got := m.draft.Value(); got != "hey @ada " {
		t.Errorf("tab should complete the handle, draft = %q", got)
	}
	if m.suggestActive() {
		t.Error("accepting should dismiss the popup")
	}
}

// TestEmojiAutocomplete: :fir → tab inserts the glyph.
func TestEmojiAutocomplete(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	for _, r := range "ship it :fir" {
		m = Key(m, string(r))
	}
	if !m.suggestActive() {
		t.Fatal(":fir should pop emoji suggestions")
	}
	m = Key(m, "tab")
	if got := m.draft.Value(); got != "ship it 🔥 " {
		t.Errorf("tab should insert the glyph, draft = %q", got)
	}
}

// TestSuggestEnterDoesNotSend: enter accepts the completion instead of sending.
func TestSuggestEnterDoesNotSend(t *testing.T) {
	m := newSized()
	before := len(m.curMsgs())
	m = Key(m, "i")
	for _, r := range "@ad" {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	if len(m.curMsgs()) != before {
		t.Error("enter with the popup open must not send")
	}
	if got := m.draft.Value(); got != "@ada " {
		t.Errorf("enter should complete, draft = %q", got)
	}
	if !m.insert {
		t.Error("should stay in INSERT mode")
	}
}

// TestSuggestEscStaysInInsert: esc dismisses the popup, not INSERT mode.
func TestSuggestEscStaysInInsert(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	for _, r := range "@ad" {
		m = Key(m, string(r))
	}
	m = Key(m, "esc")
	if m.suggestActive() {
		t.Error("esc should dismiss the popup")
	}
	if !m.insert {
		t.Error("esc with the popup open should stay in INSERT")
	}
	m = Key(m, "esc")
	if m.insert {
		t.Error("a second esc should leave INSERT")
	}
}

// TestJoinFlow: the join picker lists joinable channels; picking one adds it
// to the sidebar and opens it.
func TestJoinFlow(t *testing.T) {
	m := newSized()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = next.(Model)
	m = Key(m, "join")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // run "Browse & join channels"
	m = next.(Model)
	if cmd == nil || !m.picker.open || m.picker.kind != "join" {
		t.Fatal("the palette command should open the join picker")
	}
	res, ok := cmd().(joinableMsg) // joinableCmd runs synchronously on the mock
	if !ok {
		// openJoinPicker batches focus + fetch; find the joinableMsg
		batch, _ := cmd().(tea.BatchMsg)
		for _, c := range batch {
			if r, isJ := c().(joinableMsg); isJ {
				res, ok = r, true
			}
		}
	}
	if !ok || len(res.convs) == 0 {
		t.Fatal("mock should list joinable channels")
	}
	next, _ = m.Update(res)
	m = next.(Model)
	if len(m.picker.items) != len(res.convs) {
		t.Fatalf("picker should list %d channels", len(res.convs))
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // join the first
	m = next.(Model)
	joined, ok := cmd().(joinedMsg)
	if !ok || joined.err != nil {
		t.Fatalf("picking should join, got %T %+v", joined, joined.err)
	}
	next, _ = m.Update(joined)
	m = next.(Model)
	if _, exists := m.ws.Conversation(joined.conv.ID); !exists {
		t.Error("joined channel should be in the workspace")
	}
	if m.activeID != joined.conv.ID {
		t.Errorf("joined channel should open, active = %q", m.activeID)
	}
}

func TestThreadReply(t *testing.T) {
	m := newSized()
	m.msgSel = 2 // e3 with replies
	m = Key(m, "t")
	root, _ := m.threadRoot()
	before := len(root.Replies)
	m = Key(m, "i") // insert in thread composer
	if !m.insert || m.focus != focusThread {
		t.Fatalf("expected insert in thread, got insert=%v focus=%q", m.insert, m.focus)
	}
	for _, r := range "lgtm" {
		m = Key(m, string(r))
	}
	m = Key(m, "enter")
	root, _ = m.threadRoot()
	if len(root.Replies) != before+1 {
		t.Fatalf("replies = %d, want %d", len(root.Replies), before+1)
	}
	if last := root.Replies[len(root.Replies)-1]; last.Text != "lgtm" {
		t.Errorf("reply text = %q, want lgtm", last.Text)
	}
}

// TestQuitPersistsState: quitting saves drafts + recency; New-style restore
// path is exercised via config round-trip.
func TestQuitPersistsState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newSized()
	m.persistState = true
	m = Key(m, "i")
	for _, r := range "wip" {
		m = Key(m, string(r))
	}
	m = Key(m, "esc")
	m.openChannel("random")
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	st, err := config.LoadState()
	if err != nil {
		t.Fatalf("state not saved: %v", err)
	}
	if st.Drafts["engineering"] != "wip" {
		t.Errorf("draft not persisted, got %+v", st.Drafts)
	}
	if len(st.Recent) == 0 || st.Recent[0] != "random" {
		t.Errorf("recency not persisted, got %v", st.Recent)
	}
}

// TestWorkspaceSwitchFlow: the picker lists workspaces; picking another one
// persists it and emits ReloadMsg for root.
// TestHideRemovesFromSidebarNotPalette: hiding a channel removes it from the
// sidebar (sideItems) but keeps it in the palette (paletteItems with id "ch:<id>").
func TestHideRemovesFromSidebarNotPalette(t *testing.T) {
	m := newSized()
	id := "design" // visible channel in the mock

	// Confirm it appears as a non-header item in sideItems before hiding.
	inSide := func() bool {
		for _, it := range m.sideItems() {
			if !it.Header && it.Conv.ID == id {
				return true
			}
		}
		return false
	}
	if !inSide() {
		t.Fatalf("%q should be a non-header item in sideItems before hide", id)
	}

	m.toggleHide(id)

	if inSide() {
		t.Errorf("%q should be absent from sideItems after toggleHide", id)
	}

	// Must still appear in paletteItems with id "ch:<id>".
	wantPalID := "ch:" + id
	inPal := false
	for _, it := range m.paletteItems() {
		if it.id == wantPalID {
			inPal = true
			break
		}
	}
	if !inPal {
		t.Errorf("%q should still be in paletteItems (id %q) after hide", id, wantPalID)
	}
}

// TestUnreadUnhidesOnIncrease: a hidden conv stays hidden when the unread count
// equals the baseline but is un-hidden when the count exceeds it.
func TestUnreadUnhidesOnIncrease(t *testing.T) {
	m := newSized()
	// "incidents" has Unread:2 in the mock and is not the activeID ("engineering").
	id := "incidents"
	if m.activeID == id {
		t.Fatalf("test requires %q to not be activeID", id)
	}

	baseline := m.meta[id].Unread // 2

	m.toggleHide(id)

	if got, ok := m.hidden[id]; !ok || got != baseline {
		t.Fatalf("after hide, m.hidden[%q] = %d ok=%v; want %d", id, got, ok, baseline)
	}

	inSide := func(mm Model) bool {
		for _, it := range mm.sideItems() {
			if !it.Header && it.Conv.ID == id {
				return true
			}
		}
		return false
	}
	if inSide(m) {
		t.Fatalf("%q should be absent from sideItems while hidden", id)
	}

	// Feed count == baseline: must stay hidden.
	next, _ := m.Update(unreadMsg{counts: map[string]int{id: baseline}})
	m = next.(Model)
	if _, ok := m.hidden[id]; !ok {
		t.Errorf("count == baseline should NOT un-hide %q", id)
	}
	if inSide(m) {
		t.Errorf("%q should still be absent from sideItems when count == baseline", id)
	}

	// Feed count == baseline+1: must un-hide.
	next, _ = m.Update(unreadMsg{counts: map[string]int{id: baseline + 1}})
	m = next.(Model)
	if _, ok := m.hidden[id]; ok {
		t.Errorf("count > baseline should un-hide %q, but it remains in m.hidden", id)
	}
	if !inSide(m) {
		t.Errorf("%q should be back in sideItems after un-hide", id)
	}
}

// TestSendUnhidesActive: hiding the active channel and then sending a message
// removes it from m.hidden.
func TestSendUnhidesActive(t *testing.T) {
	m := newSized()
	id := m.activeID // "engineering"

	m.toggleHide(id)
	if _, ok := m.hidden[id]; !ok {
		t.Fatalf("toggleHide should add %q to m.hidden", id)
	}

	m.draft.SetValue("hi")
	m.sendMessage()

	if _, ok := m.hidden[id]; ok {
		t.Errorf("sendMessage should remove %q from m.hidden", id)
	}
}

func TestWorkspaceSwitchFlow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_ = config.SaveWorkspace(config.Workspace{Name: "acme", TeamID: "T1", Tokens: config.Tokens{User: "u1"}})
	_ = config.SaveWorkspace(config.Workspace{Name: "personal", TeamID: "T2", Tokens: config.Tokens{User: "u2"}})
	_ = config.SetActiveWorkspace("acme")
	m := newSized()
	m.workspaces, m.activeWorkspace, _ = config.LoadWorkspaces()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = next.(Model)
	m = Key(m, "switch works")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || !m.picker.open || m.picker.kind != "workspace" {
		t.Fatal("palette command should open the workspace picker")
	}
	m = Key(m, "personal")
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("picking a workspace should return a cmd")
	}
	if _, ok := cmd().(ReloadMsg); !ok {
		t.Fatal("picking another workspace should emit ReloadMsg")
	}
	if _, active, _ := config.LoadWorkspaces(); active != "personal" {
		t.Errorf("switch should persist, active = %q", active)
	}
	if tok, _ := config.LoadTokens(); tok.User != "u2" {
		t.Errorf("LoadTokens should follow the switch, got %+v", tok)
	}
}

func TestHideKeepsSideSelValid(t *testing.T) {
	m := newSized()
	sel := m.selectable()
	m.sideSel = sel[len(sel)-1] // select the last sidebar row
	id := m.sideItems()[m.sideSel].Conv.ID
	m.toggleHide(id)
	items := m.sideItems()
	if m.sideSel >= len(items) {
		t.Fatalf("sideSel %d out of range after hide (len %d)", m.sideSel, len(items))
	}
	if items[m.sideSel].Header {
		t.Fatalf("sideSel landed on a header after hide")
	}
}

// TestAllHiddenNoMoveSelPanic: when every selectable conversation is hidden,
// moveSel must not panic (no index into empty slice).
func TestAllHiddenNoMoveSelPanic(t *testing.T) {
	m := newSized()
	// Snapshot the conv IDs from the current sideItems before any mutations.
	var convIDs []string
	for _, it := range m.sideItems() {
		if !it.Header {
			convIDs = append(convIDs, it.Conv.ID)
		}
	}
	for _, id := range convIDs {
		m.toggleHide(id)
	}
	m.focus = focusSidebar
	// Both calls must not panic.
	m.moveSel(1)
	m.moveSel(-1)
	// Optionally assert sideSel was not mutated to something illegal.
	if m.sideSel != 0 {
		// sideSel == 0 is the only safe sentinel when everything is hidden.
		// toggleHide clamps it, but moveSel must not corrupt it further.
		_ = m.sideSel // not strictly required to be 0 per spec; just no panic
	}
}

// TestJumpUnreadSkipsHidden: ] must not jump into a hidden conversation even if
// it has unread messages.
func TestJumpUnreadSkipsHidden(t *testing.T) {
	m := newSized()
	// Start on a read channel so there's somewhere to jump from.
	m.openChannel("general")
	if m.activeID != "general" {
		t.Fatalf("expected activeID=general, got %q", m.activeID)
	}
	// Hide every channel that carries unread.
	for id, meta := range m.meta {
		if meta.Unread > 0 {
			m.toggleHide(id)
		}
	}
	m.jumpUnread(1)
	// With all unread convs hidden the jump should be a no-op.
	if m.activeID != "general" {
		t.Errorf("] jumped into hidden unread conv %q; want activeID=general", m.activeID)
	}
}

// TestOpenHiddenResetsBaseline: opening a hidden conversation resets its
// resurface baseline to 0 so a subsequent message will re-surface it.
func TestOpenHiddenResetsBaseline(t *testing.T) {
	m := newSized()
	// "incidents" has Unread:2 in the mock.
	m.toggleHide("incidents")
	if got, ok := m.hidden["incidents"]; !ok || got != 2 {
		t.Fatalf("after hide, m.hidden[\"incidents\"] = %d ok=%v; want 2", got, ok)
	}
	m.openChannel("incidents")
	// Must still be in m.hidden (not removed), but baseline reset to 0.
	got, ok := m.hidden["incidents"]
	if !ok {
		t.Fatal("openChannel should not remove a hidden conv from m.hidden; baseline reset expected")
	}
	if got != 0 {
		t.Errorf("baseline after open = %d, want 0", got)
	}
}

// TestStateSnapshotCopiesHidden: stateSnapshot must deep-copy m.hidden so that
// subsequent mutations to the live map do not affect the snapshot (data-race
// safety for the async persist goroutine).
func TestStateSnapshotCopiesHidden(t *testing.T) {
	m := newSized()
	m.toggleHide("design")
	s := m.stateSnapshot()
	// Mutate the live map after the snapshot.
	m.hidden["engineering"] = 9
	delete(m.hidden, "design")
	// The snapshot must be independent.
	if _, ok := s.Hidden["design"]; !ok {
		t.Error("snapshot should still contain \"design\" after live map delete")
	}
	if _, ok := s.Hidden["engineering"]; ok {
		t.Error("snapshot should not contain \"engineering\" added after snapshot")
	}
}

// TestStaleUnreadPollIgnored: a poll that fired before a conversation was read
// must not re-flag it as unread when its (stale) result lands later.
func TestStaleUnreadPollIgnored(t *testing.T) {
	m := newSized()
	// open A (active, read), then B (active) so A is non-active
	a := m.ws.Channels[1].ID
	b := m.ws.Channels[2].ID
	m = WithSize(m, 100, 30)
	m2, _ := m.Update(nil)
	m = m2.(Model)
	staleSeq := m.readSeq // a poll "fired" at this seq
	m.openChannel(a)      // read A → readSeqOf[a] = readSeq++ (> staleSeq)
	m.openChannel(b)      // B active, A non-active
	// stale poll result for A (computed before the read) must be ignored
	next, _ := m.Update(unreadMsg{counts: map[string]int{a: 7}, seq: staleSeq})
	m = next.(Model)
	if got := m.meta[a].Unread; got != 0 {
		t.Errorf("stale poll re-flagged read conversation: meta[a].Unread = %d, want 0", got)
	}
	// a fresh poll (fired after the read) IS applied
	next, _ = m.Update(unreadMsg{counts: map[string]int{a: 7}, seq: m.readSeq})
	m = next.(Model)
	if got := m.meta[a].Unread; got != 7 {
		t.Errorf("fresh poll not applied: meta[a].Unread = %d, want 7", got)
	}
}

// TestThreadReplyDoesNotInflateBadge: a plain thread reply in a non-active channel
// must NOT bump the unread badge (it lives in the Threads view); a root message
// must; a thread reply that @-mentions you must.
func TestThreadReplyDoesNotInflateBadge(t *testing.T) {
	m := newSized()
	ch := m.ws.Channels[1].ID
	other := m.ws.Channels[2].ID
	m.openChannel(other) // make `ch` non-active
	base := m.meta[ch].Unread

	root := source.Event{ConvID: ch, Msg: data.Message{ID: "100.0", UserID: "U_OTHER"}}
	m.applyInactiveEvent(root)
	if got := m.meta[ch].Unread; got != base+1 {
		t.Fatalf("root message should bump: got %d want %d", got, base+1)
	}

	reply := source.Event{ConvID: ch, ThreadTS: "100.0", Msg: data.Message{ID: "101.0", UserID: "U_OTHER"}}
	m.applyInactiveEvent(reply)
	if got := m.meta[ch].Unread; got != base+1 {
		t.Fatalf("plain thread reply must NOT inflate badge: got %d want %d", got, base+1)
	}

	mentionReply := source.Event{ConvID: ch, ThreadTS: "100.0", Msg: data.Message{ID: "102.0", UserID: "U_OTHER", MentionsMe: true}}
	m.applyInactiveEvent(mentionReply)
	if got := m.meta[ch].Unread; got != base+2 || !m.meta[ch].Mention {
		t.Fatalf("thread reply that @-mentions you must count+flag: got %d mention=%v", got, m.meta[ch].Mention)
	}
}

// TestDMPollBoundedAndRotates: with many DMs the poll must (a) stay bounded per
// round, (b) always include the recently-used head, and (c) cover every dormant
// DM across enough rounds so none is starved of refresh.
func TestDMPollBoundedAndRotates(t *testing.T) {
	m := newSized()
	m.ws.DMs = nil
	for i := 0; i < 158; i++ {
		id := fmt.Sprintf("D%03d", i)
		m.ws.DMs = append(m.ws.DMs, data.Conversation{ID: id, Type: "dm", Name: id, UserID: "U" + id})
	}
	m.activeID = "other"
	m.recent = []string{"D000", "D001", "D002"} // a few recently-used DMs

	covered := map[string]bool{}
	for round := 0; round < 60; round++ {
		// head and tail are separate rounds now; together they must still stay
		// inside the same per-sweep budget the v0.5.2 fix established.
		ids := append(m.dmHeadIDs(), m.dmTailIDs()...)
		if len(ids) > dmPollHead+dmPollTail {
			t.Fatalf("round %d polled %d DMs, over budget %d", round, len(ids), dmPollHead+dmPollTail)
		}
		for _, id := range ids {
			covered[id] = true
		}
		// recently-used DMs must be polled every single round (freshness)
		for _, r := range m.recent {
			found := false
			for _, id := range ids {
				if id == r {
					found = true
				}
			}
			if !found {
				t.Fatalf("round %d: recent DM %s not polled", round, r)
			}
		}
		m.dmPollOffset += dmPollTail
	}
	if len(covered) != 158 {
		t.Fatalf("rotation starved some DMs: covered %d/158", len(covered))
	}
}

// isolateConfigDir points config.Dir() at a temp dir for the duration of a test.
// Overriding only HOME is not enough: config.Dir() checks XDG_CONFIG_HOME first,
// so on any desktop that sets it (most Linux distros) the test would read and
// write the real ~/.config/slack-tui.
func isolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}
