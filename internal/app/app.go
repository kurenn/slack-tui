// Package app is the root Bubble Tea model: navigation state, the modal
// NORMAL/INSERT keyboard engine (ported from tui-app.jsx's keydown router), and
// the View that composes the panes. Data still comes from the mock workspace.
package app

import (
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/abrahamkuri/slack-tui/internal/config"
	"github.com/abrahamkuri/slack-tui/internal/data"
	"github.com/abrahamkuri/slack-tui/internal/source"
	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/abrahamkuri/slack-tui/internal/ui/components"
)

const (
	sidebarWidth = 30
	threadWidth  = 44
	composerH    = 3

	focusSidebar  = "sidebar"
	focusMessages = "messages"
	focusThread   = "thread"
)

// Model is the application state.
type Model struct {
	src       source.Source
	ws        *data.Workspace
	prefs     config.Prefs
	pal       theme.Palette
	density   theme.Density
	showHints bool
	loadErr   error

	messages map[string][]data.Message
	meta     map[string]components.Meta
	myStatus string

	activeID     string
	focus        string
	insert       bool
	sideSel      int // flat index into sidebar items
	msgSel       int
	threadRootID string
	threadSel    int

	draft       textinput.Model
	threadDraft textinput.Model

	paletteOpen  bool
	paletteQuery textinput.Model
	paletteIndex int

	width, height int
	gPending      time.Time
}

// New builds the initial model from saved (or default) prefs. The data source is
// real Slack when SLACK_USER_TOKEN is set, otherwise the local mock. The active
// channel's history is loaded synchronously; other channels load on open.
func New() Model {
	prefs, _ := config.Load()

	var src source.Source
	if tok := os.Getenv("SLACK_USER_TOKEN"); tok != "" {
		src = source.NewSlack(tok)
	} else {
		src = source.NewMock()
	}
	ws, err := src.Load()
	var loadErr error
	if err != nil { // fall back to the mock so the app still runs; surface the error
		loadErr = err
		src = source.NewMock()
		ws, _ = src.Load()
	}

	// Real-time: start Socket Mode if the app + bot tokens are present.
	if sl, ok := src.(*source.Slack); ok {
		if app, bot := os.Getenv("SLACK_APP_TOKEN"), os.Getenv("SLACK_BOT_TOKEN"); app != "" && bot != "" {
			sl.StartSocket(app, bot)
		}
	}

	// Onboarding hand-off: adopt the chosen handle as the current user's identity.
	if prefs.Handle != "" {
		me := ws.Users[ws.MeID]
		me.Name, me.Handle = prefs.Handle, prefs.Handle
		ws.Users[ws.MeID] = me
	}

	meta := map[string]components.Meta{}
	for _, c := range append(append([]data.Conversation{}, ws.Channels...), ws.DMs...) {
		meta[c.ID] = components.Meta{Unread: c.Unread, Mention: c.Mention}
	}

	mkInput := func() textinput.Model {
		ti := textinput.New()
		ti.Prompt = ""
		return ti
	}

	activeID := "engineering"
	if _, ok := ws.Conversation(activeID); !ok { // real Slack: pick the first channel/DM
		switch {
		case len(ws.Channels) > 0:
			activeID = ws.Channels[0].ID
		case len(ws.DMs) > 0:
			activeID = ws.DMs[0].ID
		}
	}

	m := Model{
		src:          src,
		ws:           ws,
		prefs:        prefs,
		pal:          theme.Resolve(prefs.Theme, prefs.Accent),
		density:      theme.ParseDensity(prefs.Density),
		showHints:    true,
		loadErr:      loadErr,
		messages:     map[string][]data.Message{},
		meta:         meta,
		myStatus:     prefs.Status,
		activeID:     activeID,
		focus:        focusMessages,
		draft:        mkInput(),
		threadDraft:  mkInput(),
		paletteQuery: mkInput(),
	}
	m.ensureHistory(activeID)
	m.sideSel = m.flatIndexOf(activeID)
	m.msgSel = max(0, len(m.messages[activeID])-1)
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{pollTick()}
	if s, ok := m.src.(streamer); ok && s.Events() != nil {
		cmds = append(cmds, listenEvents(s))
	}
	return tea.Batch(cmds...)
}

// ── derived helpers ─────────────────────────────────────────────────────────

func (m Model) sideItems() []components.SideItem {
	return components.BuildSideItems(m.ws, m.meta)
}

func (m Model) selectable() []int {
	return components.SelectableIndexes(m.sideItems())
}

func (m Model) flatIndexOf(id string) int {
	for i, it := range m.sideItems() {
		if !it.Header && it.Conv.ID == id {
			return i
		}
	}
	return 0
}

func (m Model) threadOpen() bool { return m.threadRootID != "" }

func (m Model) focusOrder() []string {
	order := []string{focusSidebar, focusMessages}
	if m.threadOpen() {
		order = append(order, focusThread)
	}
	return order
}

func (m Model) curMsgs() []data.Message { return m.messages[m.activeID] }

