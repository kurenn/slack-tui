package source

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/slack-go/slack"

	"github.com/kurenn/slack-tui/internal/data"
)

// IsRateLimited reports whether err is a Slack rate-limit error — callers in
// polling loops use it to back off instead of surfacing a scary banner.
func IsRateLimited(err error) bool {
	var rl *slack.RateLimitedError
	return errors.As(err, &rl)
}

// Slack is a Source backed by the Slack Web API, authenticated with a user
// token (xoxp). Network calls here run inside tea.Cmds.
type Slack struct {
	api       *slack.Client
	meID      string
	users     map[string]data.User // resolved lazily, seeded by Load
	handleIDs map[string]string    // lowercased @handle → user ID (outgoing mentions)
	events    chan Event           // Socket Mode stream (nil until StartSocket)
	groupDMs  bool                 // include mpims in Load
}

// NewSlack builds a Slack source from a user OAuth token (xoxp-…).
func NewSlack(userToken string) *Slack {
	return &Slack{api: slack.New(userToken), users: map[string]data.User{}, handleIDs: map[string]string{}}
}

// SetGroupDMs toggles whether Load includes group DMs (mpims). Takes effect on
// the next Load.
func (s *Slack) SetGroupDMs(on bool) { s.groupDMs = on }

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
		users[u.ID] = toUser(u)
	}
	s.users = users
	me := users[s.meID]
	me.Name, me.Handle = "you", users[s.meID].Handle

	// Real channels (the user is a member of) and 1:1 DMs; group DMs (mpims)
	// join the DM section when the Group DMs preference is on.
	types := []string{"public_channel", "private_channel", "im"}
	if s.groupDMs {
		types = append(types, "mpim")
	}
	var channels, dms []data.Conversation
	cursor := ""
	for {
		convs, next, err := s.api.GetConversations(&slack.GetConversationsParameters{
			Types: types, ExcludeArchived: true, Limit: 200, Cursor: cursor,
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
				dms = append(dms, data.Conversation{ID: c.ID, Type: "dm", UserID: c.User})
			case c.IsMpIM:
				dms = append(dms, data.Conversation{ID: c.ID, Type: "dm", Name: mpimName(c.Name)})
			case c.IsMember:
				channels = append(channels, data.Conversation{ID: c.ID, Type: "channel", Name: c.Name, Topic: c.Topic.Value})
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	s.resolveDMNames(dms, users) // users.list omits some (deactivated/external) — fetch them
	for i := range dms {
		if dms[i].UserID == "" { // mpim — name already set
			continue
		}
		if u, ok := users[dms[i].UserID]; ok && u.Name != "" {
			dms[i].Name = u.Name
		} else {
			dms[i].Name = dms[i].UserID
		}
	}
	sort.Slice(channels, func(i, j int) bool { return channels[i].Name < channels[j].Name })
	sort.Slice(dms, func(i, j int) bool { return dms[i].Name < dms[j].Name })
	s.fillUnread(channels)
	s.fillUnread(dms)

	// Index handles for outgoing-mention encoding (@handle → <@ID>).
	s.handleIDs = make(map[string]string, len(users))
	for id, u := range users {
		if u.Handle != "" {
			s.handleIDs[strings.ToLower(u.Handle)] = id
		}
	}

	ws := &data.Workspace{
		Name: auth.Team, Handle: slugify(auth.Team), MeID: s.meID,
		Users: users, Channels: channels, DMs: dms, Messages: map[string][]data.Message{},
	}
	ws.Users[s.meID] = me
	return ws, nil
}

// toUser maps a Slack user to ours, preferring display/real name.
func toUser(u slack.User) data.User {
	name := u.Name
	if u.Profile.RealName != "" {
		name = u.Profile.RealName
	}
	if u.Profile.DisplayName != "" {
		name = u.Profile.DisplayName
	}
	status := "online"
	if u.Deleted || u.IsBot {
		status = "offline"
	}
	return data.User{ID: u.ID, Name: name, Handle: u.Name, Color: ColorFor(u.ID), Status: status}
}

// resolveDMNames fetches user info for DM partners missing from the bulk
// users.list (deactivated or external users), concurrently and bounded.
func (s *Slack) resolveDMNames(dms []data.Conversation, users map[string]data.User) {
	var missing []string
	for _, d := range dms {
		if d.UserID == "" { // mpim — no single counterpart
			continue
		}
		if _, ok := users[d.UserID]; !ok {
			missing = append(missing, d.UserID)
		}
	}
	if len(missing) == 0 {
		return
	}
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, id := range missing {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			u, err := s.api.GetUserInfo(id)
			if err != nil {
				return
			}
			mu.Lock()
			users[id] = toUser(*u)
			mu.Unlock()
		}(id)
	}
	wg.Wait()
}

// Unread returns a conversation's unread message count.
func (s *Slack) Unread(convID string) (int, error) {
	ci, err := s.api.GetConversationInfo(&slack.GetConversationInfoInput{ChannelID: convID})
	if err != nil {
		return 0, err
	}
	return ci.UnreadCountDisplay, nil
}

// MarkRead marks a conversation read up to ts on Slack.
func (s *Slack) MarkRead(convID, ts string) error {
	if ts == "" {
		return nil
	}
	return s.api.MarkConversation(convID, ts)
}

// SetPresence updates presence/DND on Slack. Active/Away map to users.setPresence
// (auto/away); DND snoozes notifications. Slack only allows "auto" or "away" for
// presence — "active" is "auto".
func (s *Slack) SetPresence(status string) error {
	switch status {
	case "away":
		_, _ = s.api.EndSnooze()
		return s.api.SetUserPresence("away")
	case "dnd":
		_, err := s.api.SetSnooze(120) // 2h Do Not Disturb
		return err
	default: // online / active
		_, _ = s.api.EndSnooze()
		return s.api.SetUserPresence("auto")
	}
}

// SetStatusText sets the custom status message (text + optional :emoji:).
func (s *Slack) SetStatusText(text, emoji string) error {
	return s.api.SetUserCustomStatus(text, emoji, 0)
}

// fillUnread populates each conversation's unread count via conversations.info,
// concurrently and bounded, with an overall timeout so startup can't hang. A
// rate-limit error stops the remaining lookups (they'd all fail the same way).
func (s *Slack) fillUnread(convs []data.Conversation) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	var limited atomic.Bool
	for i := range convs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if limited.Load() {
				return
			}
			ci, err := s.api.GetConversationInfoContext(ctx, &slack.GetConversationInfoInput{ChannelID: convs[i].ID})
			if err != nil {
				if IsRateLimited(err) {
					limited.Store(true)
				}
				return
			}
			if ci.UnreadCountDisplay > 0 {
				convs[i].Unread = ci.UnreadCountDisplay
			}
		}(i)
	}
	wg.Wait()
}

