package app

import "testing"

func init() { /* tests drive the model headlessly via Key/WithSize */ }

func newSized() Model { return WithSize(New(), 100, 30) }

func TestInitialState(t *testing.T) {
	m := New()
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
