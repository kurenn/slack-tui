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
	t.Setenv("HOME", t.TempDir())
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
	t.Setenv("HOME", t.TempDir()) // keep config.Save off the real config dir
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
	m.handleEvent(source.Event{ConvID: other, Msg: data.Message{UserID: "ada", Text: "hey @you", MentionsMe: true}})
	if m.meta[other].Unread != 1 || !m.meta[other].Mention {
		t.Fatalf("non-active event should mark unread+mention, got %+v", m.meta[other])
	}
	before := m.meta[m.activeID]
	m.handleEvent(source.Event{ConvID: m.activeID, Msg: data.Message{UserID: "ada", Text: "x"}})
	if m.meta[m.activeID] != before {
		t.Error("active-channel event should not change its unread")
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
