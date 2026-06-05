package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/abrahamkuri/slack-tui/internal/ui/components"
)

const paletteMaxRows = 8

// palItem is a palette entry plus the action id used to run it.
type palItem struct {
	id   string
	item components.PaletteItem
}

// paletteItems builds the full (unfiltered) command list: jump to any
// channel/DM, then the commands.
func (m Model) paletteItems() []palItem {
	var items []palItem
	for _, c := range m.ws.Channels {
		items = append(items, palItem{"ch:" + c.ID, components.PaletteItem{Icon: "#", Label: c.Name, Hint: "channel", Kind: "channel"}})
	}
	for _, c := range m.ws.DMs {
		items = append(items, palItem{"dm:" + c.ID, components.PaletteItem{Icon: "•", Label: c.Name, Hint: "direct message", Kind: "dm"}})
	}
	hintsLabel := "Show key hints"
	if m.showHints {
		hintsLabel = "Hide key hints"
	}
	items = append(items,
		palItem{"cmd:settings", components.PaletteItem{Icon: "⚙", Label: "Settings", Hint: "appearance · presence", Kind: "cmd"}},
		palItem{"cmd:theme", components.PaletteItem{Icon: "▣", Label: "Cycle theme", Hint: "appearance", Kind: "cmd"}},
		palItem{"cmd:hints", components.PaletteItem{Icon: "⌨", Label: hintsLabel, Hint: "status bar", Kind: "cmd"}},
		palItem{"cmd:active", components.PaletteItem{Icon: "●", Label: "Set status: active", Hint: "presence", Kind: "cmd"}},
		palItem{"cmd:away", components.PaletteItem{Icon: "○", Label: "Set status: away", Hint: "presence", Kind: "cmd"}},
		palItem{"cmd:dnd", components.PaletteItem{Icon: "◓", Label: "Set status: do not disturb", Hint: "presence", Kind: "cmd"}},
		palItem{"cmd:read", components.PaletteItem{Icon: "✓", Label: "Mark all as read", Hint: "inbox", Kind: "cmd"}},
	)
	return items
}

// filteredPalette applies the query (leading #@:/ stripped, substring match on
// label+hint).
func (m Model) filteredPalette() []palItem {
	q := strings.ToLower(strings.TrimSpace(m.paletteQuery.Value()))
	q = strings.TrimLeft(q, "#@:/")
	all := m.paletteItems()
	if q == "" {
		return all
	}
	var out []palItem
	for _, it := range all {
		hay := strings.ToLower(it.item.Label + " " + it.item.Hint)
		if strings.Contains(hay, q) {
			out = append(out, it)
		}
	}
	return out
}

func (m *Model) openPalette() tea.Cmd {
	m.paletteOpen = true
	m.paletteQuery.SetValue("")
	m.paletteIndex = 0
	return m.paletteQuery.Focus()
}

func (m *Model) closePalette() {
	m.paletteOpen = false
	m.paletteQuery.Blur()
}

// paletteKey handles input while the palette is open.
func (m Model) paletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch {
	case k == "esc":
		m.closePalette()
		return m, nil
	case k == "down", k == "ctrl+n", k == "ctrl+j":
		m.paletteIndex = clamp(m.paletteIndex+1, 0, len(m.filteredPalette())-1)
		return m, nil
	case k == "up", k == "ctrl+p":
		m.paletteIndex = clamp(m.paletteIndex-1, 0, len(m.filteredPalette())-1)
		return m, nil
	case k == "enter":
		items := m.filteredPalette()
		var cmd tea.Cmd
		if m.paletteIndex < len(items) {
			cmd = m.runPalette(items[m.paletteIndex].id)
		}
		m.closePalette()
		return m, cmd
	}
	var cmd tea.Cmd
	m.paletteQuery, cmd = m.paletteQuery.Update(msg)
	m.paletteIndex = clamp(m.paletteIndex, 0, max(0, len(m.filteredPalette())-1))
	return m, cmd
}

// runPalette executes a palette action by id, returning any follow-up command.
func (m *Model) runPalette(id string) tea.Cmd {
	switch {
	case strings.HasPrefix(id, "ch:"):
		return m.openChannel(strings.TrimPrefix(id, "ch:"))
	case strings.HasPrefix(id, "dm:"):
		return m.openChannel(strings.TrimPrefix(id, "dm:"))
	case id == "cmd:settings":
		m.openSettings()
	case id == "cmd:theme":
		i := indexOf(theme.Cycle, m.prefs.Theme)
		m.prefs.Theme = theme.Cycle[(i+1)%len(theme.Cycle)]
		m.pal = theme.Resolve(m.prefs.Theme, m.prefs.Accent)
	case id == "cmd:hints":
		m.showHints = !m.showHints
	case id == "cmd:active":
		return m.setStatus("online")
	case id == "cmd:away":
		return m.setStatus("away")
	case id == "cmd:dnd":
		return m.setStatus("dnd")
	case id == "cmd:read":
		for k := range m.meta {
			m.meta[k] = components.Meta{Unread: 0, Mention: false}
		}
	}
	return nil
}
