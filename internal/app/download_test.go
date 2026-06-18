package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/source"
)

// TestDownloadNoFileFlashes: pressing S on a message with no Files flashes an error.
func TestDownloadNoFileFlashes(t *testing.T) {
	m := newSized()
	// The default active conversation ("engineering") has no files on any message.
	// Ensure the selected message has no Files (default mock messages don't).
	msgs := m.curMsgs()
	if len(msgs) == 0 {
		t.Fatal("expected at least one message in the active conversation")
	}
	m.msgSel = 0
	m.focus = focusMessages
	if len(msgs[0].Files) != 0 {
		t.Skipf("message[0] unexpectedly has files; test needs a no-file message")
	}

	cmd := m.downloadFiles()
	if cmd == nil {
		t.Fatal("downloadFiles on a file-less message should return a flash cmd (non-nil)")
	}
	// flash sets loadErr synchronously on the model pointer before returning.
	if m.loadErr == nil {
		t.Error("downloadFiles on a file-less message should set m.loadErr")
	}
	if !strings.Contains(m.loadErr.Error(), "no file") {
		t.Errorf("loadErr = %q, want to contain \"no file\"", m.loadErr.Error())
	}
}

// TestDownloadKeyFromMessages: pressing S from the message pane routes to
// downloadFiles; pressing S from the sidebar is a no-op.
func TestDownloadKeyFromMessages(t *testing.T) {
	m := newSized()

	// Inject a file-bearing message.
	fileMsg := data.Message{
		ID:    "dl2",
		Time:  "10:00",
		Text:  "look at this",
		Files: []data.File{{ID: "F2", Name: "report.pdf", URL: "https://files.slack.com/report.pdf", Size: 512}},
	}
	m.messages[m.activeID] = append(m.messages[m.activeID], fileMsg)
	m.msgSel = len(m.messages[m.activeID]) - 1

	// From focusMessages: S should trigger downloadFiles (non-nil cmd).
	m.focus = focusMessages
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	m = next.(Model)
	if cmd == nil {
		t.Error("S from focusMessages with a file-bearing message should return a non-nil cmd")
	}

	// From focusSidebar: S should be a no-op (nil cmd, no flash).
	m.focus = focusSidebar
	m.loadErr = nil // clear any prior flash
	next2, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	m = next2.(Model)
	if cmd2 != nil {
		t.Error("S from focusSidebar should be a no-op (nil cmd)")
	}
	if m.loadErr != nil {
		t.Errorf("S from focusSidebar should not flash an error, got: %v", m.loadErr)
	}
}

// TestDownloadSavesFiles: pressing S on a message with Files downloads them,
// records the download in the mock, and shows the ✓ saved toast.
func TestDownloadSavesFiles(t *testing.T) {
	m := newSized()

	// Inject a message that has a file attachment into the active conversation.
	fileMsg := data.Message{
		ID:     "dl1",
		UserID: "ada",
		Time:   "10:00",
		Text:   "here is the spec",
		Files:  []data.File{{ID: "F1", Name: "spec.pdf", URL: "https://files.slack.com/spec.pdf", Size: 1024}},
	}
	m.messages[m.activeID] = append(m.messages[m.activeID], fileMsg)
	m.msgSel = len(m.messages[m.activeID]) - 1
	m.focus = focusMessages

	// Execute the download command.
	cmd := m.downloadFiles()
	if cmd == nil {
		t.Fatal("downloadFiles should return a non-nil cmd when the message has files")
	}
	rawMsg := cmd()
	dl, ok := rawMsg.(downloadMsg)
	if !ok {
		t.Fatalf("cmd() should return downloadMsg, got %T", rawMsg)
	}
	if dl.err != nil {
		t.Fatalf("download should succeed in the mock, got err: %v", dl.err)
	}

	// Feed the downloadMsg through Update so the model processes it.
	next, _ := m.Update(dl)
	m = next.(Model)

	// Verify the mock recorded the download.
	mock := m.src.(*source.Mock)
	recs := mock.Downloads()
	if len(recs) == 0 {
		t.Fatal("mock should have recorded one Download call")
	}
	if recs[0].File.Name != "spec.pdf" {
		t.Errorf("recorded file name = %q, want spec.pdf", recs[0].File.Name)
	}
	if recs[0].Dir != downloadDir() {
		t.Errorf("recorded dir = %q, want %q", recs[0].Dir, downloadDir())
	}

	// Verify the toast was set.
	if !strings.HasPrefix(m.copyToast, "✓ saved") {
		t.Errorf("copyToast = %q, want prefix \"✓ saved\"", m.copyToast)
	}
}
