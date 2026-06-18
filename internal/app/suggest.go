package app

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kurenn/slack-tui/internal/source"
	"github.com/kurenn/slack-tui/internal/theme"
)

// Composer autocomplete: typing "@…" suggests workspace handles (so outgoing
// mentions resolve and actually ping), ":…" suggests emoji. Tab/enter accepts,
// ctrl+n/p (or ↓/↑) navigates, esc dismisses without leaving INSERT.

type suggestItem struct {
	insert string // text that replaces the token
	icon   string
	label  string
	hint   string
}

type suggestState struct {
	items      []suggestItem
	index      int
	row        int // logical line of the token
	start, end int // token rune-span within that line
}

const maxSuggestions = 6

func (m Model) suggestActive() bool { return m.insert && len(m.suggest.items) > 0 }

func (m *Model) clearSuggest() { m.suggest = suggestState{} }

// activeArea is the composer the cursor is in.
func (m *Model) activeArea() *textarea.Model {
	if m.focus == focusThread {
		return &m.threadDraft
	}
	return &m.draft
}

// suggestTokRe finds a trigger token (@handle or :emoji) ending at the cursor.
var suggestTokRe = regexp.MustCompile(`(^|\s)([@:][A-Za-z0-9._+-]*)$`)

// recomputeSuggest re-derives the suggestion popup from the text before the
// cursor. Called after every keystroke the composer consumed.
func (m *Model) recomputeSuggest() {
	m.suggest = suggestState{}
	if !m.insert {
		return
	}
	ta := m.activeArea()
	lines := strings.Split(ta.Value(), "\n")
	row := ta.Line()
	if row < 0 || row >= len(lines) {
		return
	}
	li := ta.LineInfo()
	runes := []rune(lines[row])
	col := li.StartColumn + li.CharOffset
	if col > len(runes) {
		col = len(runes)
	}
	before := string(runes[:col])
	loc := suggestTokRe.FindStringSubmatchIndex(before)
	if loc == nil {
		return
	}
	tok := before[loc[4]:loc[5]]
	var items []suggestItem
	switch {
	case tok[0] == '@':
		items = m.mentionSuggestions(strings.ToLower(tok[1:]))
	case len(tok) > 1: // ':' needs at least one character (skip "1:2", ":)")
		items = emojiSuggestions(strings.ToLower(tok[1:]))
	}
	if len(items) == 0 {
		return
	}
	m.suggest = suggestState{
		items: items,
		row:   row,
		start: utf8.RuneCountInString(before[:loc[4]]),
		end:   col,
	}
}

// mentionSuggestions ranks handles: prefix matches first, then substring on
// handle or name; the broadcast forms ride along when they match.
func (m Model) mentionSuggestions(prefix string) []suggestItem {
	type cand struct {
		rank int
		it   suggestItem
	}
	var cands []cand
	for _, u := range m.ws.Users {
		if u.Handle == "" || u.ID == m.ws.MeID {
			continue
		}
		h := strings.ToLower(u.Handle)
		rank := -1
		switch {
		case strings.HasPrefix(h, prefix):
			rank = 0
		case strings.Contains(h, prefix) || strings.Contains(strings.ToLower(u.Name), prefix):
			rank = 1
		}
		if rank < 0 {
			continue
		}
		cands = append(cands, cand{rank, suggestItem{
			insert: "@" + u.Handle + " ", icon: "@", label: u.Handle, hint: u.Name,
		}})
	}
	for _, b := range []string{"here", "channel", "everyone"} {
		if strings.HasPrefix(b, prefix) {
			cands = append(cands, cand{2, suggestItem{
				insert: "@" + b + " ", icon: "!", label: b, hint: "notify " + b,
			}})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].rank != cands[j].rank {
			return cands[i].rank < cands[j].rank
		}
		return cands[i].it.label < cands[j].it.label
	})
	items := make([]suggestItem, 0, maxSuggestions)
	for _, c := range cands {
		items = append(items, c.it)
		if len(items) == maxSuggestions {
			break
		}
	}
	return items
}