// History fetches recent messages (newest last), resolving threads.
func (s *Slack) History(convID string) ([]data.Message, error) {
	resp, err := s.api.GetConversationHistory(&slack.GetConversationHistoryParameters{ChannelID: convID, Limit: 50})
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	var out []data.Message
	for i := len(resp.Messages) - 1; i >= 0; i-- { // API returns newest first
		out = append(out, s.toMessage(resp.Messages[i]))
	}
	return out, nil
}

// HistoryBefore fetches the page of messages older than beforeTS.
func (s *Slack) HistoryBefore(convID, beforeTS string) ([]data.Message, error) {
	resp, err := s.api.GetConversationHistory(&slack.GetConversationHistoryParameters{
		ChannelID: convID, Limit: 50, Latest: beforeTS, Inclusive: false,
	})
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	var out []data.Message
	for i := len(resp.Messages) - 1; i >= 0; i-- { // API returns newest first
		out = append(out, s.toMessage(resp.Messages[i]))
	}
	return out, nil
}

func (s *Slack) Send(convID, text string) (data.Message, error) {
	_, ts, err := s.api.PostMessage(convID, slack.MsgOptionText(s.encodeMentions(text), false))
	if err != nil {
		return data.Message{}, err
	}
	return data.Message{ID: ts, UserID: s.meID, Time: hm(), Text: text}, nil
}

func (s *Slack) SendReply(convID, rootID, text string) (data.Reply, error) {
	_, ts, err := s.api.PostMessage(convID, slack.MsgOptionText(s.encodeMentions(text), false), slack.MsgOptionTS(rootID))
	if err != nil {
		return data.Reply{}, err
	}
	return data.Reply{ID: ts, UserID: s.meID, Time: hm(), Text: text}, nil
}

var mentionRe = regexp.MustCompile(`(^|\W)@([A-Za-z0-9][A-Za-z0-9._-]*)`)

