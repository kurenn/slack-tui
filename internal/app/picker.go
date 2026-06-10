package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/source"
	"github.com/kurenn/slack-tui/internal/ui/components"
)

// The picker is a generic modal "input + list" overlay, reused by the reaction
// picker (`a`), the link picker (`o`) and workspace search (`s`). It renders
// with the same component as the command palette.

type pickerState struct {
	open   bool
	kind   string    // "react" | "link" | "search" | "join" — routes async fills
	items  []palItem // full set; filtered locally when filter is true
	index  int
	filter bool   // filter items against the query as the user types
	free   bool   // enter with no match picks the raw query (emoji names)
	lastQ  string // last submitted query (search mode)
	pick   func(m *Model, id string) tea.Cmd
	submit func(m *Model, query string) tea.Cmd // search mode: enter runs this first
}

func (m *Model) openPicker(placeholder string, items []palItem, st pickerState) tea.Cmd {
	st.open = true
	st.items = items
	m.picker = st
	m.pickerInput.Placeholder = placeholder
	m.pickerInput.SetValue("")
	return m.pickerInput.Focus()
}

func (m *Model) closePicker() {
	m.picker = pickerState{}
	m.pickerInput.Blur()
}

// pickerVisible is the list as filtered by the current query.
func (m Model) pickerVisible() []palItem {
	q := strings.ToLower(strings.TrimSpace(m.pickerInput.Value()))
	if !m.picker.filter || q == "" {
		return m.picker.items
	}
	var subs, fuzzy []palItem
	for _, it := range m.picker.items {
		hay := strings.ToLower(it.item.Label + " " + it.item.Hint)
		switch {
		case strings.Contains(hay, q):
			subs = append(subs, it)
		case fuzzyMatch(hay, q):
			fuzzy = append(fuzzy, it)
		}
	}
	return append(subs, fuzzy...)
}

func (m Model) pickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.closePicker()
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		m.picker.index = clamp(m.picker.index+1, 0, max(0, len(m.pickerVisible())-1))
		return m, nil
	case "up", "ctrl+p":
		m.picker.index = clamp(m.picker.index-1, 0, max(0, len(m.pickerVisible())-1))
		return m, nil
	case "enter":
		q := strings.TrimSpace(m.pickerInput.Value())
		if m.picker.submit != nil && q != "" && q != m.picker.lastQ {
			m.picker.lastQ = q // a fresh query: run the search, don't pick yet
			return m, m.picker.submit(&m, q)
		}
		if items := m.pickerVisible(); len(items) > 0 {
			id := items[clamp(m.picker.index, 0, len(items)-1)].id
			pick := m.picker.pick
			m.closePicker()
			return m, pick(&m, id)
		}
		if m.picker.free && q != "" {
			pick := m.picker.pick
			m.closePicker()
			return m, pick(&m, q)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.pickerInput, cmd = m.pickerInput.Update(msg)
	m.picker.index = clamp(m.picker.index, 0, max(0, len(m.pickerVisible())-1))
	return m, cmd
}

// overlayPicker composites the picker box over the frame (palette position).
func (m Model) overlayPicker(frame string) string {
	boxW := 58
	if boxW > m.width-8 {
		boxW = m.width - 8
	}
	items := m.pickerVisible()
	pitems := make([]components.PaletteItem, len(items))
	for i, it := range items {
		pitems[i] = it.item
	}
	m.pickerInput.Width = max(4, boxW-10)
	box := components.Palette(m.pal, m.pickerInput.View(), pitems, m.picker.index, boxW, 10)

	x := (m.width - boxW) / 2
	y := m.height * 12 / 100
	if y < 1 {
		y = 1
	}
	return overlay(frame, box, x, y)
}

// ── concrete pickers ────────────────────────────────────────────────────────

// openReactionPicker reacts to the given message: pick a curated emoji or type
// any Slack emoji name (custom ones included).
func (m *Model) openReactionPicker(msgID string) tea.Cmd {
	conv := m.activeID
	items := make([]palItem, 0, len(source.ReactionChoices))
	for _, name := range source.ReactionChoices {
		items = append(items, palItem{id: name, item: components.PaletteItem{
			Icon: source.EmojiGlyph(name), Label: name, Hint: "react", Kind: "cmd",
		}})
	}
	return m.openPicker("react with… (any emoji name works)", items, pickerState{
		kind: "react", filter: true, free: true,
		pick: func(mm *Model, name string) tea.Cmd {
			return mm.reactCmd(conv, msgID, strings.Trim(name, ":"))
		},
	})
}

// openLinkPicker picks one of several links found in a message.
func (m *Model) openLinkPicker(links []string) tea.Cmd {
	items := make([]palItem, 0, len(links))
	for _, l := range links {
		items = append(items, palItem{id: l, item: components.PaletteItem{
			Icon: "↗", Label: l, Hint: "open", Kind: "cmd",
		}})
	}
	return m.openPicker("open link…", items, pickerState{
		kind: "link", filter: true,
		pick: func(mm *Model, url string) tea.Cmd { return openURLCmd(url) },
	})
}

// openSearchPicker runs workspace search: type a query, enter searches, enter
// on a result jumps to it.
func (m *Model) openSearchPicker() tea.Cmd {
	return m.openPicker("search workspace… (enter to search)", nil, pickerState{
		kind:   "search",
		submit: func(mm *Model, q string) tea.Cmd { return mm.searchCmd(q) },
		pick: func(mm *Model, id string) tea.Cmd { // id = "convID|msgID"
			conv, msgID, ok := strings.Cut(id, "|")
			if !ok {
				return nil
			}
			mm.pendingSelect[conv] = msgID
			return mm.openChannel(conv)
		},
	})
}

// openJoinPicker browses public channels you're not in; enter joins one.
func (m *Model) openJoinPicker() tea.Cmd {
	focus := m.openPicker("join a channel… (loading)", nil, pickerState{
		kind: "join", filter: true,
		pick: func(mm *Model, id string) tea.Cmd { return mm.joinCmd(id) },
	})
	return tea.Batch(focus, m.joinableCmd())
}
