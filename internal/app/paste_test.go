package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func pasteKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true}
}

// TestPasteTallContent: pasting more lines than the composer can show keeps
// the cursor (and the end of the paste) visible — the box grows to its cap
// and scrolls instead of hiding the text.
func TestPasteTallContent(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	next, _ := m.Update(pasteKey(strings.Repeat("line of code\n", 12) + "THE-END"))
	m = next.(Model)
	if got := m.composerHeight(); got != 2+maxComposerLines {
		t.Errorf("composer should grow to its cap, height = %d", got)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "THE-END") {
		t.Error("the cursor end of a tall paste must be visible")
	}
}

// TestPasteLongLine: a single pasted line wider than the composer soft-wraps;
// the box must grow and show the wrapped tail where the cursor is.
func TestPasteLongLine(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	next, _ := m.Update(pasteKey(strings.Repeat("wide ", 25) + "TAIL"))
	m = next.(Model)
	if got := m.composerHeight(); got <= 3 {
		t.Errorf("a soft-wrapping paste should grow the composer, height = %d", got)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "TAIL") {
		t.Error("the wrapped tail (cursor position) must be visible")
	}
}

// TestPasteCRLF: Windows/odd clipboards paste \r\n — newlines must normalize.
func TestPasteCRLF(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	next, _ := m.Update(pasteKey("one\r\ntwo"))
	m = next.(Model)
	if got := m.draft.Value(); strings.ContainsRune(got, '\r') {
		t.Errorf("draft should not contain CR, got %q", got)
	}
}
