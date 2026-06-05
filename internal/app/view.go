package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/abrahamkuri/slack-tui/internal/data"
	"github.com/abrahamkuri/slack-tui/internal/ui/components"
	"github.com/abrahamkuri/slack-tui/internal/ui/pane"
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
			Render("slack-tui needs a larger terminal — widen the window" +
				strings.Repeat(" ", 0))
	}

	conv, _ := m.ws.Conversation(m.activeID)
	panes := 2
	if threadOpen {
		panes = 3
	}

	// ── sidebar ──
	items := m.sideItems()
	sidebar := pane.Render(m.pal, pane.Options{
		Title: "workspace", Right: "@" + m.ws.Handle, Focused: m.focus == focusSidebar,
		Width: sidebarWidth, Height: bodyH,
		Body: components.SidebarBody(m.pal, m.ws, items, m.activeID, m.sideSel, m.focus == focusSidebar, sidebarWidth-2),
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
	return strings.Join([]string{title, workspace, status}, "\n")
}

func (m Model) renderCenter(conv data.Conversation, w, bodyH int) string {
	innerW := w - 2
	paneH := bodyH - composerH
	innerH := paneH - 2

	msgs := m.curMsgs()
	lines, starts := components.MessagesBody(m.pal, m.ws, msgs, m.msgSel, m.focus == focusMessages, m.density, innerW)
	ss, se := 0, 0
	if n := len(msgs); n > 0 {
		mi := clamp(m.msgSel, 0, n-1)
		ss, se = starts[mi], starts[mi+1]-1
	}
	body := strings.Join(windowLines(lines, ss, se, innerH), "\n")

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
	m.draft.Width = max(4, innerW-18)
	composer := components.Composer(m.pal, prompt, m.draft.View(), insertHere, w)

	return lipgloss.JoinVertical(lipgloss.Left, msgsPane, composer)
}

func (m Model) renderThread(conv data.Conversation, bodyH int) string {
	innerW := threadWidth - 2
	innerH := bodyH - 2
	scrollH := innerH - composerH

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
	m.threadDraft.Width = max(4, innerW-18)
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
		return []components.Hint{components.H("j/k", "messages"), components.H("t", "thread"), components.H("i", "write"), components.H("h/l", "panes"), components.H("⌃k", "palette")}
	}
}
