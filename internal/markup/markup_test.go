package markup

import (
	"strings"
	"testing"

	"github.com/kurenn/slack-tui/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

func TestInlinePreservesText(t *testing.T) {
	p := theme.Resolve("charcoal", "auto")
	got := ansi.Strip(Inline(p, "hey @you see #design and `code` at https://x.io"))
	want := "hey @you see #design and code at https://x.io"
	if got != want {
		t.Errorf("stripped inline = %q, want %q", got, want)
	}
}

func TestRenderFencedBlock(t *testing.T) {
	p := theme.Resolve("charcoal", "auto")
	got := ansi.Strip(Render(p, "before\n```\nx := 1\n```\nafter"))
	if !strings.Contains(got, "x := 1") {
		t.Errorf("fenced content missing: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("surrounding text missing: %q", got)
	}
}
