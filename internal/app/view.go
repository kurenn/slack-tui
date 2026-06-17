package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/ui/components"
	"github.com/kurenn/slack-tui/internal/ui/pane"
)

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	bodyH := m.height - 2
	threadOpen := m.threadOpen()

	centerW := m.width - sidebarWidth - 1
	if threadOpen {
		centerW = m.width - sidebarWidth - threadWidth - 2
	}
	if m.width < 50 || bodyH < 8 || centerW < 24 {
		return lipgloss.NewStyle().Foreground(m.pal.Dim).
			Render("slack-tui needs a larger terminal — widen the window")
	}

	conv, _ := m.ws.Conversation(m.activeID)
	panes := 2
	if threadOpen {
		panes = 3
	}

	// ── sidebar (scrolls to keep the cursor visible) ──
	items := m.sideItems()
	sideLines, sideCursor := components.SidebarBody(m.pal, m.ws, items, m.activeID, m.sideSel, m.focus == focusSidebar, sidebarWidth-2)
	sideBody := strings.Join(windowLines(sideLines, sideCursor, sideCursor, bodyH-2), "\n")
	sidebar := pane.Render(m.pal, pane.Options{
		Title: "workspace", Right: "@" + m.ws.Handle, Focused: m.focus == focusSidebar,
		Width: sidebarWidth, Height: bodyH, Body: sideBody,
	})

	// ── center: messages pane + composer ──
	center := m.renderCenter(conv, centerW, bodyH)

	// ── assemble workspace row ──
	cols := []string{sidebar, m.gap(bodyH), center}
	if threadOpen {
		cols = append(cols, m.gap(bodyH), m.renderThread(conv, bodyH))
	}
	workspace := lipgloss.JoinHorizontal(lipgloss.Top, cols...)

	title := components.TitleBar(m.pal, m.ws.Name, m.ws.Me().Handle, m.myStatus, panes, m.width)
	status := components.StatusBar(m.pal, m.insert, m.locName(conv), m.focus, m.hints(), m.showHints, m.width)
	frame := strings.Join([]string{title, workspace, status}, "\n")

	if m.loadErr != nil {
		msg := " ⚠ slack: " + m.loadErr.Error() + " · esc dismiss"
		if r := []rune(msg); len(r) > m.width {
			msg = string(r[:m.width-1]) + "…"
		}
		banner := lipgloss.NewStyle().Foreground(m.pal.Bg).Background(m.pal.Red).Bold(true).
			Width(m.width).Render(msg)
		frame = overlay(frame, banner, 0, 1)
	}
	if m.copyToast != "" {
		label := lipgloss.NewStyle().Foreground(m.pal.Bg).Background(m.pal.Accent).Bold(true).Render(" ✓ " + m.copyToast + " ")
		tw := lipgloss.Width(label)
		tx := clamp(m.copyToastX, 0, max(0, m.width-tw))
		frame = overlay(frame, label, tx, m.copyToastY)
	}
	if m.paletteOpen {
		frame = m.overlayPalette(frame)
	}
	if m.settingsOpen {
		frame = m.overlaySettings(frame)
	}
	if m.statusTextOpen {
		frame = m.overlayStatusText(frame)
	}
	if m.suggestActive() {
		frame = m.overlaySuggest(frame)
	}
	if m.findOpen {
		frame = m.overlayFind(frame)
	}
	if m.picker.open {
		frame = m.overlayPicker(frame)
	}
	if m.confirm.open {
		frame = m.overlayConfirm(frame)
	}
	if m.helpOpen {
		frame = m.overlayHelp(frame)
	}
	return frame
}

// overlayPalette composites the command-palette box over the frame, centered
// horizontally and ~12% from the top.
func (m Model) overlayPalette(frame string) string {
	boxW := 58
	if boxW > m.width-8 {
		boxW = m.width - 8
	}
	items := m.filteredPalette()
	pitems := make([]components.PaletteItem, len(items))
	for i, it := range items {
		pitems[i] = it.item
	}
	m.paletteQuery.Width = max(4, boxW-10)
	box := components.Palette(m.pal, m.paletteQuery.View(), pitems, m.paletteIndex, boxW, paletteMaxRows)

	x := (m.width - boxW) / 2
	y := m.height * 12 / 100
	if y < 1 {
		y = 1
	}
	return overlay(frame, box, x, y)
}

