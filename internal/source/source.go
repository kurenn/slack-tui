// Package source abstracts where workspace data comes from. The app talks to a
// Source; today that's a local mock, and the Slack implementation drops in
// behind the same interface. Network-backed methods are invoked from tea.Cmds.
package source

import "github.com/kurenn/slack-tui/internal/data"

// Source provides workspace data and message operations.
type Source interface {
	// Load fetches identity, users, channels and DMs (metadata, no history).
	Load() (*data.Workspace, error)
	// History fetches recent messages for a conversation (thread replies are
	// lazy — only ReplyCount is set; call Replies to load them).
	History(convID string) ([]data.Message, error)
	// HistoryBefore fetches the page of messages older than beforeTS (oldest
	// first); empty means there's no more history.
	HistoryBefore(convID, beforeTS string) ([]data.Message, error)
	// Replies fetches the replies of a thread root.
	Replies(convID, rootID string) ([]data.Reply, error)
	// Send posts a message to a conversation and returns the stored message.
	Send(convID, text string) (data.Message, error)
	// SendReply posts a threaded reply under rootID.
	SendReply(convID, rootID, text string) (data.Reply, error)
	// Unread returns the conversation's unread message count.
	Unread(convID string) (int, error)
	// MarkRead marks the conversation read up to ts (persists to the backend).
	MarkRead(convID, ts string) error
	// SetPresence sets the user's presence (online | away | dnd) on the backend.
	SetPresence(status string) error
	// SetStatusText sets the custom status message (text + optional :emoji:).
	SetStatusText(text, emoji string) error
	// Presence fetches current presence for the given users (DM partners),
	// returning id → status ("online" | "away"). Best-effort: ids that error
	// are omitted.
	Presence(userIDs []string) (map[string]string, error)
	// React toggles an emoji reaction (by Slack name, e.g. "thumbsup") on a
	// message. Returns whether the reaction was added (false = removed).
	React(convID, msgID, name string) (added bool, err error)
	// Edit replaces a message's text (own messages only).
	Edit(convID, msgID, text string) error
	// Delete removes a message (own messages only).
	Delete(convID, msgID string) error
	// Search runs a workspace-wide message search.
	Search(query string) ([]SearchHit, error)
	// Joinable lists public channels the user is not a member of.
	Joinable() ([]Conversation, error)
	// Join joins a channel and returns it; the caller adds it to the sidebar.
	Join(convID string) (Conversation, error)
}

// Conversation aliases the domain type for the interface signatures above.
type Conversation = data.Conversation

// SearchHit is one workspace search result.
type SearchHit struct {
	ConvID   string
	ConvName string
	UserName string
	Time     string
	MsgID    string
	Text     string
}

// tokenColors is the stable palette of syntax-token color keys assigned to users
// (Slack has no per-user color, so we hash the user id into one of these).
var tokenColors = []string{"blue", "green", "purple", "orange", "cyan", "red", "yellow"}

// ColorFor deterministically maps a user id to a token color key.
func ColorFor(id string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(id); i++ {
		h ^= uint32(id[i])
		h *= 16777619
	}
	return tokenColors[h%uint32(len(tokenColors))]
}
