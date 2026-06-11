package app

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/source"
)

// dmPollInterval refreshes DM unread counts — Socket Mode can't see personal DMs.
const dmPollInterval = 45 * time.Second

// chanPollInterval refreshes channel unread counts when Socket Mode isn't
// running (user token only) — without it the sidebar badges would freeze at
// their startup values.
const chanPollInterval = 90 * time.Second

type (
	dmPollMsg   struct{}
	chanPollMsg struct{}
	// unreadMsg carries the unread counts actually fetched this round (a
	// rate-limit abort returns a partial map; absent ids stay untouched).
	unreadMsg struct{ counts map[string]int }
)

func dmPollTick() tea.Cmd {
	return tea.Tick(dmPollInterval, func(time.Time) tea.Msg { return dmPollMsg{} })
}

func chanPollTick() tea.Cmd {
	return tea.Tick(chanPollInterval, func(time.Time) tea.Msg { return chanPollMsg{} })
}

type presenceMsg struct{ err error }

// setPresenceCmd pushes the current presence (online/away/dnd) to the backend,
// surfacing any error (e.g. a missing users:write scope).
func (m Model) setPresenceCmd() tea.Cmd {
	src, status := m.src, m.myStatus
	return func() tea.Msg { return presenceMsg{src.SetPresence(status)} }
}

// markReadCmd tells the backend the conversation is read up to its latest message
// (fire-and-forget), so unread polls/Socket Mode don't re-flag it.
func (m Model) markReadCmd(convID string) tea.Cmd {
	msgs := m.messages[convID]
	if len(msgs) == 0 {
		return nil
	}
	ts := msgs[len(msgs)-1].ID
	src := m.src
	return func() tea.Msg { _ = src.MarkRead(convID, ts); return nil }
}

// dmIDs / chanIDs list the conversations to poll, excluding the active one.
func (m Model) dmIDs() []string {
	ids := make([]string, 0, len(m.ws.DMs))
	for _, d := range m.ws.DMs {
		if d.ID != m.activeID {
			ids = append(ids, d.ID)
		}
	}
	return ids
}

func (m Model) chanIDs() []string {
	ids := make([]string, 0, len(m.ws.Channels))
	for _, c := range m.ws.Channels {
		if c.ID != m.activeID {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// unreadCmd fetches unread counts for the given conversations, concurrently,
// off the UI thread. On a rate-limit error it stops issuing further calls and
// reports only what it fetched.
func (m Model) unreadCmd(ids []string) tea.Cmd {
	if len(ids) == 0 {
		return nil
	}
	src := m.src
	return func() tea.Msg {
		counts := map[string]int{}
		sem := make(chan struct{}, 8)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var limited atomic.Bool
		for _, id := range ids {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if limited.Load() {
					return
				}
				n, err := src.Unread(id)
				if err != nil {
					if source.IsRateLimited(err) {
						limited.Store(true) // back off; finish this round with what we have
					}
					return
				}
				mu.Lock()
				counts[id] = n
				mu.Unlock()
			}(id)
		}
		wg.Wait()
		return unreadMsg{counts}
	}
}

// markAllReadCmd marks each conversation read on the backend up to its latest
// message (fetched if not cached). Used by the palette's "Mark all as read".
func (m Model) markAllReadCmd(ids []string) tea.Cmd {
	if len(ids) == 0 {
		return nil
	}
	src := m.src
	latest := map[string]string{}
	for _, id := range ids {
		if msgs := m.messages[id]; len(msgs) > 0 {
			latest[id] = msgs[len(msgs)-1].ID
		}
	}
	return func() tea.Msg {
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup
		for _, id := range ids {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				ts := latest[id]
				if ts == "" {
					msgs, err := src.History(id)
					if err != nil || len(msgs) == 0 {
						return
					}
					ts = msgs[len(msgs)-1].ID
				}
				_ = src.MarkRead(id, ts)
			}(id)
		}
		wg.Wait()
		return nil
	}
}

// streamer is a Source that pushes real-time events (Socket Mode).
type streamer interface {
	Events() <-chan source.Event
}

// eventMsg carries one real-time Socket Mode event into the update loop.
type eventMsg struct{ ev source.Event }

// listenEvents blocks on the next stream event and delivers it as a msg.
func listenEvents(s streamer) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-s.Events()
		if !ok {
			return nil
		}
		return eventMsg{ev}
	}
}

// handleEvent applies a live message event: a message in a non-active channel
// bumps its unread (and mention) so the sidebar dot lights up immediately. The
// active channel and open thread are kept fresh by polling. Returns whether the
// event deserves a notification (mention or DM).
func (m *Model) handleEvent(ev source.Event) bool {
	if ev.ConvID == "" || ev.ConvID == m.activeID {
		return false
	}
	conv, ok := m.ws.Conversation(ev.ConvID)
	if !ok {
		return false
	}
	if ev.Msg.UserID == m.ws.MeID {
		return false // our own message from another client
	}
	meta := m.meta[ev.ConvID]
	meta.Unread++
	if ev.Msg.MentionsMe {
		meta.Mention = true
	}
	m.meta[ev.ConvID] = meta
	return ev.Msg.MentionsMe || conv.Type == "dm"
}

// pollInterval is how often the active channel (and open thread) are refreshed.
const pollInterval = 6 * time.Second

