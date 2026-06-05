package source

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"

	"github.com/abrahamkuri/slack-tui/internal/data"
)

// Slack is a Source backed by the Slack Web API, authenticated with a user
// token (xoxp). Network calls here run inside tea.Cmds.
type Slack struct {
	api   *slack.Client
	meID  string
	users map[string]data.User // resolved lazily, seeded by Load
}

// NewSlack builds a Slack source from a user OAuth token (xoxp-…).
func NewSlack(userToken string) *Slack {
	return &Slack{api: slack.New(userToken), users: map[string]data.User{}}
}

// Load fetches identity, users, channels and DMs.
func (s *Slack) Load() (*data.Workspace, error) {
	auth, err := s.api.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	s.meID = auth.UserID

	users := map[string]data.User{}
	su, err := s.api.GetUsers()
	if err != nil {
		return nil, fmt.Errorf("users: %w", err)
	}
	for _, u := range su {
		name := u.Name
		if u.Profile.DisplayName != "" {
			name = u.Profile.DisplayName
		}
		status := "online"
		if u.Deleted || u.IsBot {
			status = "offline"
		}
		users[u.ID] = data.User{ID: u.ID, Name: name, Handle: u.Name, Color: ColorFor(u.ID), Status: status}
	}
	s.users = users
	me := users[s.meID]
	me.Name, me.Handle = "you", users[s.meID].Handle

	// Only real channels (the user is a member of) and 1:1 DMs. Group DMs
	// (mpim / mpdm-*) are excluded — they flood and clutter the list.
	var channels, dms []data.Conversation
	cursor := ""
	for {
		convs, next, err := s.api.GetConversations(&slack.GetConversationsParameters{
			Types: []string{"public_channel", "private_channel", "im"}, ExcludeArchived: true, Limit: 200, Cursor: cursor,
		})
		if err != nil {
			return nil, fmt.Errorf("conversations: %w", err)
		}
		for _, c := range convs {
			switch {
			case c.IsIM:
				if c.User == "" || c.IsUserDeleted {
					continue
				}
				dms = append(dms, data.Conversation{ID: c.ID, Type: "dm", Name: nameOf(users, c.User), UserID: c.User})
			case c.IsMember && !c.IsMpIM:
				channels = append(channels, data.Conversation{ID: c.ID, Type: "channel", Name: c.Name, Topic: c.Topic.Value})
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	sort.Slice(channels, func(i, j int) bool { return channels[i].Name < channels[j].Name })
	sort.Slice(dms, func(i, j int) bool { return dms[i].Name < dms[j].Name })

	ws := &data.Workspace{
		Name: auth.Team, Handle: slugify(auth.Team), MeID: s.meID,
		Users: users, Channels: channels, DMs: dms, Messages: map[string][]data.Message{},
	}
	ws.Users[s.meID] = me
	return ws, nil
}

// History fetches recent messages (newest last), resolving threads.
func (s *Slack) History(convID string) ([]data.Message, error) {
	resp, err := s.api.GetConversationHistory(&slack.GetConversationHistoryParameters{ChannelID: convID, Limit: 50})
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	var out []data.Message
	for i := len(resp.Messages) - 1; i >= 0; i-- { // API returns newest first
		out = append(out, s.toMessage(convID, resp.Messages[i]))
	}
	return out, nil
}

func (s *Slack) Send(convID, text string) (data.Message, error) {
	_, ts, err := s.api.PostMessage(convID, slack.MsgOptionText(text, false), slack.MsgOptionAsUser(true))
	if err != nil {
		return data.Message{}, err
	}
	return data.Message{ID: ts, UserID: s.meID, Time: hm(), Text: text}, nil
}

func (s *Slack) SendReply(convID, rootID, text string) (data.Reply, error) {
	_, ts, err := s.api.PostMessage(convID, slack.MsgOptionText(text, false), slack.MsgOptionTS(rootID), slack.MsgOptionAsUser(true))
	if err != nil {
		return data.Reply{}, err
	}
	return data.Reply{ID: ts, UserID: s.meID, Time: hm(), Text: text}, nil
}

// ── mapping helpers ──────────────────────────────────────────────────────────

func (s *Slack) toMessage(convID string, m slack.Message) data.Message {
	msg := data.Message{
		ID: m.Timestamp, UserID: m.User, Time: tsTime(m.Timestamp), Text: s.renderText(m.Text),
		MentionsMe: strings.Contains(m.Text, "<@"+s.meID+">"),
	}
	for _, r := range m.Reactions {
		msg.Reactions = append(msg.Reactions, data.Reaction{Emoji: emojiOf(r.Name), Count: r.Count})
	}
	if m.ReplyCount > 0 {
		if reps, _, _, err := s.api.GetConversationReplies(&slack.GetConversationRepliesParameters{ChannelID: convID, Timestamp: m.Timestamp}); err == nil {
			for _, r := range reps[1:] { // first is the root
				msg.Replies = append(msg.Replies, data.Reply{ID: r.Timestamp, UserID: r.User, Time: tsTime(r.Timestamp), Text: s.renderText(r.Text)})
			}
		}
	}
	return msg
}

var refRe = regexp.MustCompile(`<([@#])([A-Z0-9]+)(\|[^>]+)?>|<(https?://[^|>]+)(\|[^>]+)?>`)

// renderText converts Slack mrkdwn refs into our @name / #name / url forms.
func (s *Slack) renderText(text string) string {
	text = refRe.ReplaceAllStringFunc(text, func(m string) string {
		g := refRe.FindStringSubmatch(m)
		switch {
		case g[1] == "@":
			return "@" + nameOf(s.users, g[2])
		case g[1] == "#":
			if g[3] != "" {
				return "#" + strings.TrimPrefix(g[3], "|")
			}
			return "#" + g[2]
		default:
			return g[4]
		}
	})
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(text)
}

func nameOf(users map[string]data.User, id string) string {
	if u, ok := users[id]; ok {
		return u.Handle
	}
	return id
}

func tsTime(ts string) string {
	sec := ts
	if i := strings.IndexByte(ts, '.'); i >= 0 {
		sec = ts[:i]
	}
	n, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return ""
	}
	t := time.Unix(n, 0)
	return fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
}

func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}

// emojiOf maps a few common Slack reaction names to glyphs; falls back to :name:.
func emojiOf(name string) string {
	if e, ok := commonEmoji[name]; ok {
		return e
	}
	return ":" + name + ":"
}

var commonEmoji = map[string]string{
	"fire": "🔥", "rocket": "🚀", "tada": "🎉", "eyes": "👀", "wave": "👋",
	"white_check_mark": "✅", "heavy_check_mark": "✔️", "pray": "🙏", "clap": "👏",
	"thumbsup": "👍", "+1": "👍", "heart": "❤️", "joy": "😂", "sweat_smile": "😅",
	"coffee": "☕", "art": "🎨", "package": "📦", "label": "🏷️",
}
