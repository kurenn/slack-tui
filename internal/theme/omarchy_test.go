package theme

import (
	"os"
	"path/filepath"
	"testing"
)

// fixture points the desktop-theme lookup at a colors.toml we control.
func fixture(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	themeDir := filepath.Join(dir, "omarchy", "current", "theme")
	if err := os.MkdirAll(themeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(filepath.Join(themeDir, "colors.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_STATE_HOME", dir)
}

const tokyoNight = `mode = "dark"

accent = "#7aa2f7"
selection = "#292e42"
muted = "#414868"
background = "#1a1b26"
darker_background = "#0e0e14"
foreground = "#a9b1d6"
dark_foreground = "#565f89"
red = "#f7768e"
green = "#9ece6a"
magenta = "#ad8ee6"
`

func TestOmarchyPaletteMapsColors(t *testing.T) {
	fixture(t, tokyoNight)
	p := Resolve(OmarchyName, "auto")
	for _, tc := range []struct{ name, got, want string }{
		{"bg", string(p.Bg), "#1a1b26"},
		{"titlebar", string(p.TitlebarBg), "#0e0e14"},
		{"fg", string(p.Fg), "#a9b1d6"},
		{"dim", string(p.Dim), "#565f89"},
		{"border", string(p.Border), "#414868"},
		{"accent", string(p.Accent), "#7aa2f7"},
		{"purple from magenta", string(p.Purple), "#ad8ee6"},
		// selection is a finished colour, so it must survive blending intact
		// rather than being washed out like the mention tint.
		{"selection", string(p.SelBg), "#292e42"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, tc.got, tc.want)
		}
	}
}

// A light theme must come through as-is; nothing may assume a dark background.
func TestOmarchyPaletteLightMode(t *testing.T) {
	fixture(t, `mode = "light"
background = "#eff1f5"
foreground = "#4c4f69"
accent = "#1e66f5"
`)
	p := Resolve(OmarchyName, "auto")
	if string(p.Bg) != "#eff1f5" || string(p.Accent) != "#1e66f5" {
		t.Errorf("light palette not applied: bg=%s accent=%s", p.Bg, p.Accent)
	}
}

// Most machines don't run Omarchy — asking for it there must fall back to a
// built-in rather than rendering an empty palette.
func TestOmarchyFallsBackWhenAbsent(t *testing.T) {
	fixture(t, "")
	if OmarchyAvailable() {
		t.Fatal("should not detect a theme that isn't there")
	}
	p := Resolve(OmarchyName, "auto")
	if p.Name != rawThemes["charcoal"].name {
		t.Errorf("fell back to %q, want charcoal", p.Name)
	}
	if p.Bg == "" || p.Fg == "" {
		t.Error("fallback palette must still be fully populated")
	}
}

// A theme file missing optional keys must degrade, not produce empty colors.
func TestOmarchyPartialThemeDegrades(t *testing.T) {
	fixture(t, `background = "#101010"
foreground = "#e0e0e0"
`)
	p := Resolve(OmarchyName, "auto")
	for name, c := range map[string]string{
		"accent": string(p.Accent), "border": string(p.Border), "dim": string(p.Dim),
		"blue": string(p.Blue), "red": string(p.Red), "selBg": string(p.SelBg),
	} {
		if c == "" {
			t.Errorf("%s is empty on a minimal theme", name)
		}
	}
}

// The stamp is what a running client watches; it must move when the theme does.
func TestOmarchyStampTracksChanges(t *testing.T) {
	fixture(t, tokyoNight)
	first := OmarchyStamp()
	if first == "" {
		t.Fatal("stamp should be set when a theme is present")
	}
	fixture(t, tokyoNight+"\nblue = \"#000080\"\n")
	if OmarchyStamp() == first {
		t.Error("stamp must change when colors.toml does")
	}
}
