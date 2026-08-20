package theme

import "testing"

// smCharcoalFg/smCharcoalAccent/... are frozen literal hexes copied once from
// theme.go's rawThemes["charcoal"] table at the time this test was written.
// They are NOT recomputed from the implementation — this file's whole job is
// to notice if that table (or the code reading it) drifts, so these values
// intentionally do not track theme.go automatically. If a deliberate palette
// change breaks this test, update the literal here as part of that change.
const (
	smCharcoalBg     = "#0d1117"
	smCharcoalFg     = "#c9d1d9"
	smCharcoalAccent = "#56d4dd"
	smCharcoalBlue   = "#6cb6ff"
	smCharcoalGreen  = "#7ee787"
	smCharcoalPurple = "#c8a2ff"
	smCharcoalOrange = "#f0a868"
	smCharcoalCyan   = "#56d4dd"
	smCharcoalRed    = "#f47067"
	smCharcoalYellow = "#eac55f"

	smGreenAccent = "#7ee787" // accents["green"]
)

// blend must alpha-composite in plain (non-gamma-corrected) sRGB space: value
// = overlay*alpha + base*(1-alpha) per channel, rounded to the nearest byte.
// Expected hexes below were computed independently by hand (not by calling
// blend), so a regression in the formula — wrong alpha direction, swapped
// base/overlay, a gamma curve sneaking in — actually fails this test.
func TestBlend(t *testing.T) {
	tests := []struct {
		name, base, overlay string
		alpha               float64
		want                string
	}{
		// black + white at 50% is the textbook midpoint: (0+255)/2 = 127.5 -> 128 = 0x80.
		{"midpoint of black and white", "#000000", "#ffffff", 0.5, "#808080"},
		// Asymmetric channels + a non-half alpha, worked out by hand:
		//   R: 0x10=16, 0xa0=160 -> 160*.25+16*.75 = 40+12   = 52  = 0x34
		//   G: 0x20=32, 0xb0=176 -> 176*.25+32*.75 = 44+24   = 68  = 0x44
		//   B: 0x30=48, 0xc0=192 -> 192*.25+48*.75 = 48+36   = 84  = 0x54
		{"asymmetric channels, quarter alpha", "#102030", "#a0b0c0", 0.25, "#344454"},
		// alpha=0 must return base unchanged (no overlay contribution at all).
		{"zero alpha keeps base", "#123456", "#abcdef", 0, "#123456"},
		// alpha=1 must return overlay unchanged (base fully replaced).
		{"full alpha is pure overlay", "#123456", "#abcdef", 1, "#abcdef"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(blend(tc.base, tc.overlay, tc.alpha)); got != tc.want {
				t.Errorf("blend(%s, %s, %v) = %s, want %s", tc.base, tc.overlay, tc.alpha, got, tc.want)
			}
		})
	}
}

// An invalid hex must pass the base color through unchanged rather than
// panicking or silently producing black — blend is fed theme table data, and
// a typo'd hex there must degrade visibly, not crash the renderer.
func TestBlendInvalidHexPassesThroughBase(t *testing.T) {
	if got := string(blend("not-a-color", "#ffffff", 0.5)); got != "not-a-color" {
		t.Errorf("blend with invalid base = %s, want the base string unchanged", got)
	}
	if got := string(blend("#000000", "not-a-color", 0.5)); got != "#000000" {
		t.Errorf("blend with invalid overlay = %s, want the base unchanged", got)
	}
}

// An unknown theme name must fall back to charcoal rather than producing an
// empty/zero palette — this is what protects a typo'd or future-removed
// theme name in a stale prefs.json from breaking the whole app.
func TestResolveUnknownThemeFallsBackToCharcoal(t *testing.T) {
	p := Resolve("not-a-real-theme", "auto")
	if string(p.Fg) != smCharcoalFg {
		t.Errorf("Fg = %s, want charcoal's %s", p.Fg, smCharcoalFg)
	}
	if p.Name != "Charcoal" {
		t.Errorf("Name = %q, want Charcoal", p.Name)
	}
}

// A known theme must resolve its own table, not charcoal's — this is the
// case that would stay accidentally green if Resolve always fell through.
func TestResolveKnownTheme(t *testing.T) {
	p := Resolve("charcoal", "auto")
	if string(p.Bg) != smCharcoalBg || string(p.Fg) != smCharcoalFg || string(p.Accent) != smCharcoalAccent {
		t.Errorf("charcoal palette = {Bg:%s Fg:%s Accent:%s}, want {%s %s %s}",
			p.Bg, p.Fg, p.Accent, smCharcoalBg, smCharcoalFg, smCharcoalAccent)
	}
}

// An accent override must change only Accent — every other token stays the
// theme's own, so switching accents in Settings can't silently retint the
// whole palette.
func TestResolveAccentOverrideChangesOnlyAccent(t *testing.T) {
	base := Resolve("charcoal", "auto")
	withAccent := Resolve("charcoal", "green")

	if string(withAccent.Accent) != smGreenAccent {
		t.Errorf("Accent = %s, want the green accent %s", withAccent.Accent, smGreenAccent)
	}
	if withAccent.Fg != base.Fg || withAccent.Bg != base.Bg || withAccent.Blue != base.Blue ||
		withAccent.Green != base.Green || withAccent.SelBg != base.SelBg {
		t.Error("an accent override changed a non-accent token")
	}
}

