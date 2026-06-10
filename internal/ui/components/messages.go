package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/kurenn/slack-tui/internal/data"
	"github.com/kurenn/slack-tui/internal/markup"
	"github.com/kurenn/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

const timeGutter = 6 // "09:21 "

// MessagesBody renders a conversation's messages to body lines plus the starting
// line index of each message (with a trailing sentinel = total line count), so
// the caller can scroll the selected message into view. The cursor/mention
// highlight only shows when focused.
func MessagesBody(p theme.Palette, ws *data.Workspace, msgs []data.Message, selIndex int, focused bool, density theme.Density, innerW int, newMarkID string) (lines []string, starts []int) {
	today := time.Now().Format("Mon Jan 2")
	if len(msgs) == 0 || msgs[0].Day == "" { // dateless data (mock): single "today" rule
		lines = append(lines, dayBreak(p, " today ", innerW), "")
	}
	gap := density.MsgGap() + 1 // blank lines between author groups
	prevUser, prevDay := "", ""
	for i, msg := range msgs {
		dayChanged := msg.Day != "" && msg.Day != prevDay
		if dayChanged {
			label := " " + msg.Day + " "
			if msg.Day == today {
				label = " today "
			}
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, dayBreak(p, label, innerW), "")
			prevDay = msg.Day
			prevUser = "" // a new day always re-shows the author header
		}
		marked := newMarkID != "" && msg.ID == newMarkID
		if marked { // the ── new ── rule sits right above the first unread message
			if i > 0 && !dayChanged {
				lines = append(lines, "")
			}
			lines = append(lines, newBreak(p, innerW), "")
			prevUser = ""
		}
		// Group consecutive messages from the same author: only the first shows
		// the name/time header; groups are separated by a blank line.
		grouped := i > 0 && msg.UserID == prevUser
		if i > 0 && !grouped && !dayChanged && !marked {
			for g := 0; g < gap; g++ {
				lines = append(lines, "")
			}
		}
		starts = append(starts, len(lines))
		lines = append(lines, messageLines(p, ws, msg, innerW, i == selIndex && focused, msg.MentionsMe, grouped)...)
		prevUser = msg.UserID
	}
	starts = append(starts, len(lines))
	return lines, starts
}

// newBreak is the unread marker rule (── new ──), drawn in the mention hue.
func newBreak(p theme.Palette, w int) string {
	label := " new "
	side := (w - lipgloss.Width(label)) / 2
	if side < 0 {
		side = 0
	}
	rule := lipgloss.NewStyle().Foreground(p.Orange).Render(strings.Repeat("─", side))
	return rule + lipgloss.NewStyle().Foreground(p.Orange).Bold(true).Render(label) + rule
}

func dayBreak(p theme.Palette, label string, w int) string {
	side := (w - lipgloss.Width(label)) / 2
	if side < 0 {
		side = 0
	}
	rule := lipgloss.NewStyle().Foreground(p.Border).Render(strings.Repeat("─", side))
	return rule + lipgloss.NewStyle().Foreground(p.Dim2).Render(label) + rule
}

func messageLines(p theme.Palette, ws *data.Workspace, msg data.Message, w int, selected, mention, grouped bool) []string {
	u, ok := ws.Users[msg.UserID]
	if !ok {
		u = data.User{Name: msg.UserID, Color: "fg"}
	}
	bodyW := w - timeGutter
	if bodyW < 10 {
		bodyW = 10
	}

	var out []string
	if !grouped { // grouped follow-ups omit the repeated name/time header
		tm := lipgloss.NewStyle().Foreground(p.Dim2).Width(timeGutter).Render(msg.Time)
		name := lipgloss.NewStyle().Foreground(p.Token(u.Color)).Bold(true).Render(u.Name)
		out = append(out, tm+name)
	}
	for _, ln := range strings.Split(markup.Render(p, msg.Text), "\n") {
		for _, wrapped := range Wrap(ln, bodyW) {
			out = append(out, strings.Repeat(" ", timeGutter)+wrapped)
		}
	}
	for _, ex := range msg.Extra { // file/attachment annotations, dimmed
		dim := lipgloss.NewStyle().Foreground(p.Dim)
		for _, wrapped := range Wrap(ex, bodyW) {
			out = append(out, strings.Repeat(" ", timeGutter)+dim.Render(wrapped))
		}
	}
	if len(msg.Reactions) > 0 {
		var rs []string
		for _, r := range msg.Reactions {
			rs = append(rs, r.Emoji+" "+lipgloss.NewStyle().Foreground(p.Dim).Render(fmt.Sprintf("%d", r.Count)))
		}
		out = append(out, strings.Repeat(" ", timeGutter)+strings.Join(rs, "   "))
	}
	if count := msg.ReplyCount; count > 0 || len(msg.Replies) > 0 {
		if len(msg.Replies) > count {
			count = len(msg.Replies)
		}
		who := replyWho(ws, msg.Replies)
		aff := lipgloss.NewStyle().Foreground(p.Dim2).Render("└ ") +
			lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Render(fmt.Sprintf("%d %s ", count, plural(count, "reply", "replies"))) +
			lipgloss.NewStyle().Foreground(p.Dim).Render(who) +
			lipgloss.NewStyle().Foreground(p.Dim2).Render("  ↵ open")
		out = append(out, strings.Repeat(" ", timeGutter)+aff)
	}

	switch {
	case selected:
		bar := lipgloss.NewStyle().Foreground(p.Accent).Render("▌")
		for i, ln := range out {
			out[i] = bar + theme.FillBg(ln, w-1, p.SelBg)
		}
	case mention:
		bar := lipgloss.NewStyle().Foreground(p.Orange).Render("▌")
		for i, ln := range out {
			out[i] = bar + theme.FillBg(ln, w-1, p.MentionBg)
		}
	}
	return out
}

func replyWho(ws *data.Workspace, replies []data.Reply) string {
	seen := map[string]bool{}
	var names []string
	for _, r := range replies {
		n := ws.Users[r.UserID].Name
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
		if len(names) == 3 {
			break
		}
	}
	return strings.Join(names, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
