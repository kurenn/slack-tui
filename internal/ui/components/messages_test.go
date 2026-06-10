package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/theme"
)

// TestDateSeparators: messages with Day labels get a rule per day; dateless
// (mock) messages keep the single "today" rule.
func TestDateSeparators(t *testing.T) {
	p := theme.Resolve("charcoal", "auto")
	ws := &data.Workspace{Users: map[string]data.User{"a": {ID: "a", Name: "ada", Color: "blue"}}}
	msgs := []data.Message{
		{ID: "1", UserID: "a", Time: "09:00", Day: "Mon Jun 8", Text: "one"},
		{ID: "2", UserID: "a", Time: "09:01", Day: "Tue Jun 9", Text: "two"},
	}
	lines, starts := MessagesBody(p, ws, msgs, 0, false, theme.ParseDensity("comfortable"), 60, "")
	joined := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "Mon Jun 8") || !strings.Contains(joined, "Tue Jun 9") {
		t.Errorf("expected one separator per day, got:\n%s", joined)
	}
	if len(starts) != len(msgs)+1 {
		t.Errorf("starts should have one entry per message + sentinel, got %d", len(starts))
	}

	dateless := []data.Message{{ID: "1", UserID: "a", Time: "09:00", Text: "one"}}
	lines, _ = MessagesBody(p, ws, dateless, 0, false, theme.ParseDensity("comfortable"), 60, "")
	joined = ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "today") {
		t.Errorf("dateless messages should fall back to the 'today' rule, got:\n%s", joined)
	}
}

// TestNewMessagesRule: the ── new ── rule renders above the marked message.
func TestNewMessagesRule(t *testing.T) {
	p := theme.Resolve("charcoal", "auto")
	ws := &data.Workspace{Users: map[string]data.User{"a": {ID: "a", Name: "ada", Color: "blue"}}}
	msgs := []data.Message{
		{ID: "1", UserID: "a", Time: "09:00", Text: "read"},
		{ID: "2", UserID: "a", Time: "09:01", Text: "unread"},
	}
	lines, starts := MessagesBody(p, ws, msgs, 0, false, theme.ParseDensity("comfortable"), 60, "2")
	joined := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(joined, " new ") {
		t.Fatalf("expected a new-messages rule, got:\n%s", joined)
	}
	ruleAt := -1
	for i, ln := range lines {
		if strings.Contains(ansi.Strip(ln), " new ") {
			ruleAt = i
		}
	}
	if ruleAt == -1 || ruleAt >= starts[1] {
		t.Errorf("the rule should sit above the marked message (rule=%d, msg2 starts=%d)", ruleAt, starts[1])
	}
}
