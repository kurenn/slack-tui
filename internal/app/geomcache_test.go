package app

import (
	"strings"
	"testing"

	"github.com/kurenn/slack-tui/internal/data"
)

// TestGeomCacheHit: calling msgGeom twice without any mutation between
// returns identical starts/total and leaves the cache key unchanged.
func TestGeomCacheHit(t *testing.T) {
	m := newSized()

	_, _, total1, _ := m.msgGeom()
	key1 := m.geomCache.key
	starts1 := append([]int(nil), m.geomCache.starts...)

	_, _, total2, _ := m.msgGeom()
	key2 := m.geomCache.key

	if key1 != key2 {
		t.Fatalf("cache key changed between identical calls: %q → %q", key1, key2)
	}
	if total1 != total2 {
		t.Fatalf("total changed on second call: %d → %d", total1, total2)
	}
	if len(starts1) != len(m.geomCache.starts) {
		t.Fatalf("starts length changed: %d → %d", len(starts1), len(m.geomCache.starts))
	}
}

// TestGeomCacheInvalidation: appending a message bumps msgGen, so the next
// msgGeom call recomputes and the new total reflects the added message.
func TestGeomCacheInvalidation(t *testing.T) {
	m := newSized()

	_, _, totalBefore, _ := m.msgGeom()
	keyBefore := m.geomCache.key

	// Append a message via the same path the app uses (sendMessage appends +
	// invalidates; here we replicate the mutation + bump directly to stay hermetic).
	m.messages[m.activeID] = append(m.messages[m.activeID],
		data.Message{ID: "cache-test", UserID: "ada", Time: "10:00",
			Text: strings.Repeat("x", 80)}) // guaranteed to add lines
	m.invalidateGeom()

	_, _, totalAfter, _ := m.msgGeom()
	keyAfter := m.geomCache.key

	if keyBefore == keyAfter {
		t.Error("cache key should change after invalidation")
	}
	if totalAfter <= totalBefore {
		t.Errorf("total should grow after adding a message: before=%d after=%d", totalBefore, totalAfter)
	}
}

// TestGeomCacheWidthChange: resizing the terminal changes the center width,
// which is encoded in the key, so msgGeom recomputes automatically without
// an explicit invalidation.
func TestGeomCacheWidthChange(t *testing.T) {
	// Seed a long-line message so wrapping actually differs at different widths.
	m := newSized() // 100×30
	m.messages[m.activeID] = []data.Message{
		{ID: "wrap-test", UserID: "ada", Time: "09:00", Text: strings.Repeat("word ", 30)},
	}
	m.msgSel = 0

	_, _, totalWide, _ := m.msgGeom()
	keyWide := m.geomCache.key

	// Narrow the window — the same text wraps to more lines.
	m = WithSize(m, 60, 30)

	_, _, totalNarrow, _ := m.msgGeom()
	keyNarrow := m.geomCache.key

	if keyWide == keyNarrow {
		t.Error("cache key should differ at different widths")
	}
	if totalNarrow <= totalWide {
		t.Errorf("narrower width should produce more lines: wide=%d narrow=%d", totalWide, totalNarrow)
	}
}
