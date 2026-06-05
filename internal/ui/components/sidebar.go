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
func SidebarBody(p theme.Palette, ws *data.Workspace, items []SideItem, activeID string, selIndex int, focused bool, innerW int) string {
	header := lipgloss.NewStyle().Foreground(p.Dim2).Background(p.Panel)
	var lines []string
	for i, it := range items {
		if it.Header {
			lines = append(lines, header.Render(it.Label))
			continue
		}
		lines = append(lines, sideRow(p, ws, it.Conv, i == selIndex && focused, it.Conv.ID == activeID, innerW))
	}
	return strings.Join(lines, "\n")
}

func sideRow(p theme.Palette, ws *data.Workspace, c data.Conversation, cursor, active bool, w int) string {
	unread := c.Unread > 0
	nameColor := p.Dim
	if unread || active || cursor {
		nameColor = p.Fg
	}

	var sigil string
	if c.Type == "channel" {
		sigil = lipgloss.NewStyle().Foreground(p.Dim2).Bold(true).Render("#")
	} else {
		sigil = PresenceDot(p, ws.Users[c.UserID].Status)
	}
	name := lipgloss.NewStyle().Foreground(nameColor).Bold(unread || active).Render(c.Name)

	var badge string
	switch {
	case c.Mention:
		badge = lipgloss.NewStyle().Foreground(p.Bg).Background(p.Orange).Render(fmt.Sprintf(" @%d ", c.Unread))
	case unread:
		badge = lipgloss.NewStyle().Foreground(p.Dim).Background(p.SelBg).Render(fmt.Sprintf(" %d ", c.Unread))
	}

	// 2-col left margin holds the accent cursor bar when this row is the cursor.
	margin := "  "
	if cursor {
		margin = lipgloss.NewStyle().Foreground(p.Accent).Render("▌") + " "
	}
	left := margin + sigil + " " + name
	gap := w - lipgloss.Width(left) - lipgloss.Width(badge)
	if gap < 0 {
		gap = 0
	}
	row := left + strings.Repeat(" ", gap) + badge

	if cursor || active {
		return padLine(row, w, p.SelBg)
	}
	return row
}
