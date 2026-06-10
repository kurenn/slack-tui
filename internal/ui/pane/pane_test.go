package pane

import (
	"strings"
	"testing"

	"github.com/kurenn/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderDimensions(t *testing.T) {
	p := theme.Resolve("charcoal", "auto")
	out := Render(p, Options{Title: "workspace", Right: "@me", Width: 28, Height: 6, Body: "a\nb"})
	lines := strings.Split(out, "\n")
	if len(lines) != 6 {
		t.Fatalf("want 6 lines, got %d", len(lines))
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 28 {
			t.Errorf("line %d width = %d, want 28", i, w)
		}
	}
}

func TestRenderDropsRightLabelWhenNarrow(t *testing.T) {
	p := theme.Resolve("charcoal", "auto")
	out := Render(p, Options{Title: "workspace", Right: "@monospace-labs", Width: 20, Height: 3, Body: ""})
	top := strings.Split(out, "\n")[0]
	if !strings.Contains(top, "workspace") {
		t.Errorf("title should survive in narrow pane, got %q", top)
	}
	if strings.Contains(top, "monospace-labs") {
		t.Errorf("right label should be dropped in narrow pane, got %q", top)
	}
}
