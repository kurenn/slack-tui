// Command slack-tui is a keyboard-first terminal Slack client. This entrypoint
// currently renders the static three-pane shell against mock data to validate
// the theme system and the Pane primitive; the keyboard engine, palette,
// threads, and onboarding land in later steps.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/abrahamkuri/slack-tui/internal/config"
	"github.com/abrahamkuri/slack-tui/internal/data"
	"github.com/abrahamkuri/slack-tui/internal/markup"
	"github.com/abrahamkuri/slack-tui/internal/theme"
	"github.com/abrahamkuri/slack-tui/internal/ui/pane"
)

const sidebarWidth = 30

type model struct {
	ws       *data.Workspace
	prefs    config.Prefs
	pal      theme.Palette
	density  theme.Density
	width    int
	height   int
	activeID string
}

func newModel() model {
	prefs, _ := config.Load()
	return model{
		ws:       data.Mock(),
		prefs:    prefs,
		pal:      theme.Resolve(prefs.Theme, prefs.Accent),
		density:  theme.ParseDensity(prefs.Density),
		activeID: "engineering",
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "T": // temporary: cycle theme to eyeball the palettes
			i := 0
			for j, name := range theme.Cycle {
				if name == m.prefs.Theme {
					i = j
				}
			}
			m.prefs.Theme = theme.Cycle[(i+1)%len(theme.Cycle)]
			m.pal = theme.Resolve(m.prefs.Theme, m.prefs.Accent)
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.width < 50 || m.height < 12 {
		return lipgloss.NewStyle().Foreground(m.pal.Dim).
			Render("slack-tui needs at least 50×12 — resize the terminal.")
	}

	bodyH := m.height - 2 // minus titlebar + statusbar
	msgWidth := m.width - sidebarWidth - 1

	sidebar := pane.Render(m.pal, pane.Options{
		Title: "workspace", Right: "@" + m.ws.Handle, Focused: false,
		Width: sidebarWidth, Height: bodyH, Body: m.sidebarBody(sidebarWidth - 2),
	})

	conv, _ := m.ws.Conversation(m.activeID)
	msgs := pane.Render(m.pal, pane.Options{
		Title: "#" + conv.Name, Right: conv.Topic, Focused: true,
		Width: msgWidth, Height: bodyH, Body: m.messagesBody(msgWidth - 2),
	})

	workspace := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", msgs)
	return strings.Join([]string{m.titlebar(), workspace, m.statusbar(conv)}, "\n")
}

// ── titlebar ────────────────────────────────────────────────────────────────
func (m model) titlebar() string {
	bg := lipgloss.NewStyle().Background(m.pal.TitlebarBg)
	dim := bg.Foreground(m.pal.Dim)
	dot := func(hex string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Background(m.pal.TitlebarBg).Render("●")
	}
	lights := " " + dot("#ff5f57") + " " + dot("#febc2e") + " " + dot("#28c840") + "  "
	title := dim.Render("slack-tui — ") +
		bg.Foreground(m.pal.Fg).Bold(true).Render(m.ws.Name) +
		dim.Render(" — 2 panes")
	me := bg.Foreground(presenceColor(m.pal, m.prefs.Status)).Render("●") +
		dim.Render(" @"+m.ws.Me().Handle)

	left := lights + title
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(me)
	if gap < 1 {
		gap = 1
	}
	return left + bg.Render(strings.Repeat(" ", gap)) + me
}

// ── status bar ──────────────────────────────────────────────────────────────
func (m model) statusbar(conv data.Conversation) string {
	bg := lipgloss.NewStyle().Background(m.pal.StatusBg)
	mode := lipgloss.NewStyle().Background(m.pal.Accent).Foreground(m.pal.Bg).
		Bold(true).Padding(0, 1).Render("NORMAL")
	loc := bg.Foreground(m.pal.Fg).Padding(0, 1).Render("#" + conv.Name)
	focus := bg.Foreground(m.pal.Dim).Padding(0, 1).Render("messages")

	hints := []struct{ k, d string }{
		{"j/k", "messages"}, {"t", "thread"}, {"i", "write"}, {"h/l", "panes"}, {"⌃k", "palette"},
	}
	var hb strings.Builder
	for _, h := range hints {
		hb.WriteString(bg.Foreground(m.pal.Fg).Bold(true).Render(h.k))
		hb.WriteString(bg.Foreground(m.pal.Dim).Render(" " + h.d + "   "))
	}
	hintStr := hb.String()

	left := mode + loc + focus
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(hintStr)
	if gap < 1 {
		gap = 1
	}
	return left + bg.Render(strings.Repeat(" ", gap)) + hintStr
}

// ── sidebar body ────────────────────────────────────────────────────────────
func (m model) sidebarBody(w int) string {
	var lines []string
	header := lipgloss.NewStyle().Foreground(m.pal.Dim2).Background(m.pal.Panel)
	lines = append(lines, header.Render("── channels ──"))
	for _, c := range m.ws.Channels {
		lines = append(lines, m.sideRow(c, w))
	}
	lines = append(lines, "", header.Render("── direct messages ──"))
	for _, c := range m.ws.DMs {
		lines = append(lines, m.sideRow(c, w))
	}
	return strings.Join(lines, "\n")
}

func (m model) sideRow(c data.Conversation, w int) string {
	active := c.ID == m.activeID
	unread := c.Unread > 0

	nameColor := m.pal.Dim
	if unread || active {
		nameColor = m.pal.Fg
	}
	var sigil string
	if c.Type == "channel" {
		sigil = lipgloss.NewStyle().Foreground(m.pal.Dim2).Bold(true).Render("#")
	} else {
		sigil = presenceDot(m.pal, m.ws.Users[c.UserID].Status)
	}
	name := lipgloss.NewStyle().Foreground(nameColor).Bold(unread || active).Render(c.Name)

	var badge string
	if c.Mention {
		badge = lipgloss.NewStyle().Foreground(m.pal.Bg).Background(m.pal.Orange).
			Render(fmt.Sprintf(" @%d ", c.Unread))
	} else if unread {
		badge = lipgloss.NewStyle().Foreground(m.pal.Dim).Background(m.pal.SelBg).
			Render(fmt.Sprintf(" %d ", c.Unread))
	}

	left := " " + sigil + " " + name
	gap := w - lipgloss.Width(left) - lipgloss.Width(badge)
	if gap < 0 {
		gap = 0
	}
	row := left + strings.Repeat(" ", gap) + badge

	if active {
		return lipgloss.NewStyle().Width(w).Background(m.pal.SelBg).Render(ansi.Truncate(row, w, ""))
	}
	return row
}

// ── messages body ───────────────────────────────────────────────────────────
func (m model) messagesBody(w int) string {
	msgs := m.ws.Messages[m.activeID]
	var lines []string
	lines = append(lines, m.dayBreak(w), "")
	for i, msg := range msgs {
		selected := i == len(msgs)-1
		lines = append(lines, m.messageLines(msg, w, selected)...)
		for g := 0; g < m.density.MsgGap(); g++ {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) dayBreak(w int) string {
	label := " today "
	side := (w - lipgloss.Width(label)) / 2
	if side < 0 {
		side = 0
	}
	rule := lipgloss.NewStyle().Foreground(m.pal.Border).Render(strings.Repeat("─", side))
	return rule + lipgloss.NewStyle().Foreground(m.pal.Dim2).Render(label) + rule
}

func (m model) messageLines(msg data.Message, w int, selected bool) []string {
	u, ok := m.ws.Users[msg.UserID]
	if !ok {
		u = data.User{Name: msg.UserID, Color: "fg"}
	}
	const gutter = 6 // "09:21 "
	bodyW := w - gutter
	if bodyW < 10 {
		bodyW = 10
	}

	time := lipgloss.NewStyle().Foreground(m.pal.Dim2).Width(gutter).Render(msg.Time)
	name := lipgloss.NewStyle().Foreground(m.pal.Token(u.Color)).Bold(true).Render(u.Name)

	var out []string
	out = append(out, time+name)
	for _, ln := range strings.Split(markup.Render(m.pal, msg.Text), "\n") {
		for _, wrapped := range wrap(ln, bodyW) {
			out = append(out, strings.Repeat(" ", gutter)+wrapped)
		}
	}
	if len(msg.Reactions) > 0 {
		var rs []string
		for _, r := range msg.Reactions {
			rs = append(rs, lipgloss.NewStyle().Background(m.pal.SelBg).
				Render(fmt.Sprintf(" %s %d ", r.Emoji, r.Count)))
		}
		out = append(out, strings.Repeat(" ", gutter)+strings.Join(rs, " "))
	}
	if len(msg.Replies) > 0 {
		aff := lipgloss.NewStyle().Foreground(m.pal.Dim).Render("└─ ") +
			lipgloss.NewStyle().Foreground(m.pal.Accent).Render(fmt.Sprintf("%d replies ", len(msg.Replies))) +
			lipgloss.NewStyle().Foreground(m.pal.Dim2).Render("↵ open")
		out = append(out, strings.Repeat(" ", gutter)+aff)
	}

	if selected {
		bar := lipgloss.NewStyle().Foreground(m.pal.Accent).Render("▌")
		for i, ln := range out {
			padded := lipgloss.NewStyle().Width(w-1).Background(m.pal.SelBg).Render(ansi.Truncate(ln, w-1, ""))
			out[i] = bar + padded
		}
	}
	return out
}

// ── helpers ───────────────────────────────────────────────────────────────
func presenceColor(p theme.Palette, status string) lipgloss.Color {
	switch status {
	case "online":
		return p.Green
	case "away":
		return p.Yellow
	case "dnd":
		return p.Red
	default:
		return p.Dim2
	}
}

func presenceDot(p theme.Palette, status string) string {
	return lipgloss.NewStyle().Foreground(presenceColor(p, status)).Render("●")
}

// wrap hard-wraps a styled line to width, ansi-aware.
func wrap(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for lipgloss.Width(s) > width {
		out = append(out, ansi.Truncate(s, width, ""))
		s = ansi.TruncateLeft(s, width, "")
	}
	out = append(out, s)
	return out
}

func main() {
	// Hidden dev flag: `slack-tui --dump 100x30` renders one frame to stdout and
	// exits — used for headless verification and screenshots.
	if len(os.Args) == 3 && os.Args[1] == "--dump" {
		var w, h int
		if _, err := fmt.Sscanf(os.Args[2], "%dx%d", &w, &h); err == nil {
			m := newModel()
			m.width, m.height = w, h
			fmt.Println(m.View())
			return
		}
	}

	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "slack-tui:", err)
		os.Exit(1)
	}
}
