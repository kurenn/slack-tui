package app

import (
	"strings"
	"testing"
	"unicode"

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

// TestPasteBinaryRejected: pasting raw bytes (an image off the clipboard) must
// not land in the composer — it used to persist as an unclearable draft.
func TestPasteBinaryRejected(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x01\x00\x00\x00\x01\x00\x08\x06\x00\x00\x00"
	next, _ := m.Update(pasteKey(png))
	m = next.(Model)
	if got := m.draft.Value(); got != "" {
		t.Errorf("binary paste must be dropped, draft = %q", got)
	}
	if m.loadErr == nil {
		t.Error("binary paste should flash an error")
	}
	// …while ordinary text still goes in.
	next, _ = m.Update(pasteKey("hello there"))
	if got := next.(Model).draft.Value(); got != "hello there" {
		t.Errorf("text paste = %q, want it to land", got)
	}
}

// TestClearDraftCC: cc in NORMAL empties the composer and forgets the saved draft.
func TestClearDraftCC(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	next, _ := m.Update(pasteKey("junk to get rid of"))
	m = next.(Model)
	m.leaveInsert()
	m = Key(m, "c")
	m = Key(m, "c")
	if got := m.draft.Value(); got != "" {
		t.Errorf("cc should clear the composer, got %q", got)
	}
	if d := m.stateSnapshot().Drafts[m.activeID]; d != "" {
		t.Errorf("cc should drop the persisted draft, got %q", d)
	}
	// A lone c does nothing.
	m = Key(m, "i")
	next, _ = m.Update(pasteKey("keep me"))
	m = next.(Model)
	m.leaveInsert()
	m = Key(m, "c")
	m = Key(m, "j")
	if got := m.draft.Value(); got != "keep me" {
		t.Errorf("a lone c must not clear, got %q", got)
	}
}

// TestLooksBinary: table-driven over the raw classifier. Plain text — ASCII
// prose, CRLF/tabs, wide Unicode (emoji incl. a ZWJ sequence, CJK, RTL
// Arabic), ANSI color codes — must pass; a NUL byte or a heavy run of other
// control runes must not.
func TestLooksBinary(t *testing.T) {
	// looksBinary's premise is that unicode.IsControl is Cc-only, so joiners
	// and direction marks used in real text (ZWJ, RLM) are NOT control runes.
	// Assert that rather than assuming it.
	if unicode.IsControl('‍') { // ZERO WIDTH JOINER, glues emoji into one glyph
		t.Fatal("ZWJ must not be a Cc control rune, or looksBinary's premise breaks")
	}
	if unicode.IsControl('‏') { // RIGHT-TO-LEFT MARK
		t.Fatal("RLM must not be a Cc control rune, or looksBinary's premise breaks")
	}

	family := "\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466" // family emoji, ZWJ-joined
	cjk := "你好世界"                                           // "hello world" (CJK)
	arabic := "مرحبا"                                       // "hello" (Arabic, RTL)
	png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x01\x00\x00\x00\x01\x00\x08\x06\x00\x00\x00"

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false}, // 0*20 > 0 is false: the threshold can't misfire at len 0
		{"ascii prose", "the quick brown fox jumps over the lazy dog", false},
		{"crlf and tabs", "line one\r\nline two\twith a tab\r\n", false},
		{"emoji zwj cjk arabic", "hi " + family + " " + cjk + " " + arabic, false},
		{"ansi color", "\x1b[31mERROR\x1b[0m fix it", false}, // ESC is exempt
		{"png header", png, true},
		{"nul-free but control-heavy", "aaaaaaaaaaaaaaaaaaa\x08\x08", true}, // 2/21 runes > 5%
		{"lone backspace", "\x08", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksBinary([]rune(tc.input)); got != tc.want {
				t.Errorf("looksBinary(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestDropBinaryDrafts: a draft poisoned by a pre-guard binary paste is
// stripped from the persisted map; a clean draft is left alone.
func TestDropBinaryDrafts(t *testing.T) {
	drafts := map[string]string{
		"general": "just a normal note to self",
		"random":  "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x01\x00",
	}
	dropBinaryDrafts(drafts)
	if _, ok := drafts["random"]; ok {
		t.Error("the PNG-garbage draft should have been dropped")
	}
	if got := drafts["general"]; got != "just a normal note to self" {
		t.Errorf("the clean draft should be untouched, got %q", got)
	}
}

// TestPasteBinaryRejectedInOverlay: the binary-paste guard is hoisted above
// every overlay dispatch in Update — this is the regression test for that
// hoist. Find stands in for all overlays; they share the one guard.
func TestPasteBinaryRejectedInOverlay(t *testing.T) {
	m := newSized()
	m = Key(m, "/")
	if !m.findOpen {
		t.Fatal("/ should open the find overlay")
	}
	png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x01\x00\x00\x00\x01\x00\x08\x06\x00\x00\x00"
	next, _ := m.Update(pasteKey(png))
	m = next.(Model)
	if got := m.findInput.Value(); got != "" {
		t.Errorf("binary paste must not land in the find input, got %q", got)
	}
	if m.loadErr == nil {
		t.Error("binary paste should flash an error")
	}
	// …while ordinary text still lands in the overlay input.
	next, _ = m.Update(pasteKey("needle"))
	m = next.(Model)
	if got := m.findInput.Value(); got != "needle" {
		t.Errorf("text paste = %q, want it to land in the find input", got)
	}
}

// TestClearDraftThread: cc while the thread composer is focused clears only
// the thread draft — the main composer's draft and its persisted entry in
// m.drafts must survive untouched.
func TestClearDraftThread(t *testing.T) {
	m := newSized()
	m.draft.SetValue("don't touch me")
	m.drafts[m.activeID] = "don't touch me either"

	m.msgSel = 2 // e3, has replies (thread_test.go), though any message works here
	m = Key(m, "t")
	if !m.threadOpen() {
		t.Fatal("t should open the thread")
	}
	m = Key(m, "i")
	next, _ := m.Update(pasteKey("reply draft"))
	m = next.(Model)
	m.leaveInsert()

	m = Key(m, "c")
	m = Key(m, "c")

	if got := m.threadDraft.Value(); got != "" {
		t.Errorf("cc should clear the thread draft, got %q", got)
	}
	if got := m.draft.Value(); got != "don't touch me" {
		t.Errorf("cc in the thread must not touch the main composer, got %q", got)
	}
	if got := m.drafts[m.activeID]; got != "don't touch me either" {
		t.Errorf("cc in the thread must not touch the persisted draft, got %q", got)
	}
}

// TestPendingKeysDontCrossContaminate: the gPending/dPending/cPending resets
// must run before the switch in normalKey, not after — "g", "d" and "c" all
// return early from their own case, so a reset placed after the switch (the
// old layout) would never fire for those keys, letting a mismatched sequence
// like c,d,c be misread as the second half of a pair.
func TestPendingKeysDontCrossContaminate(t *testing.T) {
	m := newSized()
	m = Key(m, "i")
	next, _ := m.Update(pasteKey("do not wipe me"))
	m = next.(Model)
	m.leaveInsert()

	// c, d, c must NOT read as "cc" — the "d" in between should have
	// cancelled the pending c. Under the old (tail-reset) code this fired cc
	// and silently wiped the draft.
	m = Key(m, "c")
	m = Key(m, "d")
	m = Key(m, "c")
	if got := m.draft.Value(); got != "do not wipe me" {
		t.Errorf("stray c,d,c must not fire cc, draft = %q", got)
	}

	// g, d, g must NOT read as "gg" for the same reason — the "d" must
	// cancel the pending g, not silently return early and leave it live.
	m.msgSel = 3
	m = Key(m, "g")
	m = Key(m, "d")
	m = Key(m, "g")
	if m.msgSel != 3 {
		t.Errorf("stray g,d,g must not fire gg, msgSel = %d, want 3", m.msgSel)
	}
}
