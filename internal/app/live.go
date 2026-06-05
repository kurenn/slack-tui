package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/abrahamkuri/slack-tui/internal/data"
	"github.com/abrahamkuri/slack-tui/internal/source"
)

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
// active channel and open thread are kept fresh by polling.
func (m *Model) handleEvent(ev source.Event) {
	if ev.ConvID == "" || ev.ConvID == m.activeID {
		return
	}
	if _, ok := m.ws.Conversation(ev.ConvID); !ok {
		return
	}
	if ev.Msg.UserID == m.ws.MeID {
		return // our own message from another client
	}
	meta := m.meta[ev.ConvID]
	meta.Unread++
	if ev.Msg.MentionsMe {
		meta.Mention = true
	}
	m.meta[ev.ConvID] = meta
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
	repliesMsg struct {
		convID, rootID string
		replies        []data.Reply
		err            error
	}
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
// bottom if the user was already there).
func (m *Model) applyHistory(convID string, msgs []data.Message) {
	old := m.messages[convID]
	loaded := map[string][]data.Reply{}
	for _, o := range old {
		if len(o.Replies) > 0 {
			loaded[o.ID] = o.Replies
		}
	}
	for i := range msgs {
		if r, ok := loaded[msgs[i].ID]; ok {
			msgs[i].Replies = r
		}
	}

	if convID != m.activeID {
		m.messages[convID] = msgs
		return
	}
	atBottom := len(old) == 0 || m.msgSel >= len(old)-1
	selID := ""
	if m.msgSel >= 0 && m.msgSel < len(old) {
		selID = old[m.msgSel].ID
	}
	m.messages[convID] = msgs
	if atBottom {
		m.msgSel = max(0, len(msgs)-1)
	} else {
		m.msgSel = indexOfMsg(msgs, selID)
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
