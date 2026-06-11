package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/data"
)

// seedThreads installs a known thread set into the cache: a root I authored, a
// root someone else authored that I replied to, and a thread I'm not in.
func seedThreads(m *Model) {
	m.messages["engineering"] = []data.Message{
		{ID: "mine1", UserID: "me", Time: "10:00", Text: "shipped the new pane", ReplyCount: 2,
			Replies: []data.Reply{{ID: "mine1r1", UserID: "ada", Text: "nice"}, {ID: "mine1r2", UserID: "lin", Text: "ship it"}}},
		{ID: "theirs1", UserID: "ada", Time: "10:05", Text: "anyone seen the flake", ReplyCount: 1,
			Replies: []data.Reply{{ID: "theirs1r1", UserID: "me", Text: "looking now"}}},
		{ID: "notmine", UserID: "marco", Time: "10:09", Text: "lunch plans", ReplyCount: 1,
			Replies: []data.Reply{{ID: "notminer1", UserID: "lin", Text: "tacos"}}},
	}
}

// TestThreadsInboxLists: T lists threads I'm in (authored or replied), excluding
// threads I never touched.
func TestThreadsInboxLists(t *testing.T) {
	m := newSized()
	seedThreads(&m)
	m = Key(m, "T")
	if !m.picker.open || m.picker.kind != "threads" {
		t.Fatal("T should open the threads picker")
	}
	ids := map[string]bool{}
	for _, it := range m.picker.items {
		ids[it.id] = true
	}
	if !ids["engineering|mine1"] {
		t.Error("thread I authored should appear")
	}
	if !ids["engineering|theirs1"] {
		t.Error("thread I replied to should appear")
	}
	if ids["engineering|notmine"] {
		t.Error("thread I never touched should not appear")
	}
}

// TestThreadsInboxJump: picking an item jumps to its channel and opens the thread.
func TestThreadsInboxJump(t *testing.T) {
	m := newSized()
	m.openChannel("general") // start somewhere else
	seedThreads(&m)
	m = Key(m, "T")

	// select engineering|theirs1 (drive selection to it)
	target := "engineering|theirs1"
	for i, it := range m.picker.items {
		if it.id == target {
			m.picker.index = i
			break
		}
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	if m.picker.open {
		t.Error("picking should close the picker")
	}
	if m.activeID != "engineering" {
		t.Errorf("should switch to engineering, got %q", m.activeID)
	}
	if !m.threadOpen() {
		t.Fatal("thread pane should be open")
	}
	if m.threadRootID != "theirs1" {
		t.Errorf("threadRootID = %q, want theirs1", m.threadRootID)
	}
}

// TestThreadsInboxNewCount: a thread with replies beyond the seen count shows a
// "(+k new)" badge; opening it clears the badge.
func TestThreadsInboxNewCount(t *testing.T) {
	m := newSized()
	seedThreads(&m)
	m.seenReplies["engineering|mine1"] = 1 // we've only seen 1 of the 2 replies

	hint := ""
	for _, it := range m.threadItems() {
		if it.id == "engineering|mine1" {
			hint = it.item.Hint
		}
	}
	if !strings.Contains(hint, "(+1 new)") {
		t.Errorf("hint should flag 1 new reply, got %q", hint)
	}

	// Opening the thread records the current count as seen → no more "new".
	m.openThread("mine1") // active is engineering
	for _, it := range m.threadItems() {
		if it.id == "engineering|mine1" && strings.Contains(it.item.Hint, "new") {
			t.Errorf("after opening, hint should have no new badge, got %q", it.item.Hint)
		}
	}
}
