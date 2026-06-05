package source

import (
	"fmt"
	"strings"
	"time"

	"github.com/abrahamkuri/slack-tui/internal/data"
)

// Mock is an in-memory Source backed by the sample workspace. It returns
// instantly, so the app behaves synchronously when using it.
type Mock struct {
	ws       *data.Workspace
	messages map[string][]data.Message
}

// NewMock builds a mock source from the sample workspace.
func NewMock() *Mock {
	ws := data.Mock()
	msgs := make(map[string][]data.Message, len(ws.Messages))
	for k, v := range ws.Messages {
		msgs[k] = append([]data.Message(nil), v...)
	}
	return &Mock{ws: ws, messages: msgs}
}

func (m *Mock) Load() (*data.Workspace, error) { return m.ws, nil }

func (m *Mock) History(convID string) ([]data.Message, error) {
	out := append([]data.Message(nil), m.messages[convID]...)
	for i := range out {
		out[i].ReplyCount = len(out[i].Replies)
	}
	return out, nil
}

func (m *Mock) Replies(convID, rootID string) ([]data.Reply, error) {
	for _, msg := range m.messages[convID] {
		if msg.ID == rootID {
			return append([]data.Reply(nil), msg.Replies...), nil
		}
	}
	return nil, nil
}

func (m *Mock) Send(convID, text string) (data.Message, error) {
	msg := data.Message{ID: "m" + stamp(), UserID: m.ws.MeID, Time: hm(), Text: text}
	m.messages[convID] = append(m.messages[convID], msg)
	return msg, nil
}

func (m *Mock) SendReply(convID, rootID, text string) (data.Reply, error) {
	r := data.Reply{ID: "r" + stamp(), UserID: m.ws.MeID, Time: hm(), Text: text}
	for i := range m.messages[convID] {
		if m.messages[convID][i].ID == rootID {
			m.messages[convID][i].Replies = append(m.messages[convID][i].Replies, r)
		}
	}
	return r, nil
}

func (m *Mock) Unread(convID string) (int, error) { return 0, nil }
func (m *Mock) MarkRead(convID, ts string) error  { return nil }

func hm() string    { now := time.Now(); return fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute()) }
func stamp() string { return strings.TrimPrefix(fmt.Sprintf("%d", time.Now().UnixNano()), "1") }
