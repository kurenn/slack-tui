package onboarding

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/abrahamkuri/slack-tui/internal/theme"
)

// tline is one typewriter line. When fill is set, it renders as
// `text ......... ok` — the label, a dotted fill (dim2), then the status word
// (okClass). Otherwise it's a single-class line.
type tline struct {
	text    string
	class   string // accent | ok | dim | fill | fg
	fill    bool
	ok      string
	okClass string
}

func (l tline) visLen() int {
	n := len([]rune(l.text))
	if l.fill {
		n += dotsLen(l.text) + len([]rune(l.ok))
	}
	return n
}

func dotsLen(label string) int {
	d := 34 - len([]rune(label))
	if d < 4 {
		d = 4
	}
	return d
}

// typewriter reveals a block of lines character-by-character.
type typewriter struct {
	lines []tline
	total int
	pos   int
	done  bool
}

func newTypewriter(lines []tline) typewriter {
	total := 0
	for _, l := range lines {
		total += l.visLen() + 1
	}
	return typewriter{lines: lines, total: total}
}

func (t *typewriter) step() {
	if t.pos < t.total {
		t.pos++
	}
	if t.pos >= t.total {
		t.done = true
	}
}

func (t *typewriter) fastForward() { t.pos = t.total; t.done = true }

func bootColor(p theme.Palette, class string) lipgloss.Style {
	s := lipgloss.NewStyle()
	switch class {
	case "accent":
		return s.Foreground(p.Accent)
	case "ok":
		return s.Foreground(p.Green)
	case "fill":
		return s.Foreground(p.Dim2)
	case "dim":
		return s.Foreground(p.Dim)
	default:
		return s.Foreground(p.Fg)
	}
}

// renderBoot renders the first n visible characters of a line, styled.
func renderBoot(p theme.Palette, l tline, n int) string {
	cl := bootColor(p, l.class)
	runes := []rune(l.text)
	if !l.fill {
		if n >= len(runes) {
			return cl.Render(l.text)
		}
		return cl.Render(string(runes[:n]))
	}
	if n <= len(runes) {
		return cl.Render(string(runes[:n]))
	}
	out := cl.Render(l.text)
	rem := n - len(runes)
	dots := " " + strings.Repeat(".", dotsLen(l.text)-2) + " "
	d2 := lipgloss.NewStyle().Foreground(p.Dim2)
	if rem <= len(dots) {
		return out + d2.Render(dots[:rem])
	}
	out += d2.Render(dots)
	rem -= len(dots)
	okr := []rune(l.ok)
	if rem > len(okr) {
		rem = len(okr)
	}
	return out + bootColor(p, l.okClass).Render(string(okr[:rem]))
}

func (t typewriter) render(p theme.Palette) string {
	cursor := lipgloss.NewStyle().Foreground(p.Accent).Render("▋")
	var out []string
	remaining := t.pos
	for _, l := range t.lines {
		vl := l.visLen()
		atCursor := false
		show := vl
		if remaining < vl {
			show, atCursor = remaining, true
		}
		line := renderBoot(p, l, max0(show))
		if atCursor && !t.done {
			line += cursor
		}
		out = append(out, line)
		if atCursor && !t.done {
			break
		}
		remaining -= vl + 1
		if remaining < 0 && !t.done {
			break
		}
	}
	return strings.Join(out, "\n")
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// banner is the shared wordmark shown atop every boot-family screen.
func banner(p theme.Palette) string {
	a := lipgloss.NewStyle().Foreground(p.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(p.Dim)
	l1 := a.Render("╶──┤ slack-tui ├──╴") + lipgloss.NewStyle().Foreground(p.Accent).Render("   v0.9.0")
	l2 := dim.Render("a keyboard-first chat client")
	return l1 + "\n" + l2
}

func bootLines() []tline {
	step := func(text, ok string) tline {
		cls := "accent"
		if ok == "done" || ok == "ok" || ok == "ready" {
			cls = "ok"
		}
		return tline{text: text, class: "fg", fill: true, ok: ok, okClass: cls}
	}
	return []tline{
		{text: `monospace-labs tty1 — slack-tui v0.9.0 "phosphor"`, class: "dim"},
		{text: "", class: "dim"},
		step("booting session", "done"),
		step("mounting message store", "done"),
		step("establishing sync gateway", "done"),
		step("loading keymap", "vim-compat"),
		step("resolving workspace @monospace-labs", "ok"),
		step("indexing 6 channels · 4 direct messages", "ready"),
		{text: "", class: "dim"},
		{text: "session ready. authenticate to continue.", class: "accent"},
	}
}

func oauthLines(provider string) []tline {
	if provider == "sso" {
		return []tline{
			{text: "[ sso ] single sign-on", class: "accent"},
			{text: "redirecting to your identity provider…", class: "dim"},
			{text: "  ↳ idp.monospace-labs.com/saml/login", class: "fill"},
			{text: "awaiting assertion … verified", class: "ok"},
			{text: "establishing session … ok", class: "ok"},
			{text: "", class: "dim"},
			{text: "authenticated ✓  ·  workspace @monospace-labs", class: "accent"},
		}
	}
	return []tline{
		{text: "[ oauth ] sign in with Slack", class: "accent"},
		{text: "opening authorization page in your browser…", class: "dim"},
		{text: "  ↳ slack.com/oauth/v2/authorize?client_id=•••&scope=client", class: "fill"},
		{text: "awaiting consent in browser … granted", class: "ok"},
		{text: "exchanging authorization code → access token … ok", class: "ok"},
		{text: "fetching identity & channels … ok", class: "ok"},
		{text: "", class: "dim"},
		{text: "authenticated ✓  ·  workspace @monospace-labs", class: "accent"},
	}
}
