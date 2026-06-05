package onboarding

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/abrahamkuri/slack-tui/internal/theme"
)

// tline is one typewriter line with a color class.
type tline struct {
	text  string
	class string // accent | dim | ok | fill | fg
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
		total += len([]rune(l.text)) + 1 // +1 for the line break
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

func (t typewriter) render(p theme.Palette) string {
	color := func(class string) lipgloss.Style {
		s := lipgloss.NewStyle()
		switch class {
		case "accent":
			return s.Foreground(p.Accent).Bold(true)
		case "ok":
			return s.Foreground(p.Green)
		case "dim":
			return s.Foreground(p.Dim)
		case "fill":
			return s.Foreground(p.Dim2)
		default:
			return s.Foreground(p.Fg)
		}
	}
	cursor := lipgloss.NewStyle().Foreground(p.Accent).Render("▋")

	var out []string
	remaining := t.pos
	for _, l := range t.lines {
		runes := []rune(l.text)
		if remaining <= 0 && !t.done {
			break
		}
		n := len(runes)
		atCursor := false
		if remaining < n {
			n = remaining
			atCursor = true
		}
		line := color(l.class).Render(string(runes[:max0(n)]))
		if atCursor && !t.done {
			line += cursor
		}
		out = append(out, line)
		remaining -= len(runes) + 1
		if atCursor && !t.done {
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

func bootLines() []tline {
	return []tline{
		{"   ┌────────────────────────────────┐", "fill"},
		{"   │  s l a c k - t u i             │", "accent"},
		{"   │  terminal workspace client     │", "dim"},
		{"   └────────────────────────────────┘", "fill"},
		{"", "dim"},
		{"initializing terminal core ......... ok", "ok"},
		{"loading theme engine ............... ok", "ok"},
		{"probing truecolor support .......... yes", "ok"},
		{"mounting keymap (vim · modal) ...... ok", "ok"},
		{"establishing secure channel ........ ok", "ok"},
		{"", "dim"},
		{"slack-tui ready.", "accent"},
	}
}

func oauthLines(provider string) []tline {
	if provider == "sso" {
		return []tline{
			{"[ sso ] single sign-on", "accent"},
			{"redirecting to your identity provider…", "dim"},
			{"  ↳ idp.monospace-labs.com/saml/login", "fill"},
			{"awaiting assertion … verified", "ok"},
			{"establishing session … ok", "ok"},
			{"", "dim"},
			{"authenticated ✓  ·  workspace @monospace-labs", "accent"},
		}
	}
	return []tline{
		{"[ oauth ] sign in with Slack", "accent"},
		{"opening authorization page in your browser…", "dim"},
		{"  ↳ slack.com/oauth/v2/authorize?client_id=•••&scope=client", "fill"},
		{"awaiting consent in browser … granted", "ok"},
		{"exchanging authorization code → access token … ok", "ok"},
		{"fetching identity & channels … ok", "ok"},
		{"", "dim"},
		{"authenticated ✓  ·  workspace @monospace-labs", "accent"},
	}
}
