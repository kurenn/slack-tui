package components

import (
	"fmt"
	"strings"

	"github.com/abrahamkuri/slack-tui/internal/data"
	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

// SideItem is one flat sidebar entry: a section header or a conversation row.
// The flat list (headers interleaved with rows) matches the prototype so the
// keyboard cursor index lines up with what's drawn.
type SideItem struct {
	Header bool
	Label  string
	Conv   data.Conversation
}

// BuildSideItems flattens channels and DMs into the rendered list, applying live
// unread/mention meta. meta maps conversation id → {unread, mention}.
func BuildSideItems(ws *data.Workspace, meta map[string]Meta) []SideItem {
	items := []SideItem{{Header: true, Label: "── channels ──"}}
	for _, c := range ws.Channels {
		items = append(items, SideItem{Conv: applyMeta(c, meta)})
	}
	items = append(items, SideItem{Header: true, Label: "── direct messages ──"})
	for _, c := range ws.DMs {
		items = append(items, SideItem{Conv: applyMeta(c, meta)})
	}
	return items
}

// Meta is the per-conversation unread/mention state the app mutates.
type Meta struct {
	Unread  int
	Mention bool
}

func applyMeta(c data.Conversation, meta map[string]Meta) data.Conversation {
	if m, ok := meta[c.ID]; ok {
		c.Unread = m.Unread
		c.Mention = m.Mention
	}
	return c
}

// SelectableIndexes returns the flat-list indexes that are rows (not headers).
func SelectableIndexes(items []SideItem) []int {
	var idx []int
	for i, it := range items {
		if !it.Header {
			idx = append(idx, i)
		}
	}
	return idx
}

// SidebarBody renders the sidebar list to a body string. selIndex is a flat-list
// index; the cursor highlight only shows when focused.
// SidebarBody renders the sidebar to lines plus the line index of the cursor row
// (so the caller can scroll it into view).
func SidebarBody(p theme.Palette, ws *data.Workspace, items []SideItem, activeID string, selIndex int, focused bool, innerW int) (lines []string, cursorLine int) {
	header := lipgloss.NewStyle().Foreground(p.Dim2)
	lines = append(lines, "") // top breathing room
	for i, it := range items {
		if it.Header {
			lines = append(lines, " "+header.Render(it.Label))
			continue
		}
		if i == selIndex {
			cursorLine = len(lines)
		}
		lines = append(lines, sideRow(p, ws, it.Conv, i == selIndex && focused, it.Conv.ID == activeID, innerW))
	}
	return lines, cursorLine
}

// sideRow renders one conversation row. The active/cursor highlight is inset one
// column from each pane edge (a contained block, not a full-bleed band); the
// cursor adds an accent bar in the left margin.
func sideRow(p theme.Palette, ws *data.Workspace, c data.Conversation, cursor, active bool, w int) string {
	unread := c.Unread > 0
	nameColor := p.Dim
	if unread || active || cursor {
		nameColor = p.Fg
	}
	sigilColor := p.Dim2
	if unread || active {
		sigilColor = p.Dim
	}
	var sigil string
	if c.Type == "channel" {
		sigil = lipgloss.NewStyle().Foreground(sigilColor).Bold(true).Render("#")
	} else {
		sigil = PresenceDot(p, ws.Users[c.UserID].Status)
	}
	name := lipgloss.NewStyle().Foreground(nameColor).Bold(unread || active).Render(c.Name)

	var badge string
	switch {
	case c.Mention:
		badge = lipgloss.NewStyle().Foreground(p.Bg).Background(p.Orange).Render(fmt.Sprintf(" @%d ", c.Unread))
	case unread:
		badge = lipgloss.NewStyle().Foreground(p.Fg).Background(p.BadgeBg).Render(fmt.Sprintf(" %d ", c.Unread))
	}

	region := w - 2 // 1-col margin on each side
	lead := " " + sigil + " " + name
	gap := region - lipgloss.Width(lead) - lipgloss.Width(badge) - 1
	if gap < 1 {
		gap = 1
	}
	content := lead + strings.Repeat(" ", gap) + badge + " "

	// Left margin marks state: cursor bar, then an unread dot (orange for a
	// mention, accent otherwise).
	leftMargin, rightMargin := " ", " "
	switch {
	case cursor:
		leftMargin = lipgloss.NewStyle().Foreground(p.Accent).Render("▌")
	case c.Mention:
		leftMargin = lipgloss.NewStyle().Foreground(p.Orange).Render("•")
	case unread:
		leftMargin = lipgloss.NewStyle().Foreground(p.Accent).Render("•")
	}
	if active || cursor {
		content = lipgloss.NewStyle().Width(region).Background(p.SelBg).Render(content)
	}
	return leftMargin + content + rightMargin
}
