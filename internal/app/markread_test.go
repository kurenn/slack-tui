package app

import (
	"errors"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/source"
)

// markRecorder wraps a Source to record every conversations.mark the model
// issues, and can fail them the way a token missing im:write does.
type markRecorder struct {
	source.Source
	mu    sync.Mutex
	marks []string // "convID|ts", in call order
	err   error
}

func (r *markRecorder) MarkRead(convID, ts string) error {
	r.mu.Lock()
	r.marks = append(r.marks, convID+"|"+ts)
	r.mu.Unlock()
	return r.err
}

func (r *markRecorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.marks...)
}

// recording swaps the model's source for a recorder around the mock.
func recording(m Model) (Model, *markRecorder) {
	rec := &markRecorder{Source: m.src}
	m.src = rec
	return m, rec
}

// deliver runs a cmd and feeds its msg through Update once. It deliberately
// stops there: the follow-up of a surfaced error is an 8s tick that clears the
// banner, which the test needs to observe *before* it fires.
func deliver(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

// TestMarkReadSkipsPendingSend: conversations.mark rejects anything that isn't a
// real Slack timestamp, so a mark firing between an optimistic send and its ack
// must fall back to the last acked message rather than post "pending-1".
func TestMarkReadSkipsPendingSend(t *testing.T) {
	m, rec := recording(newTest())
	m.messages[m.activeID] = []data.Message{
		{ID: "1700000000.000100", UserID: "ada", Time: "09:00", Text: "hi"},
		{ID: pendingPrefix + "1", UserID: m.ws.MeID, Time: "09:01", Text: "not acked yet"},
	}
	cmd := m.markReadCmd(m.activeID)
	if cmd == nil {
		t.Fatal("a conversation with an acked message should still be markable")
	}
	cmd()
	got := rec.seen()
	if len(got) != 1 || got[0] != m.activeID+"|1700000000.000100" {
		t.Fatalf("mark should use the last acked ts, got %v", got)
	}
}

// TestMarkReadAllPendingIsSkipped: with nothing acked there is no valid ts, so
// the mark is dropped instead of being sent as a local pending ID.
func TestMarkReadAllPendingIsSkipped(t *testing.T) {
	m, rec := recording(newTest())
	m.messages[m.activeID] = []data.Message{
		{ID: pendingPrefix + "1", UserID: m.ws.MeID, Time: "09:01", Text: "first ever"},
	}
	if cmd := m.markReadCmd(m.activeID); cmd != nil {
		cmd()
	}
	if got := rec.seen(); len(got) != 0 {
		t.Fatalf("a pending-only conversation should issue no mark, got %v", got)
	}
}

// TestSendMarksRead: posting through the API doesn't advance your own read
// marker, so Slack's web/desktop clients keep the conversation unread until the
// TUI marks it. Acking a send must do that immediately, not wait for the poll.
func TestSendMarksRead(t *testing.T) {
	m, rec := recording(newTest())
	conv := m.activeID
	m.messages[conv] = []data.Message{
		{ID: "1700000000.000100", UserID: "ada", Time: "09:00", Text: "hi"},
		{ID: pendingPrefix + "1", UserID: m.ws.MeID, Time: "09:02", Text: "shipped"},
	}
	cmd := m.applySent(sentMsg{
		convID:    conv,
		pendingID: pendingPrefix + "1",
		text:      "shipped",
		msg:       data.Message{ID: "1700000000.000200", UserID: m.ws.MeID, Time: "09:02", Text: "shipped"},
	})
	if cmd == nil {
		t.Fatal("a successful send should mark the conversation read")
	}
	cmd()
	got := rec.seen()
	if len(got) != 1 || got[0] != conv+"|1700000000.000200" {
		t.Fatalf("send should mark read up to the sent message, got %v", got)
	}
}

// TestMarkReadFailureSurfacesOnce: a rejected mark (the missing-scope case) has
// to be visible — the sidebar clears its badge locally either way, so silence
// looked like "Slack keeps showing unread for no reason". Every poll retries, so
// it must be reported exactly once.
func TestMarkReadFailureSurfacesOnce(t *testing.T) {
	m, rec := recording(newTest())
	rec.err = errors.New("read state not syncing to Slack")
	m.messages[m.activeID] = []data.Message{
		{ID: "1700000000.000100", UserID: "ada", Time: "09:00", Text: "hi"},
	}
	m = deliver(m, m.markReadCmd(m.activeID))
	if m.loadErr == nil {
		t.Fatal("a failed mark-read should surface an error the user can act on")
	}
	m.loadErr = nil
	m = deliver(m, m.markReadCmd(m.activeID))
	if m.loadErr != nil {
		t.Fatalf("the repeat failure should stay quiet, got %v", m.loadErr)
	}
}
