// Package theme holds the five color palettes, accent overrides, and density
// settings ported from the design handoff's CSS custom properties. All theming
// in the prototype is driven by `data-theme`; here it's a Palette value passed
// down to the views.
package theme

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// Density maps the handoff's comfortable/compact density. In a terminal the
// only meaningful analog of the px line/padding vars is how many blank rows sit
// between message rows.
type Density int

const (
	Comfortable Density = iota
	Compact
)

// MsgGap is the number of blank rows rendered between message rows.
func (d Density) MsgGap() int {
	if d == Compact {
		return 0
	}
	return 1
}

func ParseDensity(s string) Density {
	if s == "compact" {
		return Compact
	}
	return Comfortable
}

func (d Density) String() string {
	if d == Compact {
		return "compact"
	}
	return "comfortable"
}

// Palette is one resolved theme: every CSS custom property the views need, as
// concrete lipgloss colors. Selection/mention/code backgrounds are pre-composited
// from their rgba() overlays to solid colors, since terminals can't alpha-blend.
type Palette struct {
	Name string

	Bg         lipgloss.Color
	Panel      lipgloss.Color
	TitlebarBg lipgloss.Color
	StatusBg   lipgloss.Color
	Border     lipgloss.Color
	Fg         lipgloss.Color
	Dim        lipgloss.Color
	Dim2       lipgloss.Color
	Accent     lipgloss.Color

	SelBg     lipgloss.Color
	MentionBg lipgloss.Color
	CodeBg    lipgloss.Color
	BadgeBg   lipgloss.Color

	Blue   lipgloss.Color
	Green  lipgloss.Color
	Purple lipgloss.Color
	Orange lipgloss.Color
	Cyan   lipgloss.Color
	Red    lipgloss.Color
	Yellow lipgloss.Color
}

// Token returns the syntax-token color for a user/message color key
// (blue/green/purple/orange/cyan/red/yellow), falling back to Fg.
func (p Palette) Token(key string) lipgloss.Color {
	switch key {
	case "blue":
		return p.Blue
	case "green":
		return p.Green
	case "purple":
		return p.Purple
	case "orange":
		return p.Orange
	case "cyan":
		return p.Cyan
	case "red":
		return p.Red
	case "yellow":
		return p.Yellow
	default:
		return p.Fg
	}
}

// rawTheme is the hand-transcribed token table from README "Design Tokens".
// An empty panel/titlebar/status field inherits the documented default.
type rawTheme struct {
	name                                               string
	bg, panel, titlebar, border, fg, dim, dim2, accent string
	blue, green, purple, orange, cyan, red, yellow     string
	selOverlay, mentionOverlay                         string
	sAlpha, mAlpha                                     float64
}

