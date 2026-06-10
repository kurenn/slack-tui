package source

import (
	"regexp"
	"strings"
	"testing"

	"github.com/slack-go/slack"

	"github.com/kurenn/slack-tui/internal/data"
)

func TestRenderText(t *testing.T) {
	s := &Slack{users: map[string]data.User{"U123": {ID: "U123", Handle: "ada"}}}
	cases := []struct{ in, want string }{
		{"<@U123> ping", "@ada ping"},
		{"<@UNKNOWN> ping", "@UNKNOWN ping"},
		{"see <#C42|general>", "see #general"},
		{"see <#C42>", "see #C42"},
		{"go to <https://example.com|the site>", "go to https://example.com"},
		{"go to <https://example.com>", "go to https://example.com"},
		{"a &amp; b &lt;c&gt;", "a & b <c>"},
	}
	for _, c := range cases {
		if got := s.renderText(c.in); got != c.want {
			t.Errorf("renderText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMentionsMe(t *testing.T) {
	s := &Slack{meID: "U1"}
	for _, txt := range []string{"<@U1> hi", "yo <!here|@here>", "<!channel> all", "<!everyone>"} {
		if !s.mentionsMe(txt) {
			t.Errorf("mentionsMe(%q) should be true", txt)
		}
	}
	for _, txt := range []string{"plain text", "<@U2> someone else", "@you literal"} {
		if s.mentionsMe(txt) {
			t.Errorf("mentionsMe(%q) should be false", txt)
		}
	}
}

func TestTsTimeAndDay(t *testing.T) {
	if got := tsTime("1718000000.000200"); !regexp.MustCompile(`^\d{2}:\d{2}$`).MatchString(got) {
		t.Errorf("tsTime = %q, want HH:MM", got)
	}
	if got := tsDay("1718000000.000200"); !regexp.MustCompile(`^[A-Z][a-z]{2} [A-Z][a-z]{2} \d{1,2}$`).MatchString(got) {
		t.Errorf("tsDay = %q, want like 'Mon Jun 10'", got)
	}
	if tsTime("garbage") != "" || tsDay("garbage") != "" {
		t.Error("unparseable ts should yield empty time/day")
	}
}

func TestEncodeMentions(t *testing.T) {
	s := &Slack{handleIDs: map[string]string{"ada": "U123", "lin.z": "U456"}}
	cases := []struct{ in, want string }{
		{"@ada ping", "<@U123> ping"},
		{"hey @ada, look", "hey <@U123>, look"},
		{"@ada. done", "<@U123>. done"},
		{"@lin.z hi", "<@U456> hi"},
		{"@here heads up", "<!here> heads up"},
		{"@channel ship it", "<!channel> ship it"},
		{"@unknown stays", "@unknown stays"},
		{"email a@b.com untouched", "email a@b.com untouched"},
	}
	for _, c := range cases {
		if got := s.encodeMentions(c.in); got != c.want {
			t.Errorf("encodeMentions(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBlockKitFallback: bot replies are often blocks with an empty top-level
// text — they must render, not show up as silent blank messages.
func TestBlockKitFallback(t *testing.T) {
	s := &Slack{users: map[string]data.User{}}
	m := slack.Message{Msg: slack.Msg{
		Timestamp: "1718000000.000100",
		BotID:     "B123",
		Username:  "deploybot",
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", "Deploy finished", false, false)),
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", "build *#42* is live", false, false), nil, nil),
			slack.NewContextBlock("", slack.NewTextBlockObject("mrkdwn", "took 41s", false, false)),
			slack.NewActionBlock("", slack.NewButtonBlockElement("", "v",
				slack.NewTextBlockObject("plain_text", "Rollback", false, false))),
		}},
	}}
	got := s.toMessage(m)
	for _, want := range []string{"Deploy finished", "build *#42* is live", "took 41s", "[button: Rollback]"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("blocks fallback missing %q in %q", want, got.Text)
		}
	}
	if got.UserID != "deploybot" {
		t.Errorf("bot author = %q, want deploybot", got.UserID)
	}
}

// TestRichTextFallback: rich_text blocks flatten to readable text.
func TestRichTextFallback(t *testing.T) {
	s := &Slack{users: map[string]data.User{"U1": {ID: "U1", Handle: "ada"}}}
	m := slack.Message{Msg: slack.Msg{
		Timestamp: "1718000000.000100",
		User:      "U2",
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			slack.NewRichTextBlock("",
				slack.NewRichTextSection(
					slack.NewRichTextSectionTextElement("hey ", nil),
					slack.NewRichTextSectionUserElement("U1", nil),
					slack.NewRichTextSectionTextElement(" see ", nil),
					slack.NewRichTextSectionLinkElement("https://x.dev", "", nil),
				)),
		}},
	}}
	got := s.toMessage(m)
	if got.Text != "hey @ada see https://x.dev" {
		t.Errorf("rich text fallback = %q", got.Text)
	}
}

func TestMpimName(t *testing.T) {
	if got := mpimName("mpdm-ada--lin--marco-1"); got != "ada, lin, marco" {
		t.Errorf("mpimName = %q, want 'ada, lin, marco'", got)
	}
}

func TestColorForIsStable(t *testing.T) {
	if ColorFor("U123") != ColorFor("U123") {
		t.Error("ColorFor must be deterministic")
	}
}

func TestEmojiOf(t *testing.T) {
	if emojiOf("fire") != "🔥" {
		t.Error("known reaction should map to a glyph")
	}
	if emojiOf("obscure_emoji") != ":obscure_emoji:" {
		t.Error("unknown reaction should fall back to :name:")
	}
}
