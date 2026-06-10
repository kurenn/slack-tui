package onboarding

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kurenn/slack-tui/internal/theme"
)

var stepKicker = map[string]string{
	"theme": "appearance · 01", "accent": "appearance · 02", "density": "layout · 03",
	"keyboard": "tutorial · 04", "status": "presence · 05",
}
var stepHead = map[string][2]string{
	"theme":    {"Choose a theme", "The whole client follows your choice. j/k previews each live."},
	"accent":   {"Accent color", "Highlights the focused pane, your cursor, and selections."},
	"density":  {"Message density", "How tightly messages pack into the pane."},
	"keyboard": {"Learn the keys", "Modal, like vim — NORMAL navigates, INSERT types. Clear each drill."},
	"status":   {"Set your presence", "How teammates see you — change it anytime from ⌃K."},
}

// wizHeading is the shared kicker + title + subtitle block for a wizard step.
func (m Model) wizHeading(p theme.Palette) string {
	w := m.contentW()
	hd := stepHead[m.step()]
	kicker := lipgloss.NewStyle().Foreground(p.Dim2).Render(stepKicker[m.step()])
	head := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(hd[0])
	sub := wrapStyled(lipgloss.NewStyle().Foreground(p.Dim), hd[1], w)
	return lipgloss.JoinVertical(lipgloss.Left, kicker, head, sub)
}

func (m Model) viewWizardBody(p theme.Palette) string {
	return lipgloss.JoinVertical(lipgloss.Left, m.wizHeading(p), "", m.viewOptions(p, m.contentW()))
}

func (m Model) viewOptions(p theme.Palette, w int) string {
	switch m.step() {
	case "theme":
		return m.themeCards(p, w)
	case "accent":
		return m.accentCards(p, w)
	case "density":
		return m.densityCards(p, w)
	case "status":
		return m.statusList(p, w)
	}
	return ""
}

// ── theme: 3-column grid of cards, each with a header and a swatch strip ──────

func (m Model) themeCards(p theme.Palette, w int) string {
	var cards []string
	for i, o := range themeOpts {
		cards = append(cards, m.themeCard(p, i, o.name, o.val, i == m.optSel))
	}
	return cardGrid(cards, 3, cardInner(w, 3))
}

func (m Model) themeCard(p theme.Palette, i int, name, val string, sel bool) string {
	inner := cardInner(m.contentW(), 3)
	header := cardHeader(p, i, name, sel, inner)
	swatch := swatchStrip([]lipgloss.Color{
		theme.Resolve(val, "auto").Blue, theme.Resolve(val, "auto").Green,
		theme.Resolve(val, "auto").Purple, theme.Resolve(val, "auto").Orange,
	}, inner, 3)
	return cardBox(p, sel, inner, header+"\n"+swatch)
}

// ── accent: 3-column grid of chip cards ──────────────────────────────────────

func (m Model) accentCards(p theme.Palette, w int) string {
	var cards []string
	inner := cardInner(w, 3)
	for i, o := range accentOpts {
		num := keycap(p, fmt.Sprintf("%d", i+1))
		chip := lipgloss.NewStyle().Foreground(accentColor(p, o.val)).Render("●")
		name := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(o.name)
		left := num + " " + chip + " " + name
		check := " "
		if i == m.optSel {
			check = lipgloss.NewStyle().Foreground(p.Accent).Render("◉")
		}
		gap := inner - lipgloss.Width(left) - 1
		if gap < 1 {
			gap = 1
		}
		cards = append(cards, cardBox(p, i == m.optSel, inner, left+strings.Repeat(" ", gap)+check))
	}
	return cardGrid(cards, 3, inner)
}

func accentColor(p theme.Palette, val string) lipgloss.Color {
	switch val {
	case "cyan":
		return lipgloss.Color("#56d4dd")
	case "green":
		return lipgloss.Color("#7ee787")
	case "purple":
		return lipgloss.Color("#c8a2ff")
	case "orange":
		return lipgloss.Color("#f0a868")
	case "magenta":
		return lipgloss.Color("#ff7bd5")
	default:
		return p.Accent
	}
}

// ── density: 2 cards with a mini message demo ────────────────────────────────

func (m Model) densityCards(p theme.Palette, w int) string {
	inner := cardInner(w, 2)
	demo := func(gap bool) string {
		var rows []string
		for j, u := range []struct{ name, color string }{{"ada.k", "purple"}, {"lin.z", "green"}, {"marco", "orange"}} {
			t := lipgloss.NewStyle().Foreground(p.Dim2).Render(fmt.Sprintf("09:2%d ", j))
			n := lipgloss.NewStyle().Foreground(p.Token(u.color)).Bold(true).Render(u.name)
			rows = append(rows, t+n+lipgloss.NewStyle().Foreground(p.Dim).Render(" short message"))
			if gap {
				rows = append(rows, "")
			}
		}
		return strings.Join(rows, "\n")
	}
	contents := make([]string, len(densityOpts))
	maxLines := 0
	for i, o := range densityOpts {
		contents[i] = cardHeader(p, i, o.name, i == m.optSel, inner) + "\n\n" + demo(o.val == "comfortable")
		if n := strings.Count(contents[i], "\n"); n > maxLines {
			maxLines = n
		}
	}
	var cards []string
	for i := range densityOpts {
		body := contents[i] + strings.Repeat("\n", maxLines-strings.Count(contents[i], "\n"))
		cards = append(cards, cardBox(p, i == m.optSel, inner, body))
	}
	return cardGrid(cards, 2, inner)
}