// encodeMentions converts "@handle" into Slack's <@UID> wire form (and
// @here/@channel/@everyone into <!…>) so outgoing mentions actually notify.
// Unknown handles are left as typed.
func (s *Slack) encodeMentions(text string) string {
	return mentionRe.ReplaceAllStringFunc(text, func(tok string) string {
		g := mentionRe.FindStringSubmatch(tok)
		prefix, handle := g[1], g[2]
		switch strings.ToLower(handle) {
		case "here":
			return prefix + "<!here>"
		case "channel":
			return prefix + "<!channel>"
		case "everyone":
			return prefix + "<!everyone>"
		}
		if id, ok := s.handleIDs[strings.ToLower(handle)]; ok {
			return prefix + "<@" + id + ">"
		}
		// "@ada." at a sentence end: retry without trailing punctuation.
		trimmed := strings.TrimRight(handle, "._-")
		if trimmed != handle {
			if id, ok := s.handleIDs[strings.ToLower(trimmed)]; ok {
				return prefix + "<@" + id + ">" + handle[len(trimmed):]
			}
		}
		return tok
	})
}

// ── mapping helpers ──────────────────────────────────────────────────────────

// Replies fetches a thread's replies, following pagination so long threads
// aren't silently truncated (lazy — called when a thread is opened or polled).
func (s *Slack) Replies(convID, rootID string) ([]data.Reply, error) {
	params := &slack.GetConversationRepliesParameters{ChannelID: convID, Timestamp: rootID, Limit: 200}
	var out []data.Reply
	for {
		reps, hasMore, next, err := s.api.GetConversationReplies(params)
		if err != nil {
			return nil, err
		}
		for _, r := range reps {
			if r.Timestamp == rootID { // skip the root itself
				continue
			}
			out = append(out, data.Reply{ID: r.Timestamp, UserID: messageAuthor(r), Time: tsTime(r.Timestamp), Text: s.messageText(r)})
		}
		if !hasMore || next == "" {
			return out, nil
		}
		params.Cursor = next
	}
}

// mentionsMe reports whether raw mrkdwn text pings the current user, including
// the broadcast forms (@here/@channel/@everyone).
func (s *Slack) mentionsMe(text string) bool {
	return strings.Contains(text, "<@"+s.meID+">") ||
		strings.Contains(text, "<!here") ||
		strings.Contains(text, "<!channel") ||
		strings.Contains(text, "<!everyone")
}

// messageText renders a message's body, falling back to its Block Kit blocks —
// bots commonly send blocks with an empty top-level text, which used to show
// up as a silent blank message ("the bot isn't responding").
func (s *Slack) messageText(m slack.Message) string {
	text := s.renderText(m.Text)
	if strings.TrimSpace(text) != "" {
		return text
	}
	var lines []string
	for _, ln := range blocksText(m.Blocks) {
		lines = append(lines, s.renderText(ln))
	}
	return strings.Join(lines, "\n")
}

// messageAuthor resolves who to display: bot_message-style posts carry no
// user id, only a bot identity.
func messageAuthor(m slack.Message) string {
	if m.User != "" {
		return m.User
	}
	switch {
	case m.Username != "":
		return m.Username
	case m.BotProfile != nil && m.BotProfile.Name != "":
		return m.BotProfile.Name
	case m.BotID != "":
		return "bot"
	}
	return m.User
}

// blocksText flattens Block Kit blocks into mrkdwn lines. Interactive elements
// can't be driven from a TUI, so buttons render as labeled placeholders.
func blocksText(blocks slack.Blocks) []string {
	var out []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	for _, b := range blocks.BlockSet {
		switch blk := b.(type) {
		case *slack.SectionBlock:
			if blk.Text != nil {
				add(blk.Text.Text)
			}
			for _, f := range blk.Fields {
				add(f.Text)
			}
		case *slack.HeaderBlock:
			if blk.Text != nil {
				add(blk.Text.Text)
			}
		case *slack.ContextBlock:
			var parts []string
			for _, el := range blk.ContextElements.Elements {
				if t, ok := el.(*slack.TextBlockObject); ok {
					parts = append(parts, t.Text)
				}
			}
			add(strings.Join(parts, " · "))
		case *slack.ActionBlock:
			if blk.Elements == nil {
				continue
			}
			var parts []string
			for _, el := range blk.Elements.ElementSet {
				if btn, ok := el.(*slack.ButtonBlockElement); ok && btn.Text != nil {
					parts = append(parts, "[button: "+btn.Text.Text+"]")
				}
			}
			add(strings.Join(parts, " "))
		case *slack.DividerBlock:
			add("———")
		case *slack.ImageBlock:
			add("[image: " + blk.AltText + "]")
		case *slack.RichTextBlock:
			add(richTextText(blk))
		}
	}
	return out
}