// emojiSuggestions matches known emoji names; accepting inserts the glyph.
func emojiSuggestions(prefix string) []suggestItem {
	var pre, sub []suggestItem
	for _, name := range source.EmojiNames() {
		it := suggestItem{
			insert: source.EmojiGlyph(name) + " ", icon: source.EmojiGlyph(name),
			label: ":" + name + ":", hint: "emoji",
		}
		switch {
		case strings.HasPrefix(name, prefix):
			pre = append(pre, it)
		case strings.Contains(name, prefix):
			sub = append(sub, it)
		}
	}
	items := append(pre, sub...)
	if len(items) > maxSuggestions {
		items = items[:maxSuggestions]
	}
	return items
}

// acceptSuggest replaces the trigger token with the selected completion and
// puts the cursor right after it.
func (m *Model) acceptSuggest() {
	if len(m.suggest.items) == 0 {
		return
	}
	it := m.suggest.items[clamp(m.suggest.index, 0, len(m.suggest.items)-1)]
	ta := m.activeArea()
	lines := strings.Split(ta.Value(), "\n")
	if m.suggest.row >= len(lines) {
		m.clearSuggest()
		return
	}
	runes := []rune(lines[m.suggest.row])
	start, end := clamp(m.suggest.start, 0, len(runes)), clamp(m.suggest.end, 0, len(runes))
	lines[m.suggest.row] = string(runes[:start]) + it.insert + string(runes[end:])
	ta.SetValue(strings.Join(lines, "\n")) // cursor lands at the very end…
	for ta.Line() > m.suggest.row {        // …walk it back to just after the insertion
		ta.CursorUp()
	}
	ta.SetCursor(start + utf8.RuneCountInString(it.insert))
	m.clearSuggest()
}

// suggestKey handles keys while the popup is visible. Returns handled=false
// for keys the composer should process normally.
func (m *Model) suggestKey(k string) (handled bool) {
	switch k {
	case "tab", "enter":
		m.acceptSuggest()
	case "ctrl+n", "down":
		m.suggest.index = clamp(m.suggest.index+1, 0, len(m.suggest.items)-1)
	case "ctrl+p", "up":
		m.suggest.index = clamp(m.suggest.index-1, 0, len(m.suggest.items)-1)
	case "esc":
		m.clearSuggest()
	default:
		return false
	}
	return true
}

// overlaySuggest composites the popup just above the active composer.
func (m Model) overlaySuggest(frame string) string {
	p := m.pal
	w := 44
	if w > m.width-4 {
		w = m.width - 4
	}
	inner := w - 2

	sel := m.suggest.index
	var rows []string
	for i, it := range m.suggest.items {
		bg := p.Panel
		labelColor := p.Dim
		if i == sel {
			bg = p.SelBg
			labelColor = p.Fg
		}
		icon := lipgloss.NewStyle().Foreground(p.Accent).Background(bg).Render(it.icon)
		label := lipgloss.NewStyle().Foreground(labelColor).Background(bg).Bold(i == sel).Render(it.label)
		hint := lipgloss.NewStyle().Foreground(p.Dim2).Background(bg).Render(it.hint)
		line := " " + icon + " " + label + "  " + hint
		rows = append(rows, theme.FillBg(line, inner, bg))
	}

	bs := lipgloss.NewStyle().Foreground(p.Border)
	box := []string{bs.Render("┌" + strings.Repeat("─", inner) + "┐")}
	for _, r := range rows {
		box = append(box, bs.Render("│")+r+bs.Render("│"))
	}
	box = append(box, bs.Render("└"+strings.Repeat("─", inner)+"┘"))

	x := sidebarWidth + 2 // above the center composer
	composerH := m.composerHeight()
	if m.focus == focusThread {
		x = m.width - m.threadWidth + 1
		composerH = m.threadComposerHeight() + 1
	}
	if x+w > m.width {
		x = max(0, m.width-w-1)
	}
	y := m.height - 1 - composerH - len(box)
	if y < 1 {
		y = 1
	}
	return overlay(frame, strings.Join(box, "\n"), x, y)
}