// ── status: a clean presence list ────────────────────────────────────────────

func (m Model) statusList(p theme.Palette, w int) string {
	var rows []string
	for i, o := range statusOpts {
		if i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, m.statusRow(p, i, o.val, o.label, o.desc, i == m.optSel, w))
	}
	return strings.Join(rows, "\n")
}

func (m Model) statusRow(p theme.Palette, i int, val, label, desc string, sel bool, w int) string {
	dot := lipgloss.NewStyle().Foreground(presenceColor(p, val)).Render("●")
	name := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(label)
	d := lipgloss.NewStyle().Foreground(p.Dim).Render(desc)
	key := keycap(p, fmt.Sprintf("%d", i+1))

	left := dot + "  " + name + "   " + d
	gap := (w - 4) - lipgloss.Width(left) - lipgloss.Width(key)
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + key

	border := p.Border
	if sel {
		border = p.Accent
	}
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).Width(w-2).Padding(0, 1)
	return box.Render(line)
}

// ── card primitives ──────────────────────────────────────────────────────────

// cardInner returns the inner width of one card given total width and column count.
func cardInner(w, cols int) int {
	gap := 2
	return (w-gap*(cols-1))/cols - 2 // -2 for the card border
}

func cardHeader(p theme.Palette, i int, name string, sel bool, inner int) string {
	num := keycap(p, fmt.Sprintf("%d", i+1))
	nm := lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(name)
	left := num + " " + nm
	check := " "
	if sel {
		check = lipgloss.NewStyle().Foreground(p.Accent).Render("◉")
	}
	gap := inner - lipgloss.Width(left) - 1
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + check
}

// keycap renders a small "key" chip on a faint background, echoing the design's
// boxed option numbers.
func keycap(p theme.Palette, s string) string {
	return lipgloss.NewStyle().Foreground(p.Dim).Background(p.SelBg).Padding(0, 1).Render(s)
}

func cardBox(p theme.Palette, sel bool, inner int, content string) string {
	border := p.Border
	if sel {
		border = p.Accent
	}
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).
		Width(inner).Render(content)
}

// swatchStrip renders `rows` rows of solid color blocks filling width w.
func swatchStrip(cols []lipgloss.Color, w, rows int) string {
	widths := distribute(w, len(cols))
	var row strings.Builder
	for j, c := range cols {
		row.WriteString(lipgloss.NewStyle().Background(c).Render(strings.Repeat(" ", widths[j])))
	}
	line := row.String()
	out := make([]string, rows)
	for i := range out {
		out[i] = line
	}
	return strings.Join(out, "\n")
}

// cardGrid arranges cards into rows of `cols`, separated by a blank line.
func cardGrid(cards []string, cols, inner int) string {
	var rows []string
	for i := 0; i < len(cards); i += cols {
		end := i + cols
		if end > len(cards) {
			end = len(cards)
		}
		var seg []string
		for j := i; j < end; j++ {
			if j > i {
				seg = append(seg, "  ")
			}
			seg = append(seg, cards[j])
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, seg...))
	}
	// interleave a blank line between card rows
	var out []string
	for i, r := range rows {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, r)
	}
	return strings.Join(out, "\n")
}

func distribute(w, n int) []int {
	if n <= 0 {
		return nil
	}
	base, rem := w/n, w%n
	out := make([]int, n)
	for i := range out {
		out[i] = base
		if i < rem {
			out[i]++
		}
	}
	return out
}

func (m Model) stepRail(p theme.Palette) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(p.Dim2).Render("["))
	for i := range wizSteps {
		if i > 0 {
			c := p.Dim2
			if i <= m.stepIndex {
				c = p.Accent
			}
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render("──"))
		}
		glyph, c := "○", p.Dim2
		if i <= m.stepIndex {
			glyph, c = "◉", p.Accent
		}
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render(glyph))
	}
	b.WriteString(lipgloss.NewStyle().Foreground(p.Dim2).Render("]"))
	return b.String()
}

func (m Model) viewFooter(p theme.Palette, w int) string {
	var back string
	if m.stepIndex > 0 {
		back = lipgloss.NewStyle().Foreground(p.Dim).Render("← back ") + lipgloss.NewStyle().Foreground(p.Dim2).Render("esc")
	}
	label := "continue ↵"
	if m.stepIndex == len(wizSteps)-1 {
		label = "finish ↵"
	}
	button := lipgloss.NewStyle().Background(p.Accent).Foreground(p.Bg).Bold(true).Padding(0, 1).Render(label)
	cont := button
	if m.step() == "keyboard" && !m.kbDone {
		cont = lipgloss.NewStyle().Foreground(p.Dim2).Render("complete the drills to continue") + "   " + button
	}
	gap := w - lipgloss.Width(back) - lipgloss.Width(cont)
	if gap < 1 {
		gap = 1
	}
	return back + strings.Repeat(" ", gap) + cont
}

func presenceColor(p theme.Palette, status string) lipgloss.Color {
	switch status {
	case "online":
		return p.Green
	case "away":
		return p.Yellow
	case "dnd":
		return p.Red
	default:
		return p.Dim2
	}
}

func wrapStyled(s lipgloss.Style, text string, w int) string {
	return s.Width(w).Render(text)
}
