package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/source"
)

// TestParseDroppedPaths exercises the path parser with a variety of terminal
// drop / paste formats.
func TestParseDroppedPaths(t *testing.T) {
	dir := t.TempDir()

	// Create real files for positive cases.
	plain := filepath.Join(dir, "report.pdf")
	spaced := filepath.Join(dir, "my file.txt")
	second := filepath.Join(dir, "image.png")
	for _, p := range []string{plain, spaced, second} {
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "plain path",
			input: plain,
			want:  []string{plain},
		},
		{
			name:  "double-quoted path",
			input: `"` + plain + `"`,
			want:  []string{plain},
		},
		{
			name:  "backslash-escaped space",
			input: strings.ReplaceAll(spaced, " ", `\ `),
			want:  []string{spaced},
		},
		{
			name:  "file:// URL form",
			input: "file://" + plain,
			want:  []string{plain},
		},
		{
			name:  "two space-separated paths",
			input: plain + " " + second,
			want:  []string{plain, second},
		},
		{
			name:  "nonexistent path yields nothing",
			input: filepath.Join(dir, "nope.txt"),
			want:  nil,
		},
		{
			name:  "directory yields nothing",
			input: dir,
			want:  nil,
		},
		{
			name:  "non-path string yields nothing",
			input: "hello world this is normal text",
			want:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDroppedPaths(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestPasteStagesFile: a Paste KeyMsg carrying a real file path stages it.
func TestPasteStagesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "drop.txt")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	m := newSized()
	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(path),
		Paste: true,
	})
	m = next.(Model)

	if len(m.pendingFiles) != 1 || m.pendingFiles[0] != path {
		t.Errorf("pendingFiles = %v, want [%s]", m.pendingFiles, path)
	}
	if !m.insert {
		t.Error("stageAttachments should enter INSERT mode")
	}
}

// TestSendAttachmentsUploads: staged file + draft text → Upload called with
// the right args; pendingFiles and draft cleared afterwards.
func TestSendAttachmentsUploads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, []byte("pdf"), 0600); err != nil {
		t.Fatal(err)
	}

	m := newSized()
	m.pendingFiles = []string{path}
	m.draft.SetValue("see attached")

	cmd := m.sendMessage()
	if cmd == nil {
		t.Fatal("sendMessage with staged files must return a cmd")
	}
	msg := cmd()
	um, ok := msg.(uploadMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want uploadMsg", msg)
	}
	if um.err != nil {
		t.Fatalf("upload failed: %v", um.err)
	}

	mk := m.src.(*source.Mock)
	uploads := mk.Uploads()
	if len(uploads) != 1 {
		t.Fatalf("Upload called %d times, want 1", len(uploads))
	}
	rec := uploads[0]
	if rec.ConvID != m.activeID {
		t.Errorf("ConvID = %q, want %q", rec.ConvID, m.activeID)
	}
	if len(rec.Paths) != 1 || rec.Paths[0] != path {
		t.Errorf("Paths = %v, want [%s]", rec.Paths, path)
	}
	if rec.Comment != "see attached" {
		t.Errorf("Comment = %q, want %q", rec.Comment, "see attached")
	}
	if len(m.pendingFiles) != 0 {
		t.Errorf("pendingFiles should be empty after send, got %v", m.pendingFiles)
	}
	if m.draft.Value() != "" {
		t.Errorf("draft should be cleared after send, got %q", m.draft.Value())
	}
}

// TestOpenChannelClearsPending: switching conversations clears staged files.
func TestOpenChannelClearsPending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	m := newSized()
	m.pendingFiles = []string{path}

	other := "random" // a different conv in the mock workspace
	m.openChannel(other)

	if len(m.pendingFiles) != 0 {
		t.Errorf("openChannel should clear pendingFiles, got %v", m.pendingFiles)
	}
}

// TestParseProseWithPathNotStaged: a prose string containing a real file path
// should not be staged — parseDroppedPaths must return nil so the paste goes
// through as normal text.
func TestParseProseWithPathNotStaged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	prose := "see " + path + " for details"
	got := parseDroppedPaths(prose)
	if got != nil {
		t.Errorf("prose with embedded path should return nil, got %v", got)
	}
}

// TestParseWholePathWithSpaces: a path whose name contains an unescaped space
// (no quotes, no backslashes) should be recognized as a single file.
func TestParseWholePathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my report.pdf")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	got := parseDroppedPaths(path)
	if len(got) != 1 || got[0] != path {
		t.Errorf("parseDroppedPaths(%q) = %v, want [%s]", path, got, path)
	}
}

// TestChipDoesNotOverflowHeight: staging a file must not cause View() to
// produce more lines than m.height (the double-count geometry bug).
func TestChipDoesNotOverflowHeight(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	m := WithSize(newTest(), 120, 40)
	m.pendingFiles = []string{path}
	m.draft.SetValue("some draft text")
	m.syncComposerSizes()

	out := m.View()
	lines := strings.Count(out, "\n") + 1
	if lines != m.height {
		t.Errorf("View() produced %d lines, want %d (height); double-count geometry bug", lines, m.height)
	}
}

// TestUploadErrorRestores: when an upload fails the staged files and comment
// are restored so the user can retry without retyping.
func TestUploadErrorRestores(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	m := newSized()
	m.pendingFiles = []string{path}
	m.draft.SetValue("important comment")

	// Arm the mock to fail.
	m.src.(*source.Mock).UploadErr = errors.New("boom")

	cmd := m.sendMessage()
	if cmd == nil {
		t.Fatal("sendMessage with staged files must return a cmd")
	}
	msg := cmd()
	next, _ := m.Update(msg)
	m = next.(Model)

	if len(m.pendingFiles) == 0 {
		t.Error("pendingFiles should be restored after upload error")
	}
	if m.uploadNote != "" {
		t.Errorf("uploadNote should be cleared after error, got %q", m.uploadNote)
	}
}