type (
	pollMsg    struct{}
	historyMsg struct {
		convID string
		msgs   []data.Message
		err    error
	}
	olderMsg struct {
		convID string
		msgs   []data.Message
		err    error
	}
	repliesMsg struct {
		convID, rootID string
		replies        []data.Reply
		err            error
	}
	// sentMsg / sentReplyMsg reconcile an optimistic send with the backend.
	sentMsg struct {
		convID, pendingID, text string
		msg                     data.Message
		err                     error
	}
	sentReplyMsg struct {
		convID, rootID, pendingID, text string
		reply                           data.Reply
		err                             error
	}
	// clearErrMsg removes the error banner if no newer error replaced it.
	clearErrMsg struct{ seq int }
)

func pollTick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollMsg{} })
}

// historyCmd fetches a conversation's messages off the UI thread.
func (m Model) historyCmd(convID string) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		msgs, err := src.History(convID)
		return historyMsg{convID, msgs, err}
	}
}

// repliesCmd fetches a thread's replies off the UI thread.
func (m Model) repliesCmd(convID, rootID string) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		reps, err := src.Replies(convID, rootID)
		return repliesMsg{convID, rootID, reps, err}
	}
}

// refresh re-fetches the active channel and the open thread.
func (m Model) refresh() tea.Cmd {
	cmds := []tea.Cmd{m.historyCmd(m.activeID)}
	if m.threadOpen() {
		cmds = append(cmds, m.repliesCmd(m.activeID, m.threadRootID))
	}
	return tea.Batch(cmds...)
}

// applyHistory replaces a conversation's cached messages, carrying over any
// already-loaded thread replies and preserving the selection (following the
// bottom if the user was already there). It returns follow-up cmds: bots and
// agents commonly answer in a thread under your message — when a poll shows a
// new reply on a thread you started, fetch it (so the inline preview renders)
// and ring the bell, otherwise the answer is just a silent "└ 1 reply" line.
func (m *Model) applyHistory(convID string, msgs []data.Message) []tea.Cmd {
	old := m.messages[convID]
	loaded := map[string][]data.Reply{}
	prevCount := map[string]int{}
	for _, o := range old {
		if len(o.Replies) > 0 {
			loaded[o.ID] = o.Replies
		}
		prevCount[o.ID] = o.ReplyCount
	}
	var cmds []tea.Cmd
	rung := false
	for _, msg := range msgs {
		prev, known := prevCount[msg.ID]
		if known && msg.ReplyCount > prev && msg.UserID == m.ws.MeID {
			cmds = append(cmds, m.repliesCmd(convID, msg.ID))
			if !rung && msg.ID != m.threadRootID { // an open thread is already in view
				cmds = append(cmds, bellCmd)
				rung = true
			}
		}
	}
	for i := range msgs {
		if r, ok := loaded[msgs[i].ID]; ok {
			msgs[i].Replies = r
		}
	}
	// Carry over optimistic sends the backend hasn't acked yet — a poll
	// snapshot taken between send and ack doesn't contain them.
	for _, o := range old {
		if strings.HasPrefix(o.ID, pendingPrefix) {
			msgs = append(msgs, o)
		}
	}

	if convID != m.activeID {
		m.messages[convID] = msgs
		return cmds
	}
	atBottom := len(old) == 0 || m.msgSel >= len(old)-1
	selID := ""
	if m.msgSel >= 0 && m.msgSel < len(old) {
		selID = old[m.msgSel].ID
	}
	m.messages[convID] = msgs
	if atBottom {
		m.msgSel = max(0, len(msgs)-1)
		m.msgExtra = 0
	} else {
		m.msgSel = indexOfMsg(msgs, selID)
	}
	return cmds
}

// loadOlderCmd fetches the page of history before the oldest cached message.
func (m Model) loadOlderCmd(convID string) tea.Cmd {
	msgs := m.messages[convID]
	if len(msgs) == 0 {
		return nil
	}
	oldest := msgs[0].ID
	src := m.src
	return func() tea.Msg {
		older, err := src.HistoryBefore(convID, oldest)
		return olderMsg{convID, older, err}
	}
}

// prependHistory prepends an older page, keeping the selection on the same
// message; an empty page marks the conversation fully loaded.
func (m *Model) prependHistory(convID string, older []data.Message) {
	if len(older) == 0 {
		m.fullyLoaded[convID] = true
		return
	}
	m.messages[convID] = append(older, m.messages[convID]...)
	if convID == m.activeID {
		m.msgSel += len(older)
	}
}

// applyReplies updates a thread root's replies in the cache.
func (m *Model) applyReplies(convID, rootID string, replies []data.Reply) {
	list := m.messages[convID]
	for i := range list {
		if list[i].ID == rootID {
			list[i].Replies = replies
			list[i].ReplyCount = len(replies)
		}
	}
	m.messages[convID] = list
	if m.threadRootID == rootID {
		m.threadSel = clamp(m.threadSel, 0, max(0, len(replies)-1))
		m.seenReplies[convID+"|"+rootID] = len(replies) // viewing it clears the inbox "new" badge
	}
}

func indexOfMsg(msgs []data.Message, id string) int {
	for i := range msgs {
		if msgs[i].ID == id {
			return i
		}
	}
	return max(0, len(msgs)-1)
}
