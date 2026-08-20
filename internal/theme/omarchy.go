package theme

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// OmarchyName is the theme name that means "follow whatever Omarchy is set to"
// rather than one of the built-in palettes.
const OmarchyName = "omarchy"

// omarchyColorsPath is where Omarchy publishes the active theme's colors. It's
// a symlinked state directory rewritten on every `omarchy theme set`, so
// re-reading it is all that's needed to follow a theme change.
func omarchyColorsPath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "omarchy", "current", "theme", "colors.toml")
}

// OmarchyAvailable reports whether an Omarchy theme is readable here, which is
// what decides whether the onboarding wizard bothers asking about colors.
func OmarchyAvailable() bool {
	_, ok := readOmarchyColors()
	return ok
}

// OmarchyStamp changes whenever the active theme does. It's the size+mtime of
// colors.toml rather than its contents: cheap enough to check on a timer, since
// a running client has no other way to learn the desktop re-themed (the
// theme-set hook would need per-machine installation).
func OmarchyStamp() string {
	fi, err := os.Stat(omarchyColorsPath())
	if err != nil {
		return ""
	}
	return strconv.FormatInt(fi.Size(), 10) + ":" + strconv.FormatInt(fi.ModTime().UnixNano(), 10)
}

// readOmarchyColors parses Omarchy's colors.toml. It is a flat file of
// `key = "#hex"` lines, so it's read directly rather than taking on a TOML
// dependency for one file — CONTRIBUTING asks for a very good reason before
// adding one, and this isn't it.
func readOmarchyColors() (map[string]string, bool) {
	p := omarchyColorsPath()
	if p == "" {
		return nil, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	if out["background"] == "" || out["foreground"] == "" {
		return nil, false // not a colors.toml we understand
	}
	return out, true
}

// omarchyRaw maps Omarchy's palette onto the same rawTheme the built-in themes
// use, so the derived colors (selection, mention tint, badges) come out of the
// one blending path instead of a parallel one.
func omarchyRaw(c map[string]string) rawTheme {
	// pick returns the first key that's actually set, so a theme omitting an
	// optional color degrades to a sensible neighbour rather than to black.
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v := c[k]; v != "" {
				return v
			}
		}
		return c["foreground"]
	}
	return rawTheme{
		name:     "Omarchy",
		bg:       pick("background"),
		panel:    pick("background"),
		titlebar: pick("darker_background", "dark_background", "background"),
		border:   pick("muted", "lighter_background"),
		fg:       pick("foreground", "bright_foreground"),
		dim:      pick("dark_foreground", "muted"),
		dim2:     pick("muted", "dark_foreground"),
		accent:   pick("accent", "blue"),
		blue:     pick("blue"),
		green:    pick("green"),
		purple:   pick("magenta", "purple"),
		orange:   pick("orange", "yellow"),
		cyan:     pick("cyan"),
		red:      pick("red"),
		yellow:   pick("yellow"),
		// selection is a finished colour, not an overlay, so it's blended at
		// full alpha; the mention tint stays a light wash of the accent.
		selOverlay:     pick("selection", "lighter_background"),
		mentionOverlay: pick("accent", "blue"),
		sAlpha:         1.0,
		mAlpha:         0.14,
	}
}

// OmarchyPalette builds the palette from the desktop's active theme.
func OmarchyPalette() (Palette, bool) {
	c, ok := readOmarchyColors()
	if !ok {
		return Palette{}, false
	}
	return paletteFrom(omarchyRaw(c), c["accent"]), true
}