// richTextText flattens a rich_text block's sections into plain mrkdwn.
func richTextText(rtb *slack.RichTextBlock) string {
	var b strings.Builder
	for _, el := range rtb.Elements {
		sec, ok := el.(*slack.RichTextSection)
		if !ok {
			continue
		}
		for _, e := range sec.Elements {
			switch t := e.(type) {
			case *slack.RichTextSectionTextElement:
				b.WriteString(t.Text)
			case *slack.RichTextSectionLinkElement:
				if t.Text != "" && t.Text != t.URL {
					b.WriteString(t.Text + " ")
				}
				b.WriteString(t.URL)
			case *slack.RichTextSectionUserElement:
				b.WriteString("<@" + t.UserID + ">")
			case *slack.RichTextSectionChannelElement:
				b.WriteString("<#" + t.ChannelID + ">")
			case *slack.RichTextSectionEmojiElement:
				b.WriteString(":" + t.Name + ":")
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (s *Slack) toMessage(m slack.Message) data.Message {
	msg := data.Message{
		ID: m.Timestamp, UserID: messageAuthor(m), Time: tsTime(m.Timestamp), Day: tsDay(m.Timestamp),
		Text: s.messageText(m), ReplyCount: m.ReplyCount, MentionsMe: s.mentionsMe(m.Text),
	}
	// Uploads and attachments land in Extra (annotations rendered after the
	// body) and Links (permalinks for the `o` open action).
	for _, f := range m.Files {
		name := f.Name
		if name == "" {
			name = f.Title
		}
		if name == "" {
			name = "file"
		}
		msg.Extra = append(msg.Extra, "[file: "+name+"]")
		if f.Permalink != "" {
			msg.Links = append(msg.Links, f.Permalink)
		}
	}
	for _, a := range m.Attachments {
		label := a.Title
		if label == "" {
			label = a.Fallback
		}
		if label != "" {
			msg.Extra = append(msg.Extra, "[attachment: "+label+"]")
		}
		if a.TitleLink != "" {
			msg.Links = append(msg.Links, a.TitleLink)
		}
	}
	for _, r := range m.Reactions {
		msg.Reactions = append(msg.Reactions, data.Reaction{Emoji: emojiOf(r.Name), Count: r.Count})
	}
	return msg
}

// React toggles a reaction: add, and if Slack says we already reacted, remove.
func (s *Slack) React(convID, msgID, name string) (bool, error) {
	ref := slack.NewRefToMessage(convID, msgID)
	err := s.api.AddReaction(name, ref)
	if err != nil && err.Error() == "already_reacted" {
		if err := s.api.RemoveReaction(name, ref); err != nil {
			return false, err
		}
		return false, nil
	}
	return err == nil, err
}

// Edit replaces a message's text via chat.update.
func (s *Slack) Edit(convID, msgID, text string) error {
	_, _, _, err := s.api.UpdateMessage(convID, msgID, slack.MsgOptionText(s.encodeMentions(text), false))
	return err
}

// Joinable lists public channels the user is not a member of.
func (s *Slack) Joinable() ([]data.Conversation, error) {
	var out []data.Conversation
	cursor := ""
	for {
		convs, next, err := s.api.GetConversations(&slack.GetConversationsParameters{
			Types: []string{"public_channel"}, ExcludeArchived: true, Limit: 200, Cursor: cursor,
		})
		if err != nil {
			return nil, err
		}
		for _, c := range convs {
			if !c.IsMember {
				out = append(out, data.Conversation{ID: c.ID, Type: "channel", Name: c.Name, Topic: c.Topic.Value})
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Join joins a public channel (needs the channels:join scope).
func (s *Slack) Join(convID string) (data.Conversation, error) {
	ch, _, _, err := s.api.JoinConversation(convID)
	if err != nil {
		return data.Conversation{}, err
	}
	return data.Conversation{ID: ch.ID, Type: "channel", Name: ch.Name, Topic: ch.Topic.Value}, nil
}

// Delete removes a message via chat.delete.
func (s *Slack) Delete(convID, msgID string) error {
	_, _, err := s.api.DeleteMessage(convID, msgID)
	return err
}

// Search runs search.messages (needs the search:read user scope).
func (s *Slack) Search(query string) ([]SearchHit, error) {
	params := slack.NewSearchParameters()
	params.Count = 30
	res, err := s.api.SearchMessages(query, params)
	if err != nil {
		return nil, err
	}
	var out []SearchHit
	for _, hit := range res.Matches {
		name := hit.Username
		if u, ok := s.users[hit.User]; ok {
			name = u.Name
		}
		out = append(out, SearchHit{
			ConvID: hit.Channel.ID, ConvName: hit.Channel.Name, UserName: name,
			Time: tsDay(hit.Timestamp) + " " + tsTime(hit.Timestamp),
			MsgID: hit.Timestamp, Text: s.renderText(hit.Text),
		})
	}
	return out, nil
}

// mpimName prettifies "mpdm-ada--lin--marco-1" into "ada, lin, marco".
func mpimName(raw string) string {
	name := strings.TrimPrefix(raw, "mpdm-")
	if i := strings.LastIndex(name, "-"); i > 0 {
		name = name[:i]
	}
	parts := strings.Split(name, "--")
	return strings.Join(parts, ", ")
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

// tsParse converts a Slack ts ("1718000000.000200") into a local time.
func tsParse(ts string) (time.Time, bool) {
	sec := ts
	if i := strings.IndexByte(ts, '.'); i >= 0 {
		sec = ts[:i]
	}
	n, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(n, 0), true
}

func tsTime(ts string) string {
	t, ok := tsParse(ts)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
}

// tsDay is the date-separator label for a message ("Mon Jan 2").
func tsDay(ts string) string {
	t, ok := tsParse(ts)
	if !ok {
		return ""
	}
	return t.Format("Mon Jan 2")
}

func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}

// emojiOf maps common Slack reaction names to glyphs; falls back to :name:.
func emojiOf(name string) string {
	if e, ok := commonEmoji[name]; ok {
		return e
	}
	return ":" + name + ":"
}

// EmojiGlyph is emojiOf for other packages (the reaction picker).
func EmojiGlyph(name string) string { return emojiOf(name) }

// EmojiNames returns all known emoji names, sorted (composer autocomplete).
var EmojiNames = sync.OnceValue(func() []string {
	names := make([]string, 0, len(commonEmoji))
	for n := range commonEmoji {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
})

var commonEmoji = map[string]string{
	"fire": "🔥", "rocket": "🚀", "tada": "🎉", "eyes": "👀", "wave": "👋",
	"white_check_mark": "✅", "heavy_check_mark": "✔️", "pray": "🙏", "clap": "👏",
	"thumbsup": "👍", "+1": "👍", "thumbsdown": "👎", "-1": "👎", "heart": "❤️",
	"joy": "😂", "sweat_smile": "😅", "smile": "😄", "laughing": "😆", "wink": "😉",
	"thinking_face": "🤔", "sob": "😭", "cry": "😢", "scream": "😱", "grimacing": "😬",
	"raised_hands": "🙌", "ok_hand": "👌", "muscle": "💪", "point_up": "☝️", "v": "✌️",
	"100": "💯", "heavy_plus_sign": "➕", "question": "❓", "exclamation": "❗",
	"warning": "⚠️", "x": "❌", "no_entry": "⛔", "bulb": "💡", "zap": "⚡",
	"star": "⭐", "sparkles": "✨", "boom": "💥", "bug": "🐛", "wrench": "🔧",
	"hammer": "🔨", "lock": "🔒", "key": "🔑", "mag": "🔍", "memo": "📝",
	"book": "📖", "chart_with_upwards_trend": "📈", "calendar": "📅", "bell": "🔔",
	"coffee": "☕", "beer": "🍺", "pizza": "🍕", "cake": "🍰", "ship": "🚢",
	"shipit": "🐿️", "art": "🎨", "package": "📦", "label": "🏷️", "dart": "🎯",
	"checkered_flag": "🏁", "construction": "🚧", "recycle": "♻️", "seedling": "🌱",
	"salute": "🫡", "handshake": "🤝", "crossed_fingers": "🤞", "melting_face": "🫠",
	"skull": "💀", "ghost": "👻", "robot_face": "🤖", "brain": "🧠", "heart_eyes": "😍",
	"partying_face": "🥳", "smiling_face_with_3_hearts": "🥰", "face_palm": "🤦",
	"shrug": "🤷", "wave_dash": "〰️", "speech_balloon": "💬", "loudspeaker": "📢",
}

// ReactionChoices is the curated, ordered list shown by the reaction picker.
// Any Slack emoji name (including custom ones) can still be typed free-form.
var ReactionChoices = []string{
	"thumbsup", "heart", "joy", "tada", "fire", "rocket", "eyes", "pray", "clap",
	"white_check_mark", "wave", "thinking_face", "sob", "sweat_smile", "raised_hands",
	"100", "ok_hand", "muscle", "salute", "handshake", "partying_face", "heart_eyes",
	"scream", "skull", "shrug", "face_palm", "bulb", "warning", "x", "question",
	"memo", "bug", "ship", "shipit", "coffee", "pizza", "cake", "beer", "star",
	"sparkles", "boom", "zap", "dart", "checkered_flag", "construction", "seedling",
}
