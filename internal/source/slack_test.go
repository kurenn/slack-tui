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

// TestToMessageMapsFiles: toMessage maps slack.File entries to data.File,
// preserving name and url_private_download.
func TestToMessageMapsFiles(t *testing.T) {
	s := &Slack{users: map[string]data.User{}}
	m := slack.Message{Msg: slack.Msg{
		Timestamp: "1718000000.000100",
		User:      "U1",
		Files: []slack.File{{
			ID:                 "F1",
			Name:               "a.pdf",
			URLPrivateDownload: "https://x/a",
			Size:               10,
		}},
	}}
	got := s.toMessage(m)
	if len(got.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(got.Files))
	}
	f := got.Files[0]
	if f.Name != "a.pdf" {
		t.Errorf("File.Name = %q, want a.pdf", f.Name)
	}
	if f.URL != "https://x/a" {
		t.Errorf("File.URL = %q, want https://x/a", f.URL)
	}
}

func TestColorForIsStable(t *testing.T) {
	if ColorFor("U123") != ColorFor("U123") {
		t.Error("ColorFor must be deterministic")
	}
}

// TestMockPresenceReturnsStatuses: Mock.Presence echoes back the workspace's
// own statuses for the requested ids — deterministic, no network.
func TestMockPresenceReturnsStatuses(t *testing.T) {
	m := NewMock()
	got, err := m.Presence([]string{"ada", "marco", "tomo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{
		"ada":   "online",
		"marco": "away",
		"tomo":  "dnd",
	}
	for id, wantStatus := range want {
		if got[id] != wantStatus {
			t.Errorf("Presence[%q] = %q, want %q", id, got[id], wantStatus)
		}
	}
}

// TestMockPresenceUnknownID: ids not in the workspace are silently omitted.
func TestMockPresenceUnknownID(t *testing.T) {
	m := NewMock()
	got, err := m.Presence([]string{"nobody"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got["nobody"]; ok {
		t.Error("unknown id should be omitted from the result")
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

// TestGemojiFallback: the generated map covers names the curated one doesn't.
func TestGemojiFallback(t *testing.T) {
	for name, want := range map[string]string{"avocado": "🥑", "face_with_monocle": "🧐", "shrimp": "🦐"} {
		if got := emojiOf(name); got != want {
			t.Errorf("emojiOf(%q) = %q, want %q", name, got, want)
		}
	}
	if got := emojiOf("custom-workspace-emoji"); got != ":custom-workspace-emoji:" {
		t.Errorf("unknown emoji should fall back to :name:, got %q", got)
	}
	if n := len(EmojiNames()); n < 1500 {
		t.Errorf("EmojiNames should include the full gemoji set, got %d", n)
	}
}

// TestRichTextCode: inline-code elements get backtick wrapping; preformatted
// blocks become fenced code blocks.
func TestRichTextCode(t *testing.T) {
	blk := slack.NewRichTextBlock("",
		slack.NewRichTextSection(
			slack.NewRichTextSectionTextElement("x", &slack.RichTextSectionTextStyle{Code: true}),
		),
		&slack.RichTextPreformatted{
			Type: "rich_text_preformatted",
			Elements: []slack.RichTextSectionElement{
				slack.NewRichTextSectionTextElement("line1\nline2", nil),
			},
		},
	)
	got := richTextText(blk)
	if !strings.Contains(got, "`x`") {
		t.Errorf("inline-code element should be wrapped in backticks, got %q", got)
	}
	if !strings.Contains(got, "```") {
		t.Errorf("preformatted block should produce a fenced code block, got %q", got)
	}
	if !strings.Contains(got, "line1\nline2") {
		t.Errorf("preformatted text should appear in output, got %q", got)
	}
}

func TestRenderTextEmojiShortcodes(t *testing.T) {
	s := &Slack{}
	cases := []struct{ in, want string }{
		{":warning: alert", emojiOf("warning") + " alert"},
		{"done :white_check_mark:", "done " + emojiOf("white_check_mark")},
		{"no :notarealemoji: here", "no :notarealemoji: here"},   // unknown stays literal
		{"window 12:30:00 – 13:59:59", "window 12:30:00 – 13:59:59"}, // times untouched
		{":tada::tada:", emojiOf("tada") + emojiOf("tada")},
	}
	for _, c := range cases {
		if got := s.renderText(c.in); got != c.want {
			t.Errorf("renderText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
