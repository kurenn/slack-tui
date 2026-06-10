// Package app is the root Bubble Tea model: navigation state, the modal
// NORMAL/INSERT keyboard engine (ported from tui-app.jsx's keydown router), and
// the View that composes the panes. Data still comes from the mock workspace.
package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/source"
	"github.com/kurenn/slack-tui/internal/theme"
	"github.com/kurenn/slack-tui/internal/ui/components"
)

const (
	sidebarWidth = 30
	threadWidth  = 44

	focusSidebar  = "sidebar"
	focusMessages = "messages"
	focusThread   = "thread"
)

// composerLines is how many draft rows the composer shows before scrolling.
const maxComposerLines = 4

// draftWidth / threadDraftWidth mirror the textarea widths set at render time —
// height math must wrap at the same column the textarea does.
func (m Model) draftWidth() int {
	centerW := m.width - sidebarWidth - 1
	if m.threadOpen() {
		centerW = m.width - sidebarWidth - threadWidth - 2
	}
	return max(4, (centerW - 2) - 24)
}

func (m Model) threadDraftWidth() int {
	return max(4, (threadWidth - 2) - 24)
}

// wrappedRows counts the display rows value occupies in a textarea of width w —
// logical lines plus soft-wrapping (pasted code often has lines wider than the
// composer; counting only "\n" would leave the box too short to show them).
func wrappedRows(value string, w int) int {
	if w < 1 {
		return 1
	}
	rows := 0
	for _, ln := range strings.Split(value, "\n") {
		rows += lipgloss.Width(ln)/w + 1 // textarea wraps the cursor cell too
	}
	return max(1, rows)
}

// composerHeight is the center composer's total height (borders + draft rows).
func (m Model) composerHeight() int {
	return 2 + clamp(wrappedRows(m.draft.Value(), m.draftWidth()), 1, maxComposerLines)
}

// threadComposerHeight is the thread composer's total height.
func (m Model) threadComposerHeight() int {
	return 2 + clamp(wrappedRows(m.threadDraft.Value(), m.threadDraftWidth()), 1, maxComposerLines)
}

// syncComposerSizes pushes the current geometry into the persistent textareas.
// This must happen at update time (not on render copies): the textarea
// repositions its viewport to the cursor inside Update, using whatever size it
// has then — sizing only a render copy leaves the real one at a stale height,
// which is how pasted code ended up typing into an invisible row.
func (m *Model) syncComposerSizes() {
	m.draft.SetWidth(m.draftWidth())
	m.draft.SetHeight(m.composerHeight() - 2)
	m.threadDraft.SetWidth(m.threadDraftWidth())
	m.threadDraft.SetHeight(m.threadComposerHeight() - 2)
	// Settle the viewports: the textarea only loads content into its viewport
	// during View, and only repositions to the cursor during Update — so a
	// paste that grows the box and moves the cursor in one event would stay
	// scrolled wrong until the next keystroke. View-then-Update(nil) runs both
	// steps now, against the new size.
	_ = m.draft.View()
	m.draft, _ = m.draft.Update(nil)
	_ = m.threadDraft.View()
	m.threadDraft, _ = m.threadDraft.Update(nil)
}

// Model is the application state.
type Model struct {
	src       source.Source
	ws        *data.Workspace
	prefs     config.Prefs
	pal       theme.Palette
	density   theme.Density
	showHints bool
	loadErr   error

	messages    map[string][]data.Message
	fullyLoaded map[string]bool // conversations with no more older history
	loading     map[string]bool // conversations with an in-flight history fetch
	meta        map[string]components.Meta
	myStatus    string

	activeID     string
	focus        string
	insert       bool
	sideSel      int // flat index into sidebar items
	msgSel       int
	msgExtra     int // extra line scroll within the message pane (for tall messages)
	threadRootID string
	threadSel    int

	draft       textarea.Model
	threadDraft textarea.Model
	drafts      map[string]string // per-conversation unsent draft text

	pendingSeq int // counter for optimistic (not yet acked) send IDs
	errSeq     int // generation counter for the auto-clearing error banner
	lastTitleN int // last unread total pushed to the terminal title

	helpOpen bool

	editingID string // message being edited in the composer ("" = composing new)
	prevDraft string // draft parked while editing
	dPending  time.Time
	suggest   suggestState // composer autocomplete popup

	picker      pickerState
	pickerInput textinput.Model
	confirm     confirmState

	findOpen    bool
	findInput   textinput.Model
	searchQuery string // last local-find query (n/N repeat)

	newMark       map[string]string // convID → first-unread message ID (the ── new ── rule)
	pendingUnread map[string]int    // unread count snapshot for channels opening async
	pendingSelect map[string]string // convID → message ID to select once history lands

	paletteOpen  bool
	paletteQuery textinput.Model
	paletteIndex int

	settingsOpen bool
	settingsSel  int

	statusTextOpen  bool
	statusTextInput textinput.Model

	width, height int
	gPending      time.Time
}