// Per-theme rgba() overlays for the selection / mention backgrounds, taken from
// the <style> blocks (charcoal defaults unless a theme overrides them).
var rawThemes = map[string]rawTheme{
	"charcoal": {
		name: "Charcoal",
		bg:   "#0d1117", panel: "#0d1117", titlebar: "#090c11", border: "#262d38",
		fg: "#c9d1d9", dim: "#7a838f", dim2: "#4d5663", accent: "#56d4dd",
		blue: "#6cb6ff", green: "#7ee787", purple: "#c8a2ff", orange: "#f0a868",
		cyan: "#56d4dd", red: "#f47067", yellow: "#eac55f",
		selOverlay: "#7e8a99", sAlpha: 0.14, mentionOverlay: "#f0a868", mAlpha: 0.09,
	},
	"midnight": {
		name: "Midnight",
		bg:   "#0a0e1a", panel: "#0a0e1a", titlebar: "#070a14", border: "#1d2740",
		fg: "#cdd6f4", dim: "#7986b3", dim2: "#454f73", accent: "#c8a2ff",
		blue: "#89b4fa", green: "#a6e3a1", purple: "#cba6f7", orange: "#fab387",
		cyan: "#94e2d5", red: "#f38ba8", yellow: "#f9e2af",
		selOverlay: "#8996c8", sAlpha: 0.14, mentionOverlay: "#fab387", mAlpha: 0.09,
	},
	"phosphor": {
		name: "Phosphor",
		bg:   "#04130a", panel: "#04130a", titlebar: "#020c06", border: "#12361f",
		fg: "#5ff08a", dim: "#2f9456", dim2: "#1d5e37", accent: "#5ff08a",
		blue: "#54d6c2", green: "#8df0a0", purple: "#c5e89a", orange: "#f0c868",
		cyan: "#5ff0c2", red: "#f08a6a", yellow: "#e8e07a",
		selOverlay: "#5ff08a", sAlpha: 0.12, mentionOverlay: "#f0c868", mAlpha: 0.10,
	},
	"solarized": {
		name: "Solarized",
		bg:   "#002b36", panel: "#002b36", titlebar: "#00212b", border: "#0c3a47",
		fg: "#93a1a1", dim: "#5d7479", dim2: "#3f565c", accent: "#2aa198",
		blue: "#268bd2", green: "#859900", purple: "#6c71c4", orange: "#cb4b16",
		cyan: "#2aa198", red: "#dc322f", yellow: "#b58900",
		selOverlay: "#586e75", sAlpha: 0.20, mentionOverlay: "#cb4b16", mAlpha: 0.12,
	},
	"paper": {
		name: "Paper",
		bg:   "#f7f3e9", panel: "#fbf8f0", titlebar: "#efe9da", border: "#ddd4bf",
		fg: "#2c2a24", dim: "#8a8270", dim2: "#b3aa92", accent: "#1f7a8c",
		blue: "#2563a8", green: "#4d7c2f", purple: "#7c3a9e", orange: "#b5630a",
		cyan: "#0f7f8f", red: "#c0392b", yellow: "#8a6d00",
		selOverlay: "#1f7a8c", sAlpha: 0.10, mentionOverlay: "#b5630a", mAlpha: 0.10,
	},
}

// Accents are the theme-independent accent overrides. "auto" keeps the theme's own.
var accents = map[string]string{
	"cyan":    "#56d4dd",
	"green":   "#7ee787",
	"purple":  "#c8a2ff",
	"orange":  "#f0a868",
	"magenta": "#ff7bd5",
	"blue":    "#6cb6ff",
}

// Cycle is the theme order used by the "Cycle theme" palette command.
var Cycle = []string{"charcoal", "midnight", "phosphor", "solarized", "paper"}

// blend alpha-composites overlay over base and returns the solid hex result.
func blend(base, overlay string, alpha float64) lipgloss.Color {
	b, err1 := colorful.Hex(base)
	o, err2 := colorful.Hex(overlay)
	if err1 != nil || err2 != nil {
		return lipgloss.Color(base)
	}
	r := o.R*alpha + b.R*(1-alpha)
	g := o.G*alpha + b.G*(1-alpha)
	bl := o.B*alpha + b.B*(1-alpha)
	return lipgloss.Color(colorful.Color{R: r, G: g, B: bl}.Clamped().Hex())
}

// Resolve builds the Palette for a theme name with an accent override applied.
// Unknown theme falls back to charcoal; unknown/"auto" accent keeps theme default.
func Resolve(themeName, accent string) Palette {
	rt, ok := rawThemes[themeName]
	if !ok {
		rt = rawThemes["charcoal"]
	}
	accentHex := rt.accent
	if a, ok := accents[accent]; ok {
		accentHex = a
	}
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }
	return Palette{
		Name:       rt.name,
		Bg:         c(rt.bg),
		Panel:      c(rt.panel),
		TitlebarBg: c(rt.titlebar),
		StatusBg:   c(rt.titlebar),
		Border:     c(rt.border),
		Fg:         c(rt.fg),
		Dim:        c(rt.dim),
		Dim2:       c(rt.dim2),
		Accent:     c(accentHex),
		SelBg:      blend(rt.panel, rt.selOverlay, rt.sAlpha),
		MentionBg:  blend(rt.bg, rt.mentionOverlay, rt.mAlpha),
		CodeBg:     blend(rt.panel, rt.selOverlay, rt.sAlpha),
		BadgeBg:    blend(rt.panel, rt.selOverlay, 0.26),
		Blue:       c(rt.blue),
		Green:      c(rt.green),
		Purple:     c(rt.purple),
		Orange:     c(rt.orange),
		Cyan:       c(rt.cyan),
		Red:        c(rt.red),
		Yellow:     c(rt.yellow),
	}
}
