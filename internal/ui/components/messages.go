package components

import (
	"fmt"
	"strings"

	"github.com/abrahamkuri/slack-tui/internal/data"
	"github.com/abrahamkuri/slack-tui/internal/markup"
	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

const timeGutter = 6 // "09:21 "

// MessagesBody renders a conversation's messages to body lines plus the starting
// line index of each message (with a trailing sentinel = total line count), so
// the caller can scroll the selected message into view. The cursor/mention
// highlight only shows when focused.
func MessagesBody(p theme.Palette, ws *data.Workspace, msgs []data.Message, selIndex int, focused bool, density theme.Density, innerW int) (lines []string, starts []int) {
	lines = append(lines, dayBreak(p, innerW), "")
	for i, msg := range msgs {
		starts = append(starts, len(lines))
		lines = append(lines, messageLines(p, ws, msg, innerW, i == selIndex && focused, msg.MentionsMe)...)
		for g := 0; g < density.MsgGap(); g++ {
			lines = append(lines, "")
		}
	}
	starts = append(starts, len(lines))
	return lines, starts
}

func dayBreak(p theme.Palette, w int) string {
	label := " today "
	side := (w - lipgloss.Width(label)) / 2
	if side < 0 {
		side = 0
	}
	rule := lipgloss.NewStyle().Foreground(p.Border).Render(strings.Repeat("─", side))
	return rule + lipgloss.NewStyle().Foreground(p.Dim2).Render(label) + rule
}

func messageLines(p theme.Palette, ws *data.Workspace, msg data.Message, w int, selected, mention bool) []string {
	u, ok := ws.Users[msg.UserID]
	if !ok {
		u = data.User{Name: msg.UserID, Color: "fg"}
	}
	bodyW := w - timeGutter
	if bodyW < 10 {
		bodyW = 10
	}

	tm := lipgloss.NewStyle().Foreground(p.Dim2).Width(timeGutter).Render(msg.Time)
	name := lipgloss.NewStyle().Foreground(p.Token(u.Color)).Bold(true).Render(u.Name)

	var out []string
	out = append(out, tm+name)
	for _, ln := range strings.Split(markup.Render(p, msg.Text), "\n") {
		for _, wrapped := range Wrap(ln, bodyW) {
			out = append(out, strings.Repeat(" ", timeGutter)+wrapped)
		}
	}
	if len(msg.Reactions) > 0 {
		var rs []string
		for _, r := range msg.Reactions {
			rs = append(rs, lipgloss.NewStyle().Background(p.SelBg).Render(fmt.Sprintf(" %s %d ", r.Emoji, r.Count)))
		}
		out = append(out, strings.Repeat(" ", timeGutter)+strings.Join(rs, " "))
	}
	if len(msg.Replies) > 0 {
		who := replyWho(ws, msg.Replies)
		aff := lipgloss.NewStyle().Foreground(p.Dim).Render("└─ ") +
			lipgloss.NewStyle().Foreground(p.Accent).Render(fmt.Sprintf("%d %s ", len(msg.Replies), plural(len(msg.Replies), "reply", "replies"))) +
			lipgloss.NewStyle().Foreground(p.Dim2).Render(who+"  ↵ open")
		out = append(out, strings.Repeat(" ", timeGutter)+aff)
	}

	switch {
	case selected:
		bar := lipgloss.NewStyle().Foreground(p.Accent).Render("▌")
		for i, ln := range out {
			out[i] = bar + padLine(ln, w-1, p.SelBg)
		}
	case mention:
		bar := lipgloss.NewStyle().Foreground(p.Orange).Render("▌")
		for i, ln := range out {
			out[i] = bar + padLine(ln, w-1, p.MentionBg)
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
