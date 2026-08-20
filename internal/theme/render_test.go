package theme

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// FillBg's whole documented purpose is re-asserting the background sequence
// after every embedded reset, so a styled span ending mid-line doesn't leave
// a ragged unpainted gap behind it. These inputs are built from literal
// escape strings (not lipgloss.NewStyle().Render(...)) so the test doesn't
// depend on lipgloss's color-profile detection — bgSeq itself is profile-
// independent (it hand-builds \x1b[48;2;… with fmt.Sprintf).
func TestFillBgReassertsBackgroundAfterEveryReset(t *testing.T) {
	// Two styled spans, each terminated by an SGR reset — as lipgloss would
	// emit for "colored-word plain-word colored-word".
	in := "\x1b[38;2;255;0;0mred\x1b[0m plain \x1b[38;2;0;255;0mgreen\x1b[0m"
	out := FillBg(in, 30, lipgloss.Color("#112233"))

	wantSeq := bgSeq(lipgloss.Color("#112233"))
	if wantSeq == "" {
		t.Fatal("bgSeq produced no sequence for a valid hex — test setup is broken")
	}
	if !strings.HasPrefix(out, wantSeq) {
		t.Fatalf("output does not open with the background sequence:\n%q", out)
	}
	// Every reset in the input except a genuinely final one must be
	// immediately followed by the background sequence, or the paint breaks.
	resets := strings.Count(in, "\x1b[0m")
	reassertions := strings.Count(out, "\x1b[0m"+wantSeq)
	if reassertions != resets {
		t.Errorf("found %d reset-then-repaint sequences, want %d (one per embedded reset):\n%q",
			reassertions, resets, out)
	}
	// The whole thing terminates with a final reset (no bleeding into the next line).
	if !strings.HasSuffix(out, "\x1b[0m") {
		t.Errorf("output should end with a final reset, got:\n%q", out)
	}
}

// The printable width of the output must equal the requested width whether
// the input is shorter (padded) or longer (truncated) than it — this is what
// makes FillBg usable to paint a fixed-width row.
func TestFillBgWidthInvariant(t *testing.T) {
	bg := lipgloss.Color("#223344")
	short := FillBg("hi", 10, bg)
	if got := lipgloss.Width(stripAllEscapes(short)); got != 10 {
		t.Errorf("short input: printable width = %d, want 10; output=%q", got, short)
	}

	long := FillBg("this is way too long for the box", 10, bg)
	if got := lipgloss.Width(stripAllEscapes(long)); got != 10 {
		t.Errorf("overlong input: printable width = %d, want 10; output=%q", got, long)
	}
}

// stripAllEscapes removes the SGR sequences FillBg emits by hand (bgSeq +
// resets), so the padded/truncated *text* width can be measured independent
// of them — ansi.Strip alone won't catch our own literal \x1b[48;2;…m runs
// mixed with lipgloss.Width's assumptions, so do it explicitly and simply.
func stripAllEscapes(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// An invalid color must degrade to plain padded text with no escape
// sequences at all — bgSeq returning "" is the signal FillBg uses to skip
// painting rather than emit a broken/empty color sequence.
func TestFillBgInvalidColorIsPlainText(t *testing.T) {
	out := FillBg("hi", 5, lipgloss.Color("not-a-color"))
	if out != "hi   " {
		t.Errorf("FillBg with an invalid color = %q, want plain padded text %q", out, "hi   ")
	}
	if strings.Contains(out, "\x1b") {
		t.Errorf("FillBg with an invalid color should emit no escapes, got %q", out)
	}
}

// bgSeq must hand-build a truecolor background-set sequence from the color's
// RGB bytes — independently computed here from the hex digits, not derived
// from colorful's own arithmetic.
func TestBgSeq(t *testing.T) {
	if got := bgSeq(lipgloss.Color("#ff8000")); got != "\x1b[48;2;255;128;0m" {
		t.Errorf("bgSeq(#ff8000) = %q, want \\x1b[48;2;255;128;0m", got)
	}
	if got := bgSeq(lipgloss.Color("bogus")); got != "" {
		t.Errorf("bgSeq(invalid) = %q, want empty", got)
	}
}