func (m Model) threadRoot() (data.Message, bool) {
	if m.threadRootID == "" {
		return data.Message{}, false
	}
	for _, list := range m.messages {
		for _, msg := range list {
			if msg.ID == m.threadRootID {
				return msg, true
			}
		}
	}
	return data.Message{}, false
}

// ── navigation actions ──────────────────────────────────────────────────────

// ensureHistory loads a conversation's messages once and caches them (an empty
// entry still counts as loaded, so we never refetch).
func (m *Model) ensureHistory(id string) {
	if _, ok := m.messages[id]; ok {
		return
	}
	msgs, err := m.src.History(id)
	if err != nil {
		m.loadErr = err
	}
	m.messages[id] = msgs
}

func (m *Model) openChannel(id string) {
	m.activeID = id
	m.ensureHistory(id)
	m.msgSel = max(0, len(m.messages[id])-1)
	m.meta[id] = components.Meta{Unread: 0, Mention: false}
	m.focus = focusMessages
	m.sideSel = m.flatIndexOf(id)
	m.threadRootID = ""
}

func (m *Model) openThread(msgID string) tea.Cmd {
	m.threadRootID = msgID
	m.threadSel = 0
	m.focus = focusThread
	return m.repliesCmd(m.activeID, msgID)
}

func (m *Model) closeThread() {
	m.threadRootID = ""
	m.focus = focusMessages
}

func (m *Model) enterInsert(which string) tea.Cmd {
	m.insert = true
	if which == focusThread {
		return m.threadDraft.Focus()
	}
	return m.draft.Focus()
}

func (m *Model) leaveInsert() {
	m.insert = false
	m.draft.Blur()
	m.threadDraft.Blur()
}

func (m *Model) sendMessage() {
	text := strings.TrimSpace(m.draft.Value())
	if text == "" {
		return
	}
	msg, err := m.src.Send(m.activeID, text)
	if err != nil {
		m.loadErr = err
		return
	}
	m.messages[m.activeID] = append(m.messages[m.activeID], msg)
	m.draft.SetValue("")
	m.msgSel = len(m.messages[m.activeID]) - 1
}

func (m *Model) sendReply() {
	text := strings.TrimSpace(m.threadDraft.Value())
	if text == "" || m.threadRootID == "" {
		return
	}
	r, err := m.src.SendReply(m.activeID, m.threadRootID, text)
	if err != nil {
		m.loadErr = err
		return
	}
	for k, list := range m.messages {
		for i := range list {
			if list[i].ID == m.threadRootID {
				list[i].Replies = append(list[i].Replies, r)
			}
		}
		m.messages[k] = list
	}
	m.threadDraft.SetValue("")
}

// ── update ──────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case pollMsg:
		return m, tea.Batch(pollTick(), m.refresh())
	case historyMsg:
		if msg.err == nil {
			m.applyHistory(msg.convID, msg.msgs)
		}
		return m, nil
	case repliesMsg:
		if msg.err == nil {
			m.applyReplies(msg.convID, msg.rootID, msg.replies)
		}
		return m, nil
	case eventMsg:
		m.handleEvent(msg.ev)
		if s, ok := m.src.(streamer); ok {
			return m, listenEvents(s) // keep listening
		}
		return m, nil
	case tea.KeyMsg:
		// Ctrl-K toggles the palette from any mode (Cmd never reaches a terminal).
		if msg.String() == "ctrl+k" {
			if m.paletteOpen {
				m.closePalette()
				return m, nil
			}
			return m, m.openPalette()
		}
		if m.paletteOpen {
			return m.paletteKey(msg)
		}
		if m.insert {
			return m.insertKey(msg)
		}
		return m.normalKey(msg)
	}
	// forward non-key msgs (cursor blink) to the active input
	if m.paletteOpen {
		var cmd tea.Cmd
		m.paletteQuery, cmd = m.paletteQuery.Update(msg)
		return m, cmd
	}
	if m.insert {
		var cmd tea.Cmd
		if m.focus == focusThread {
			m.threadDraft, cmd = m.threadDraft.Update(msg)
		} else {
			m.draft, cmd = m.draft.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m Model) insertKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.leaveInsert()
		return m, nil
	case "enter":
		if m.focus == focusThread {
			m.sendReply()
		} else {
			m.sendMessage()
		}
		return m, nil
	}
	var cmd tea.Cmd
	if m.focus == focusThread {
		m.threadDraft, cmd = m.threadDraft.Update(msg)
	} else {
		m.draft, cmd = m.draft.Update(msg)
	}
	return m, cmd
}