// An unknown accent name (including "auto") must keep the theme's own accent
// rather than resolving to an empty color.
func TestResolveUnknownAccentKeepsThemeDefault(t *testing.T) {
	for _, accent := range []string{"auto", "not-a-real-accent", ""} {
		p := Resolve("charcoal", accent)
		if string(p.Accent) != smCharcoalAccent {
			t.Errorf("Resolve(charcoal, %q).Accent = %s, want the theme default %s", accent, p.Accent, smCharcoalAccent)
		}
	}
}

// Token maps the seven syntax-color keys to their palette fields and falls
// back to Fg for anything else — this is what tints usernames per person.
func TestPaletteToken(t *testing.T) {
	p := Resolve("charcoal", "auto")
	tests := map[string]string{
		"blue": smCharcoalBlue, "green": smCharcoalGreen, "purple": smCharcoalPurple,
		"orange": smCharcoalOrange, "cyan": smCharcoalCyan, "red": smCharcoalRed, "yellow": smCharcoalYellow,
	}
	for key, want := range tests {
		if got := string(p.Token(key)); got != want {
			t.Errorf("Token(%q) = %s, want %s", key, got, want)
		}
	}
	// Unknown/empty key and a color-key typo must both fall back to Fg, never
	// to an empty lipgloss.Color (which would render as no color at all).
	for _, key := range []string{"", "unknown", "Blue", "bl"} {
		if got := p.Token(key); got != p.Fg {
			t.Errorf("Token(%q) = %s, want Fg (%s)", key, got, p.Fg)
		}
	}
}

// ParseDensity/String round-trip, including the unknown-input fallback to
// comfortable — a corrupt prefs.json density value must not crash Resolve.
func TestDensityParseAndString(t *testing.T) {
	if ParseDensity("compact") != Compact {
		t.Error(`ParseDensity("compact") should be Compact`)
	}
	for _, in := range []string{"comfortable", "", "bogus", "COMPACT"} {
		if ParseDensity(in) != Comfortable {
			t.Errorf("ParseDensity(%q) should fall back to Comfortable, got %v", in, ParseDensity(in))
		}
	}
	if Comfortable.String() != "comfortable" || Compact.String() != "compact" {
		t.Errorf("String() round trip broke: comfortable=%q compact=%q", Comfortable.String(), Compact.String())
	}
}

// MsgGap is the actual rendered feature of density in a terminal (blank rows
// between message groups) — compact must be tighter than comfortable.
func TestDensityMsgGap(t *testing.T) {
	if Compact.MsgGap() != 0 {
		t.Errorf("Compact.MsgGap() = %d, want 0", Compact.MsgGap())
	}
	if Comfortable.MsgGap() != 1 {
		t.Errorf("Comfortable.MsgGap() = %d, want 1", Comfortable.MsgGap())
	}
}

// CycleFor(true) must prepend "omarchy" without mutating the shared Cycle
// slice — the classic append(Cycle, ...) aliasing bug would corrupt every
// later CycleFor(false) caller (e.g. Settings on a non-Omarchy machine) once
// a follow-desktop caller ran first.
func TestCycleForDoesNotAliasCycle(t *testing.T) {
	originalLen := len(Cycle)
	originalFirst := Cycle[0]

	withDesktop := CycleFor(true)
	if withDesktop[0] != OmarchyName {
		t.Errorf("CycleFor(true)[0] = %q, want %q", withDesktop[0], OmarchyName)
	}
	if len(withDesktop) != originalLen+1 {
		t.Errorf("CycleFor(true) length = %d, want %d", len(withDesktop), originalLen+1)
	}

	if len(Cycle) != originalLen || Cycle[0] != originalFirst {
		t.Fatalf("CycleFor(true) mutated the shared Cycle slice: len=%d first=%q (want len=%d first=%q)",
			len(Cycle), Cycle[0], originalLen, originalFirst)
	}

	withoutDesktop := CycleFor(false)
	if len(withoutDesktop) != originalLen || withoutDesktop[0] != originalFirst {
		t.Error("CycleFor(false) should return Cycle unchanged")
	}
}

// DisplayName covers both branches: the special "follow desktop" label, and
// falling through to a resolved theme's own Name for everything else.
func TestDisplayName(t *testing.T) {
	if got := DisplayName(OmarchyName); got != "Omarchy (follow desktop)" {
		t.Errorf("DisplayName(omarchy) = %q, want the follow-desktop label", got)
	}
	if got := DisplayName("charcoal"); got != "Charcoal" {
		t.Errorf("DisplayName(charcoal) = %q, want Charcoal", got)
	}
	// Unknown theme names fall through Resolve's charcoal fallback.
	if got := DisplayName("not-a-theme"); got != "Charcoal" {
		t.Errorf("DisplayName(unknown) = %q, want Charcoal (Resolve's fallback)", got)
	}
}