func (m Model) renderCenter(conv data.Conversation, w, bodyH int) string {
	paneH := bodyH - m.composerHeight()
	innerH := paneH - 2

	var body string
	msgs := m.curMsgs()
	if m.loading[m.activeID] && len(msgs) == 0 {
		body = lipgloss.NewStyle().Foreground(m.pal.Dim).Render("  loading…")
	} else {
		lines, top, _, _ := m.msgViewport()
		// Work on a copy so we don't mutate the slice returned by msgViewport.
		var visible []string
		if len(lines) > innerH {
			visible = append([]string(nil), lines[top:top+innerH]...)
		} else {
			visible = append([]string(nil), lines...)
		}
		// Paint the mouse-drag selection highlight.
		if m.selActive {
			a, b := orderCells(m.selAnchor, m.selHead)
			for j := range visible {
				L := top + j
				if L < a.line || L > b.line {
					continue
				}
				from, to := 0, lipgloss.Width(visible[j])
				if L == a.line {
					from = a.col
				}
				if L == b.line {
					to = b.col
				}
				if to <= from {
					continue
				}
				seg := ansi.Strip(ansi.TruncateLeft(ansi.Truncate(visible[j], to, ""), from, ""))
				hl := lipgloss.NewStyle().Foreground(m.pal.Fg).Background(m.pal.SelBg).Render(seg)
				visible[j] = spliceLine(visible[j], hl, from)
			}
		}
		body = strings.Join(visible, "\n")
	}

	title := "#" + conv.Name
	right := conv.Topic
	if conv.Type == "dm" {
		title = conv.Name
		if right == "" {
			right = "direct message"
		}
	}
	msgsPane := pane.Render(m.pal, pane.Options{
		Title: title, Right: right, Focused: m.focus == focusMessages,
		Width: w, Height: paneH, Body: body,
	})

	insertHere := m.insert && m.focus != focusThread
	prompt := "·"
	if insertHere {
		prompt = "❯"
	}
	m.draft.Placeholder = "message " + m.locName(conv)
	if m.editingID != "" {
		prompt = "✎"
		m.draft.Placeholder = "edit message"
	}
	// width/height live on the persistent model (syncComposerSizes) — sizing a
	// render copy would leave the real textarea repositioning at a stale size.
	composer := components.Composer(m.pal, prompt, m.draft.View(), insertHere, w)

	return lipgloss.JoinVertical(lipgloss.Left, msgsPane, composer)
}

func (m Model) renderThread(conv data.Conversation, bodyH int) string {
	innerW := threadWidth - 2
	innerH := bodyH - 2
	scrollH := innerH - m.threadComposerHeight()

	root, ok := m.threadRoot()
	if !ok {
		return pane.Render(m.pal, pane.Options{Title: "thread", Width: threadWidth, Height: bodyH})
	}

	lines, starts := components.ThreadScroll(m.pal, m.ws, root, m.threadSel, m.focus == focusThread, innerW)
	ss, se := 0, 0
	if r := len(root.Replies); r > 0 {
		ti := clamp(m.threadSel, 0, r-1)
		ss, se = starts[ti], starts[ti+1]-1
	}
	windowed := windowLines(lines, ss, se, scrollH)
	for len(windowed) < scrollH {
		windowed = append(windowed, "")
	}

	insertHere := m.insert && m.focus == focusThread
	prompt := "·"
	if insertHere {
		prompt = "❯"
	}
	m.threadDraft.Placeholder = "reply in thread"
	composer := components.Composer(m.pal, prompt, m.threadDraft.View(), insertHere, innerW)

	body := strings.Join(windowed, "\n") + "\n" + composer
	return pane.Render(m.pal, pane.Options{
		Title: "thread", Right: "#" + conv.Name, Focused: m.focus == focusThread,
		Width: threadWidth, Height: bodyH, Body: body,
	})
}

func (m Model) locName(conv data.Conversation) string {
	if conv.Type == "dm" {
		return "@" + conv.Name
	}
	return "#" + conv.Name
}

func (m Model) hints() []components.Hint {
	switch {
	case m.insert:
		return []components.Hint{components.H("↵", "send"), components.H("esc", "normal"), components.H("⌃k", "palette")}
	case m.focus == focusSidebar:
		return []components.Hint{components.H("j/k", "move"), components.H("↵", "open"), components.H("l", "messages"), components.H("⌃k", "palette")}
	case m.focus == focusThread:
		return []components.Hint{components.H("j/k", "replies"), components.H("i", "reply"), components.H("esc", "close"), components.H("h", "back")}
	default:
		return []components.Hint{components.H("j/k", "messages"), components.H("t", "thread"), components.H("i", "write"), components.H("h/l", "panes"), components.H("⌃k", "palette"), components.H("?", "help")}
	}
}