func (m Model) normalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	order := m.focusOrder()
	idx := indexOf(order, m.focus)

	switch k {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "ctrl+r": // manual refresh of the active channel + open thread
		return m, m.refresh()

	case "tab":
		m.focus = order[(idx+1)%len(order)]
	case "shift+tab":
		m.focus = order[(idx-1+len(order))%len(order)]
	case "h", "ctrl+h":
		if idx > 0 {
			m.focus = order[idx-1]
		}
	case "l", "ctrl+l":
		if idx < len(order)-1 {
			m.focus = order[idx+1]
		}

	case "g":
		if !m.gPending.IsZero() && time.Since(m.gPending) < 500*time.Millisecond {
			m.gPending = time.Time{}
			m.jumpTop()
		} else {
			m.gPending = time.Now()
		}
		return m, nil
	case "G":
		m.jumpBottom()

	case "j", "down":
		m.moveSel(1)
	case "k", "up":
		m.moveSel(-1)

	case "enter":
		switch m.focus {
		case focusSidebar:
			if it := m.sideItems()[m.sideSel]; !it.Header {
				m.openChannel(it.Conv.ID)
			}
		case focusMessages:
			if msgs := m.curMsgs(); m.msgSel < len(msgs) {
				return m, m.openThread(msgs[m.msgSel].ID)
			}
		case focusThread:
			return m, m.enterInsert(focusThread)
		}

	case "t":
		if m.focus == focusMessages {
			if msgs := m.curMsgs(); m.msgSel < len(msgs) {
				return m, m.openThread(msgs[m.msgSel].ID)
			}
		}

	case "i":
		which := focusMessages
		if m.focus == focusThread {
			which = focusThread
		}
		if m.focus == focusSidebar {
			m.focus = focusMessages
		}
		return m, m.enterInsert(which)

	case "r":
		if m.focus == focusThread {
			return m, m.enterInsert(focusThread)
		}
		if m.focus == focusMessages {
			if msgs := m.curMsgs(); m.msgSel < len(msgs) {
				openCmd := m.openThread(msgs[m.msgSel].ID)
				return m, tea.Batch(openCmd, m.enterInsert(focusThread))
			}
		}

	case "esc":
		if m.threadOpen() {
			m.closeThread()
		}
	}

	// any non-g key cancels a pending gg
	if k != "g" {
		m.gPending = time.Time{}
	}
	return m, nil
}

func (m *Model) moveSel(delta int) {
	switch m.focus {
	case focusSidebar:
		sel := m.selectable()
		pos := indexOfInt(sel, m.sideSel)
		pos = clamp(pos+delta, 0, len(sel)-1)
		m.sideSel = sel[pos]
	case focusMessages:
		m.msgSel = clamp(m.msgSel+delta, 0, len(m.curMsgs())-1)
	case focusThread:
		if root, ok := m.threadRoot(); ok {
			m.threadSel = clamp(m.threadSel+delta, 0, len(root.Replies)-1)
		}
	}
}

func (m *Model) jumpTop() {
	switch m.focus {
	case focusSidebar:
		if sel := m.selectable(); len(sel) > 0 {
			m.sideSel = sel[0]
		}
	case focusMessages:
		m.msgSel = 0
	case focusThread:
		m.threadSel = 0
	}
}

func (m *Model) jumpBottom() {
	switch m.focus {
	case focusSidebar:
		if sel := m.selectable(); len(sel) > 0 {
			m.sideSel = sel[len(sel)-1]
		}
	case focusMessages:
		m.msgSel = max(0, len(m.curMsgs())-1)
	case focusThread:
		if root, ok := m.threadRoot(); ok {
			m.threadSel = max(0, len(root.Replies)-1)
		}
	}
}

// ── small utilities ─────────────────────────────────────────────────────────

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return 0
}

func indexOfInt(s []int, v int) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return 0
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// gap renders a 1-col vertical separator filled with the app background.
func (m Model) gap(height int) string {
	return lipgloss.NewStyle().Width(1).Height(height).Background(m.pal.Bg).Render("")
}

// overlay composites an over block onto base at cell (x,y), ansi-aware.
func overlay(base, over string, x, y int) string {
	baseLines := strings.Split(base, "\n")
	overLines := strings.Split(over, "\n")
	for i, ol := range overLines {
		row := y + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		baseLines[row] = spliceLine(baseLines[row], ol, x)
	}
	return strings.Join(baseLines, "\n")
}

// spliceLine replaces the [x, x+width(over)) slice of base with over, ansi-aware,
// padding the left segment with spaces if base is shorter than x.
func spliceLine(base, over string, x int) string {
	ow := lipgloss.Width(over)
	left := ansi.Truncate(base, x, "")
	if lw := lipgloss.Width(left); lw < x {
		left += strings.Repeat(" ", x-lw)
	}
	right := ansi.TruncateLeft(base, x+ow, "")
	return left + "\x1b[0m" + over + "\x1b[0m" + right
}

// windowLines slices lines to a height-h window that keeps [selStart,selEnd] visible.
func windowLines(lines []string, selStart, selEnd, h int) []string {
	if h <= 0 {
		return nil
	}
	if len(lines) <= h {
		return lines
	}
	top := 0
	if selEnd >= h {
		top = selEnd - h + 1
	}
	if selStart < top {
		top = selStart
	}
	top = clamp(top, 0, len(lines)-h)
	return lines[top : top+h]
}