// New builds the production model from saved (or default) prefs. The data
// source is real Slack when a user token is configured (env or tokens.json),
// otherwise the local mock. Call it off the UI thread — Load talks to the
// network (root runs it inside a tea.Cmd behind a connecting screen).
func New() Model {
	prefs, _ := config.Load()

	// Tokens come from the stored file, with env vars overriding per-token.
	saved, _ := config.LoadTokens()
	tok := saved.Resolve()

	var src source.Source
	if tok.User != "" {
		sl := source.NewSlack(tok.User)
		sl.SetGroupDMs(prefs.GroupDMs)
		src = sl
	} else {
		src = source.NewMock()
	}
	m := NewWith(src, prefs)

	// Real-time: start Socket Mode if the app + bot tokens are present (m.src
	// may have fallen back to the mock if Load failed).
	if sl, ok := m.src.(*source.Slack); ok && tok.App != "" && tok.Bot != "" {
		sl.StartSocket(tok.App, tok.Bot)
	}
	return m
}

// NewWith builds the model on an explicit source and prefs. Tests inject the
// mock here so they never touch the network or the user's config files.
func NewWith(src source.Source, prefs config.Prefs) Model {
	ws, err := src.Load()
	var loadErr error
	if err != nil { // fall back to the mock so the app still runs; surface the error
		loadErr = err
		src = source.NewMock()
		ws, _ = src.Load()
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
	mkArea := func() textarea.Model {
		ta := textarea.New()
		ta.Prompt = ""
		ta.ShowLineNumbers = false
		ta.CharLimit = 0
		ta.SetHeight(1)
		// Enter sends (handled by insertKey); newlines come from alt+enter/ctrl+j.
		ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
		// The composer draws its own chrome — flatten the textarea's.
		ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
		ta.FocusedStyle.Base = lipgloss.NewStyle()
		ta.BlurredStyle.Base = lipgloss.NewStyle()
		return ta
	}

	activeID := "engineering" // the mock's demo channel
	if _, ok := ws.Conversation(activeID); !ok { // real Slack: pick the first channel/DM
		switch {
		case len(ws.Channels) > 0:
			activeID = ws.Channels[0].ID
		case len(ws.DMs) > 0:
			activeID = ws.DMs[0].ID
		}
	}

	m := Model{
		src:             src,
		ws:              ws,
		prefs:           prefs,
		pal:             theme.Resolve(prefs.Theme, prefs.Accent),
		density:         theme.ParseDensity(prefs.Density),
		showHints:       true,
		loadErr:         loadErr,
		messages:        map[string][]data.Message{},
		fullyLoaded:     map[string]bool{},
		loading:         map[string]bool{},
		meta:            meta,
		myStatus:        prefs.Status,
		activeID:        activeID,
		focus:           focusMessages,
		draft:           mkArea(),
		threadDraft:     mkArea(),
		drafts:          map[string]string{},
		paletteQuery:    mkInput(),
		statusTextInput: mkInput(),
		pickerInput:     mkInput(),
		findInput:       mkInput(),
		newMark:         map[string]string{},
		pendingUnread:   map[string]int{},
		pendingSelect:   map[string]string{},
	}
	m.ensureHistory(activeID)
	m.sideSel = m.flatIndexOf(activeID)
	m.msgSel = max(0, len(m.messages[activeID])-1)
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{pollTick()}
	if sl, ok := m.src.(*source.Slack); ok {
		if sl.Events() != nil {
			cmds = append(cmds, listenEvents(sl)) // live channel events
		} else {
			cmds = append(cmds, chanPollTick()) // no Socket Mode: poll channel unread instead
		}
		cmds = append(cmds, dmPollTick()) // periodic DM unread (Socket Mode can't see DMs)
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

func (m *Model) openChannel(id string) tea.Cmd {
	if id != m.activeID { // park the unsent draft; restore the target's
		m.drafts[m.activeID] = m.draft.Value()
		m.draft.SetValue(m.drafts[id])
		m.syncComposerSizes()
	}
	unread := m.meta[id].Unread // captured before clearing — drives the ── new ── rule
	m.activeID = id
	m.msgExtra = 0
	m.meta[id] = components.Meta{Unread: 0, Mention: false}
	m.focus = focusMessages
	m.sideSel = m.flatIndexOf(id)
	m.threadRootID = ""
	delete(m.newMark, id)
	if _, ok := m.messages[id]; !ok {
		if _, instant := m.src.(*source.Mock); instant {
			m.ensureHistory(id) // the mock answers in-memory — keep it synchronous
		} else { // real Slack: fetch off the UI thread, show "loading…" meanwhile
			m.loading[id] = true
			if unread > 0 {
				m.pendingUnread[id] = unread // mark + cursor computed when history lands
			}
			m.msgSel = 0
			return tea.Batch(m.historyCmd(id), m.titleCmd())
		}
	}
	m.msgSel = max(0, len(m.messages[id])-1)
	m.applyNewMark(id, unread)
	m.applyPendingSelect(id)
	return tea.Batch(m.markReadCmd(id), m.titleCmd()) // persist read state to the backend
}

// applyNewMark places the ── new ── rule before the first unread message and
// lands the cursor there (unread counts messages after last_read, so the first
// unread is len-unread).
func (m *Model) applyNewMark(id string, unread int) {
	msgs := m.messages[id]
	if unread <= 0 || len(msgs) == 0 {
		return
	}
	idx := max(0, len(msgs)-unread)
	m.newMark[id] = msgs[idx].ID
	if id == m.activeID {
		m.msgSel, m.msgExtra = idx, 0
	}
}

// applyPendingSelect moves the cursor to a requested message (search jump).
func (m *Model) applyPendingSelect(id string) {
	want, ok := m.pendingSelect[id]
	if !ok {
		return
	}
	delete(m.pendingSelect, id)
	for i, msg := range m.messages[id] {
		if msg.ID == want && id == m.activeID {
			m.msgSel, m.msgExtra = i, 0
			return
		}
	}
}

// pageJump is the selection step for half-page scrolling (Ctrl-d/Ctrl-u).
func (m Model) pageJump() int {
	if n := (m.height - 6) / 4; n > 1 {
		return n
	}
	return 1
}

// msgGeom computes the selected-message line span, total body lines, and the
// message-pane inner height — for line-accurate scrolling.
func (m Model) msgGeom() (ss, se, total, innerH int) {
	centerW := m.width - sidebarWidth - 1
	if m.threadOpen() {
		centerW = m.width - sidebarWidth - threadWidth - 2
	}
	innerH = (m.height - 2 - m.composerHeight()) - 2
	msgs := m.curMsgs()
	lines, starts := components.MessagesBody(m.pal, m.ws, msgs, m.msgSel, m.focus == focusMessages, m.density, centerW-2, m.newMark[m.activeID])
	total = len(lines)
	if n := len(msgs); n > 0 {
		mi := clamp(m.msgSel, 0, n-1)
		ss, se = starts[mi], starts[mi+1]-1
	}
	return
}

// scrollMessages line-scrolls the message pane by step (Ctrl-d/Ctrl-u), letting
// you read through a message taller than the viewport. Other panes move selection.
func (m *Model) scrollMessages(step int) {
	if m.focus != focusMessages {
		m.moveSel(step)
		return
	}
	ss, se, total, innerH := m.msgGeom()
	base := windowBaseTop(ss, se, innerH, total)
	maxTop := max(0, total-innerH)
	m.msgExtra = clamp(base+m.msgExtra+step, 0, maxTop) - base
}

// msgScrollStep is the per-keystroke line step when j/k scrolls within a message
// taller than the viewport.
const msgScrollStep = 3

// messagesDown handles j/↓ in the message pane: scroll down through the selected
// message if it overflows the viewport, otherwise advance to the next message.
func (m *Model) messagesDown() {
	msgs := m.curMsgs()
	ss, se, total, innerH := m.msgGeom()
	if total > innerH { // there is something to scroll
		base := windowBaseTop(ss, se, innerH, total)
		curTop := clamp(base+m.msgExtra, 0, total-innerH)
		if se > curTop+innerH-1 { // selected message extends below the viewport
			m.msgExtra = clamp(curTop+msgScrollStep, 0, total-innerH) - base
			return
		}
	}
	if m.msgSel < len(msgs)-1 {
		m.msgSel++
		m.msgExtra = 0
	}
}

// messagesUp handles k/↑ in the message pane: scroll back up within a tall
// message, otherwise move to the previous message.
func (m *Model) messagesUp() {
	if m.msgExtra > 0 {
		ss, se, total, innerH := m.msgGeom()
		base := windowBaseTop(ss, se, innerH, total)
		curTop := clamp(base+m.msgExtra, 0, max(0, total-innerH))
		m.msgExtra = max(0, clamp(curTop-msgScrollStep, 0, max(0, total-innerH))-base)
		return
	}
	if m.msgSel > 0 {
		m.msgSel--
		m.msgExtra = 0
	}
}

// atHistoryTop reports whether the message list is focused, scrolled to its
// first message, and may have more (unloaded) older history.
func (m Model) atHistoryTop() bool {
	return m.focus == focusMessages && m.msgSel == 0 && len(m.curMsgs()) > 0 && !m.fullyLoaded[m.activeID]
}

// jumpUnread opens the next/prev conversation (in sidebar order) with unread,
// wrapping around. No-op if nothing is unread.
func (m *Model) jumpUnread(dir int) tea.Cmd {
	var ids []string
	for _, c := range m.ws.Channels {
		ids = append(ids, c.ID)
	}
	for _, c := range m.ws.DMs {
		ids = append(ids, c.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	start := 0
	for i, id := range ids {
		if id == m.activeID {
			start = i
			break
		}
	}
	for k := 1; k <= len(ids); k++ {
		i := ((start+dir*k)%len(ids) + len(ids)) % len(ids)
		if m.meta[ids[i]].Unread > 0 {
			return m.openChannel(ids[i])
		}
	}
	return nil
}

func (m *Model) openThread(msgID string) tea.Cmd {
	m.threadRootID = msgID
	m.threadSel = 0
	m.focus = focusThread
	m.syncComposerSizes() // the center pane narrows when the thread opens
	return m.repliesCmd(m.activeID, msgID)
}

func (m *Model) closeThread() {
	m.threadRootID = ""
	m.focus = focusMessages
	m.syncComposerSizes()
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
	m.clearSuggest()
	m.draft.Blur()
	m.threadDraft.Blur()
}

// pendingPrefix marks optimistic local messages awaiting the backend's ack.
const pendingPrefix = "pending-"

func nowClock() string { return time.Now().Format("15:04") }

// sendMessage appends the draft optimistically and posts it off the UI thread;
// sentMsg reconciles (real ID on success, removal + draft restore on error).
func (m *Model) sendMessage() tea.Cmd {
	text := strings.TrimSpace(m.draft.Value())
	if text == "" {
		return nil
	}
	m.pendingSeq++
	pid := fmt.Sprintf("%s%d", pendingPrefix, m.pendingSeq)
	m.messages[m.activeID] = append(m.messages[m.activeID],
		data.Message{ID: pid, UserID: m.ws.MeID, Time: nowClock(), Text: text})
	m.draft.SetValue("")
	m.clearSuggest()
	m.syncComposerSizes()
	m.msgSel = len(m.messages[m.activeID]) - 1
	m.msgExtra = 0
	src, conv := m.src, m.activeID
	return func() tea.Msg {
		msg, err := src.Send(conv, text)
		return sentMsg{convID: conv, pendingID: pid, text: text, msg: msg, err: err}
	}
}

// sendReply mirrors sendMessage for thread replies.
func (m *Model) sendReply() tea.Cmd {
	text := strings.TrimSpace(m.threadDraft.Value())
	if text == "" || m.threadRootID == "" {
		return nil
	}
	m.pendingSeq++
	pid := fmt.Sprintf("%s%d", pendingPrefix, m.pendingSeq)
	m.eachRootMsg(m.threadRootID, func(msg *data.Message) {
		msg.Replies = append(msg.Replies, data.Reply{ID: pid, UserID: m.ws.MeID, Time: nowClock(), Text: text})
	})
	m.threadDraft.SetValue("")
	m.clearSuggest()
	m.syncComposerSizes()
	src, conv, root := m.src, m.activeID, m.threadRootID
	return func() tea.Msg {
		r, err := src.SendReply(conv, root, text)
		return sentReplyMsg{convID: conv, rootID: root, pendingID: pid, text: text, reply: r, err: err}
	}
}

// eachRootMsg runs fn on every cached copy of the message with the given ID.
func (m *Model) eachRootMsg(id string, fn func(*data.Message)) {
	for k, list := range m.messages {
		for i := range list {
			if list[i].ID == id {
				fn(&list[i])
			}
		}
		m.messages[k] = list
	}
}

// ── update ──────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncComposerSizes()
		return m, nil
	case pollMsg:
		return m, tea.Batch(pollTick(), m.refresh(), m.markReadCmd(m.activeID))
	case dmPollMsg:
		return m, tea.Batch(dmPollTick(), m.unreadCmd(m.dmIDs()))
	case chanPollMsg:
		return m, tea.Batch(chanPollTick(), m.unreadCmd(m.chanIDs()))
	case unreadMsg:
		for id, n := range msg.counts { // only ids actually fetched this round
			if id == m.activeID {
				continue
			}
			mm := m.meta[id]
			mm.Unread = n
			if n == 0 {
				mm.Mention = false
			}
			m.meta[id] = mm
		}
		return m, m.titleCmd()
	case presenceMsg:
		return m, m.flash(msg.err)
	case historyMsg:
		delete(m.loading, msg.convID)
		if msg.err != nil {
			return m, m.flash(msg.err)
		}
		m.applyHistory(msg.convID, msg.msgs)
		if n, ok := m.pendingUnread[msg.convID]; ok { // async channel open: place the ── new ── rule
			delete(m.pendingUnread, msg.convID)
			m.applyNewMark(msg.convID, n)
		}
		m.applyPendingSelect(msg.convID)
		if msg.convID == m.activeID {
			return m, m.markReadCmd(msg.convID)
		}
		return m, nil
	case reactMsg:
		return m, m.applyReact(msg)
	case editMsg:
		return m, m.flash(msg.err)
	case deleteMsg:
		return m, m.flash(msg.err)
	case searchMsg:
		return m, m.applySearch(msg)
	case joinableMsg:
		return m, m.applyJoinable(msg)
	case joinedMsg:
		return m, m.applyJoined(msg)
	case wsMsg:
		return m, m.applyWorkspace(msg)
	case repliesMsg:
		if msg.err != nil {
			return m, m.flash(msg.err)
		}
		m.applyReplies(msg.convID, msg.rootID, msg.replies)
		return m, nil
	case olderMsg:
		if msg.err != nil {
			return m, m.flash(msg.err)
		}
		m.prependHistory(msg.convID, msg.msgs)
		return m, nil
	case sentMsg:
		return m, m.applySent(msg)
	case sentReplyMsg:
		return m, m.applySentReply(msg)
	case clearErrMsg:
		if msg.seq == m.errSeq {
			m.loadErr = nil
		}
		return m, nil
	case eventMsg:
		notify := m.handleEvent(msg.ev)
		cmds := []tea.Cmd{m.titleCmd()}
		if notify {
			cmds = append(cmds, bellCmd)
		}
		if s, ok := m.src.(streamer); ok {
			cmds = append(cmds, listenEvents(s)) // keep listening
		}
		return m, tea.Batch(cmds...)
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollMessages(-msgScrollStep)
		case tea.MouseButtonWheelDown:
			m.scrollMessages(msgScrollStep)
		}
		return m, nil
	case tea.KeyMsg:
		// Ctrl-K toggles the palette from any mode (Cmd never reaches a terminal).
		if msg.String() == "ctrl+k" {
			if m.paletteOpen {
				m.closePalette()
				return m, nil
			}
			m.closePicker() // the palette supersedes any open picker
			return m, m.openPalette()
		}
		if m.helpOpen {
			return m.helpKey(msg)
		}
		if m.confirm.open {
			return m.confirmKey(msg)
		}
		if m.picker.open {
			return m.pickerKey(msg)
		}
		if m.findOpen {
			return m.findKey(msg)
		}
		if m.statusTextOpen {
			return m.statusTextKey(msg)
		}
		if m.settingsOpen {
			return m.settingsKey(msg)
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
	if m.picker.open {
		var cmd tea.Cmd
		m.pickerInput, cmd = m.pickerInput.Update(msg)
		return m, cmd
	}
	if m.findOpen {
		var cmd tea.Cmd
		m.findInput, cmd = m.findInput.Update(msg)
		return m, cmd
	}
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
	if m.suggestActive() && m.suggestKey(msg.String()) {
		return m, nil // the popup consumed the key
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.editingID != "" {
			m.cancelEdit()
		}
		m.leaveInsert()
		return m, nil
	case "enter":
		if m.focus == focusThread {
			return m, m.sendReply()
		}
		if m.editingID != "" {
			return m, m.applyEditDraft()
		}
		return m, m.sendMessage()
	}
	var cmd tea.Cmd
	if m.focus == focusThread {
		m.threadDraft, cmd = m.threadDraft.Update(msg)
	} else {
		m.draft, cmd = m.draft.Update(msg)
	}
	m.syncComposerSizes() // the content may have grown/shrunk the composer
	m.recomputeSuggest()
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
	case ",": // open the settings overlay
		m.openSettings()
		return m, nil
	case "?": // keymap cheat sheet
		m.helpOpen = true
		return m, nil
	case "]": // jump to the next conversation with unread
		return m, m.jumpUnread(1)
	case "[": // jump to the previous conversation with unread
		return m, m.jumpUnread(-1)

	case "tab":
		m.focus = order[(idx+1)%len(order)]
	case "shift+tab":
		m.focus = order[(idx-1+len(order))%len(order)]
	case "h", "ctrl+h", "left":
		if idx > 0 {
			m.focus = order[idx-1]
		}
	case "l", "ctrl+l", "right":
		if idx < len(order)-1 {
			m.focus = order[idx+1]
		}

	case "g":
		if !m.gPending.IsZero() && time.Since(m.gPending) < 500*time.Millisecond {
			m.gPending = time.Time{}
			m.jumpTop()
			if m.atHistoryTop() {
				return m, m.loadOlderCmd(m.activeID)
			}
		} else {
			m.gPending = time.Now()
		}
		return m, nil
	case "G":
		m.jumpBottom()

	case "j", "down":
		if m.focus == focusMessages {
			m.messagesDown()
		} else {
			m.moveSel(1)
		}
	case "k", "up":
		if m.focus == focusMessages {
			if m.atHistoryTop() && m.msgExtra == 0 { // top of the list: pull older history
				return m, m.loadOlderCmd(m.activeID)
			}
			m.messagesUp()
		} else {
			m.moveSel(-1)
		}
	case "ctrl+d":
		m.scrollMessages(m.pageJump())
	case "ctrl+u":
		m.scrollMessages(-m.pageJump())

	case "enter":
		switch m.focus {
		case focusSidebar:
			if it := m.sideItems()[m.sideSel]; !it.Header {
				return m, m.openChannel(it.Conv.ID)
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

	case "a": // react to the selected message
		if msg, ok := m.selectedMsg(); ok && !isPending(msg.ID) {
			return m, m.openReactionPicker(msg.ID)
		}

	case "e": // edit your own selected message
		if msg, ok := m.selectedMsg(); ok && msg.UserID == m.ws.MeID && !isPending(msg.ID) {
			return m, m.startEdit(msg)
		}

	case "d": // dd deletes your own selected message (with confirm)
		if !m.dPending.IsZero() && time.Since(m.dPending) < 500*time.Millisecond {
			m.dPending = time.Time{}
			if msg, ok := m.selectedMsg(); ok && msg.UserID == m.ws.MeID && !isPending(msg.ID) {
				id := msg.ID
				m.openConfirm("delete this message?", func(mm *Model) tea.Cmd {
					return mm.deleteMessage(id)
				})
			}
		} else {
			m.dPending = time.Now()
		}
		return m, nil

	case "o": // open the selected message's link(s)
		if m.focus == focusMessages {
			return m, m.openMsgLinks()
		}

	case "/": // find within the loaded channel history
		return m, m.openFind()
	case "n":
		if m.focus == focusMessages && m.searchQuery != "" {
			return m, m.jumpFind(1)
		}
	case "N":
		if m.focus == focusMessages && m.searchQuery != "" {
			return m, m.jumpFind(-1)
		}

	case "s": // workspace-wide message search
		return m, m.openSearchPicker()

	case "y": // yank the selected message's text to the clipboard
		if msg, ok := m.selectedMsg(); ok {
			if err := clipboard.WriteAll(msg.Text); err != nil {
				return m, m.flash(fmt.Errorf("clipboard: %w", err))
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
		if m.loadErr != nil { // dismiss the error banner first
			m.loadErr = nil
			return m, nil
		}
		if m.threadOpen() {
			m.closeThread()
		}
	}

	// any non-g key cancels a pending gg (same for dd)
	if k != "g" {
		m.gPending = time.Time{}
	}
	if k != "d" {
		m.dPending = time.Time{}
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
		m.msgExtra = 0
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
		m.msgSel, m.msgExtra = 0, 0
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
		m.msgSel, m.msgExtra = max(0, len(m.curMsgs())-1), 0
	case focusThread:
		if root, ok := m.threadRoot(); ok {
			m.threadSel = max(0, len(root.Replies)-1)
		}
	}
}

// ── async send reconciliation, error flash, notifications ──────────────────

// flash surfaces an error in the banner and schedules its auto-clear. Returns
// nil for a nil error, so handlers can `return m, m.flash(msg.err)` directly.
func (m *Model) flash(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	m.loadErr = err
	m.errSeq++
	seq := m.errSeq
	return tea.Tick(8*time.Second, func(time.Time) tea.Msg { return clearErrMsg{seq} })
}

// applySent reconciles an optimistic message with the backend's answer: swap in
// the real message, or roll back and restore the draft on error.
func (m *Model) applySent(msg sentMsg) tea.Cmd {
	list := m.messages[msg.convID]
	idx := -1
	for i := range list {
		if list[i].ID == msg.pendingID {
			idx = i
			break
		}
	}
	if msg.err != nil {
		if idx >= 0 {
			m.messages[msg.convID] = append(list[:idx], list[idx+1:]...)
			if msg.convID == m.activeID {
				m.msgSel = clamp(m.msgSel, 0, max(0, len(m.messages[msg.convID])-1))
			}
		}
		if msg.convID == m.activeID && strings.TrimSpace(m.draft.Value()) == "" {
			m.draft.SetValue(msg.text) // give the user their words back
			m.syncComposerSizes()
		}
		return m.flash(msg.err)
	}
	if idx >= 0 {
		already := false // a poll may have delivered the real message first
		for i := range list {
			if list[i].ID == msg.msg.ID {
				already = true
				break
			}
		}
		if already {
			m.messages[msg.convID] = append(list[:idx], list[idx+1:]...)
		} else {
			list[idx] = msg.msg
			m.messages[msg.convID] = list
		}
	}
	return nil
}

// applySentReply is applySent for thread replies.
func (m *Model) applySentReply(msg sentReplyMsg) tea.Cmd {
	if msg.err != nil {
		m.eachRootMsg(msg.rootID, func(root *data.Message) {
			for i := range root.Replies {
				if root.Replies[i].ID == msg.pendingID {
					root.Replies = append(root.Replies[:i], root.Replies[i+1:]...)
					break
				}
			}
		})
		if m.threadRootID == msg.rootID && strings.TrimSpace(m.threadDraft.Value()) == "" {
			m.threadDraft.SetValue(msg.text)
		}
		return m.flash(msg.err)
	}
	m.eachRootMsg(msg.rootID, func(root *data.Message) {
		for i := range root.Replies {
			if root.Replies[i].ID == msg.pendingID {
				root.Replies[i] = msg.reply
				return
			}
		}
	})
	return nil
}

// titleCmd reflects the total unread count in the terminal title (when changed).
func (m *Model) titleCmd() tea.Cmd {
	n := 0
	for _, mm := range m.meta {
		n += mm.Unread
	}
	if n == m.lastTitleN {
		return nil
	}
	m.lastTitleN = n
	title := "slack-tui"
	if n > 0 {
		title = fmt.Sprintf("slack-tui (%d)", n)
	}
	return tea.SetWindowTitle(title)
}

// bellCmd rings the terminal bell — BEL prints nothing, so it can't disturb the
// alt screen. Terminals surface it as the usual badge/sound.
func bellCmd() tea.Msg {
	_, _ = os.Stdout.WriteString("\a")
	return nil
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
	top := windowBaseTop(selStart, selEnd, h, len(lines))
	return lines[top : top+h]
}

// windowBaseTop is the viewport top that keeps the selection [selStart,selEnd]
// visible (anchored to its bottom, but never past its top).
func windowBaseTop(selStart, selEnd, h, total int) int {
	if h <= 0 || total <= h {
		return 0
	}
	top := 0
	if selEnd >= h {
		top = selEnd - h + 1
	}
	if selStart < top {
		top = selStart
	}
	return clamp(top, 0, total-h)
}
