// Package source abstracts where workspace data comes from. The app talks to a
// Source; today that's a local mock, and the Slack implementation drops in
// behind the same interface. Network-backed methods are invoked from tea.Cmds.
package source

import "github.com/abrahamkuri/slack-tui/internal/data"

// Source provides workspace data and message operations.
type Source interface {
	// Load fetches identity, users, channels and DMs (metadata, no history).
	Load() (*data.Workspace, error)
	// History fetches recent messages for a conversation (thread replies are
	// lazy — only ReplyCount is set; call Replies to load them).
	History(convID string) ([]data.Message, error)
	// Replies fetches the replies of a thread root.
	Replies(convID, rootID string) ([]data.Reply, error)
	// Send posts a message to a conversation and returns the stored message.
	Send(convID, text string) (data.Message, error)
	// SendReply posts a threaded reply under rootID.
	SendReply(convID, rootID, text string) (data.Reply, error)
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
