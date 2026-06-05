package app

import "testing"

func TestPaletteOpenClose(t *testing.T) {
	m := newSized()
	m = Key(m, "ctrl+k")
	if !m.paletteOpen {
		t.Fatal("ctrl+k should open the palette")
	}
	m = Key(m, "esc")
	if m.paletteOpen {
		t.Error("esc should close the palette")
	}
}

func TestPaletteFilterAndJump(t *testing.T) {
	m := newSized()
	m = Key(m, "ctrl+k")
	m = Key(m, "random")
	got := m.filteredPalette()
	if len(got) != 1 || got[0].id != "ch:random" {
		t.Fatalf("filter 'random' = %v, want single ch:random", ids(got))
	}
	m = Key(m, "enter")
	if m.paletteOpen {
		t.Error("running an item should close the palette")
	}
	if m.activeID != "random" {
		t.Errorf("activeID = %q, want random", m.activeID)
	}
}

func TestPaletteToggleHints(t *testing.T) {
	m := newSized()
	before := m.showHints
	m = Key(m, "ctrl+k")
	m = Key(m, "hide key")
	m = Key(m, "enter")
	if m.showHints == before {
		t.Errorf("showHints should toggle, still %v", m.showHints)
	}
}

func TestPaletteCycleTheme(t *testing.T) {
	m := newSized()
	before := m.prefs.Theme
	m = Key(m, "ctrl+k")
	m = Key(m, "cycle theme")
	m = Key(m, "enter")
	if m.prefs.Theme == before {
		t.Errorf("theme should change from %q", before)
	}
	if m.pal.Name == "" {
		t.Error("palette should re-resolve after theme change")
	}
}

func ids(items []palItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}
