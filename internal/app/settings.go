package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/source"
	"github.com/kurenn/slack-tui/internal/theme"
	"github.com/kurenn/slack-tui/internal/ui/components"
)

// Settings overlay — change appearance/presence live (`,` to open). Changes
// apply immediately and persist to prefs.json on close.

var (
	accentChoices  = []string{"auto", "cyan", "green", "purple", "orange", "magenta", "blue"}
	densityChoices = []string{"comfortable", "compact"}
	statusChoices  = []string{"online", "away", "dnd"}
)

const settingsRows = 6

func (m *Model) openSettings()  { m.settingsOpen, m.settingsSel = true, 0 }
func (m *Model) closeSettings() { m.settingsOpen = false; _ = config.Save(m.prefs) }

// setStatus updates presence locally, persists it, and pushes it to the backend.
func (m *Model) setStatus(status string) tea.Cmd {
	m.myStatus = status
	m.prefs.Status = status
	_ = config.Save(m.prefs)
	return m.setPresenceCmd()
}

func (m Model) settingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", ",", "q", "ctrl+c":
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		m.closeSettings()
	case "j", "down":
		m.settingsSel = clamp(m.settingsSel+1, 0, settingsRows-1)
	case "k", "up":
		m.settingsSel = clamp(m.settingsSel-1, 0, settingsRows-1)
	case "l", "right", " ":
		return m, m.cycleSetting(1)
	case "h", "left":
		return m, m.cycleSetting(-1)
	}
	return m, nil
}

func (m *Model) cycleSetting(dir int) tea.Cmd {
	switch m.settingsSel {
	case 0:
		m.prefs.Theme = theme.Cycle[wrap(indexOf(theme.Cycle, m.prefs.Theme)+dir, len(theme.Cycle))]
		m.pal = theme.Resolve(m.prefs.Theme, m.prefs.Accent)
	case 1:
		m.prefs.Accent = accentChoices[wrap(indexOfStr(accentChoices, m.prefs.Accent)+dir, len(accentChoices))]
		m.pal = theme.Resolve(m.prefs.Theme, m.prefs.Accent)
	case 2:
		m.density = theme.ParseDensity(densityChoices[wrap(indexOfStr(densityChoices, m.density.String())+dir, len(densityChoices))])
		m.prefs.Density = m.density.String()
	case 3:
		m.myStatus = statusChoices[wrap(indexOfStr(statusChoices, m.myStatus)+dir, len(statusChoices))]
		m.prefs.Status = m.myStatus
		return m.setPresenceCmd()
	case 4: // group DMs: reload the conversation list with mpims toggled
		m.prefs.GroupDMs = !m.prefs.GroupDMs
		if sl, ok := m.src.(*source.Slack); ok {
			sl.SetGroupDMs(m.prefs.GroupDMs)
			return m.reloadCmd()
		}
	case 5:
		m.showHints = !m.showHints
	}
	return nil
}

// overlaySettings composites the settings card over the frame.
func (m Model) overlaySettings(frame string) string {
	p := m.pal
	w := 52
	inner := w - 4
	val := func(s string) string {
		return lipgloss.NewStyle().Foreground(p.Accent).Render("‹ ") +
			lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(s) +
			lipgloss.NewStyle().Foreground(p.Accent).Render(" ›")
	}
	rows := []struct{ label, value, extra string }{
		{"Theme", titleCase(m.prefs.Theme), settingsSwatch(m.prefs.Theme)},
		{"Accent", titleCase(m.prefs.Accent), accentChip(p, m.prefs.Accent)},
		{"Density", m.density.String(), ""},
		{"Status", statusName(m.myStatus), components.PresenceDot(p, m.myStatus)},
		{"Group DMs", onOff(m.prefs.GroupDMs), ""},
		{"Key hints", onOff(m.showHints), ""},
	}

	var lines []string
	for i, r := range rows {
		bar := "  "
		labelColor := p.Dim
		if i == m.settingsSel {
			bar = lipgloss.NewStyle().Foreground(p.Accent).Render("▌") + " "
			labelColor = p.Fg
		}
		label := lipgloss.NewStyle().Foreground(labelColor).Width(11).Render(r.label)
		line := bar + label + val(r.value)
		if r.extra != "" {
			line += "  " + r.extra
		}
		lines = append(lines, theme.FillBg(line, inner, p.Panel))
	}

	body := []string{
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(" settings"), inner, p.Panel),
		theme.FillBg("", inner, p.Panel),
	}
	body = append(body, lines...)
	body = append(body,
		theme.FillBg("", inner, p.Panel),
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Dim2).Render(" j/k move · h/l change · esc close"), inner, p.Panel),
	)

	bs := lipgloss.NewStyle().Foreground(p.Accent)
	box := []string{bs.Render("┌" + strings.Repeat("─", w-2) + "┐")}
	for _, l := range body {
		box = append(box, bs.Render("│")+" "+l+" "+bs.Render("│"))
	}
	box = append(box, bs.Render("└"+strings.Repeat("─", w-2)+"┘"))

	x := (m.width - w) / 2
	y := (m.height - len(box)) / 2
	if y < 1 {
		y = 1
	}
	return overlay(frame, strings.Join(box, "\n"), x, y)
}

// ── small helpers ─────────────────────────────────────────────────────────

func settingsSwatch(themeVal string) string {
	tp := theme.Resolve(themeVal, "auto")
	var b strings.Builder
	for _, c := range []lipgloss.Color{tp.Blue, tp.Green, tp.Purple, tp.Orange} {
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render("██"))
	}
	return b.String()
}

func accentChip(p theme.Palette, val string) string {
	c := p.Accent
	switch val {
	case "cyan":
		c = lipgloss.Color("#56d4dd")
	case "green":
		c = lipgloss.Color("#7ee787")
	case "purple":
		c = lipgloss.Color("#c8a2ff")
	case "orange":
		c = lipgloss.Color("#f0a868")
	case "magenta":
		c = lipgloss.Color("#ff7bd5")
	case "blue":
		c = lipgloss.Color("#6cb6ff")
	}
	return lipgloss.NewStyle().Foreground(c).Render("██")
}

func statusName(s string) string {
	switch s {
	case "online":
		return "Active"
	case "away":
		return "Away"
	case "dnd":
		return "Do not disturb"
	}
	return s
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func wrap(i, n int) int { return ((i % n) + n) % n }

func indexOfStr(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return 0
}
