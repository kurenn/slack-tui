package app

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kurenn/slack-tui/internal/theme"
)

// uploadMsg is returned by uploadCmd when the upload completes (or fails).
type uploadMsg struct {
	convID  string
	paths   []string
	comment string
	err     error
}

// parseDroppedPaths extracts existing local file paths from text a terminal inserted
// for a file drop (or a typed path). Handles quoting, backslash-escaped spaces,
// file:// URLs, ~ expansion, and multiple space-separated paths.
//
// Strategy: first try the WHOLE trimmed string as a single path (handles paths
// with unescaped spaces like "/Users/me/My File.pdf"). Otherwise split into tokens
// and require EVERY token to be an existing regular file — so prose containing a
// path like "see /etc/hosts for details" falls through to nil (normal text paste).
func parseDroppedPaths(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Try the whole string as one path first.
	if p := normalizeDropPath(s); p != "" {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return []string{p}
		}
	}
	tokens := splitDropTokens(s)
	if len(tokens) == 0 {
		return nil
	}
	var out []string
	for _, t := range tokens {
		p := normalizeDropPath(t)
		if p == "" {
			return nil
		}
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			return nil // any non-file token → treat the whole paste as text
		}
		out = append(out, p)
	}
	return out
}

func splitDropTokens(s string) []string {
	var toks []string
	var cur strings.Builder
	var quote rune
	esc := false
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\' && quote != '\'':
			esc = true // drop the backslash, take next char literally (escaped spaces)
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return toks
}

func normalizeDropPath(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if strings.HasPrefix(t, "file://") {
		u := strings.TrimPrefix(t, "file://")
		if i := strings.Index(u, "/"); i > 0 {
			u = u[i:] // drop optional host
		}
		if dec, err := url.PathUnescape(u); err == nil {
			t = dec
		} else {
			t = u
		}
	}
	if t == "~" || strings.HasPrefix(t, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			t = filepath.Join(home, strings.TrimPrefix(t, "~"))
		}
	}
	return t
}

// stageAttachments adds files to the pending set (deduped) and focuses the composer.
func (m *Model) stageAttachments(paths []string) tea.Cmd {
	seen := map[string]bool{}
	for _, p := range m.pendingFiles {
		seen[p] = true
	}
	for _, p := range paths {
		if !seen[p] {
			m.pendingFiles = append(m.pendingFiles, p)
			seen[p] = true
		}
	}
	if m.focus == focusSidebar {
		m.focus = focusMessages
	}
	return m.enterInsert(focusMessages)
}

// sendAttachments uploads the staged files to the active conversation with the
// composer text as comment, clearing the staging state.
func (m *Model) sendAttachments() tea.Cmd {
	paths := m.pendingFiles
	comment := strings.TrimSpace(m.draft.Value())
	m.pendingFiles = nil
	m.draft.SetValue("")
	m.clearSuggest()
	m.syncComposerSizes()
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}
	m.uploadNote = "uploading " + strings.Join(names, ", ") + " …"
	return m.uploadCmd(m.activeID, paths, comment)
}

func (m Model) uploadCmd(convID string, paths []string, comment string) tea.Cmd {
	src := m.src
	return func() tea.Msg {
		return uploadMsg{convID: convID, paths: paths, comment: comment, err: src.Upload(convID, paths, comment)}
	}
}

// attachRows is how many extra rows the staging chip + upload note occupy; folded
// into composerHeight so the message viewport height stays correct.
func (m Model) attachRows() int {
	n := 0
	if len(m.pendingFiles) > 0 {
		n++
	}
	if m.uploadNote != "" {
		n++
	}
	return n
}

// openAttach opens the attach-file input overlay.
func (m *Model) openAttach() tea.Cmd {
	m.attachOpen = true
	m.attachInput.SetValue("")
	return m.attachInput.Focus()
}

// attachKey handles keystrokes for the attach overlay.
func (m Model) attachKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, m.quit()
	case "esc":
		m.attachOpen = false
		m.attachInput.Blur()
		return m, nil
	case "enter":
		m.attachOpen = false
		m.attachInput.Blur()
		paths := parseDroppedPaths(m.attachInput.Value())
		if len(paths) > 0 {
			return m, m.stageAttachments(paths)
		}
		return m, m.flash(fmt.Errorf("no such file: %s", m.attachInput.Value()))
	}
	var cmd tea.Cmd
	m.attachInput, cmd = m.attachInput.Update(msg)
	return m, cmd
}

// overlayAttach composites the attach prompt over the frame.
func (m Model) overlayAttach(frame string) string {
	p := m.pal
	w := 48
	inner := w - 4
	bs := lipgloss.NewStyle().Foreground(p.Accent)
	m.attachInput.Width = inner - 4

	rows := []string{
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Fg).Bold(true).Render(" attach file…"), inner, p.Panel),
		theme.FillBg(" "+lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Render("📎 ")+m.attachInput.View(), inner, p.Panel),
		theme.FillBg(lipgloss.NewStyle().Foreground(p.Dim2).Render(" ↵ stage · esc cancel"), inner, p.Panel),
	}
	box := []string{bs.Render("┌" + strings.Repeat("─", w-2) + "┐")}
	for _, r := range rows {
		box = append(box, bs.Render("│")+" "+r+" "+bs.Render("│"))
	}
	box = append(box, bs.Render("└"+strings.Repeat("─", w-2)+"┘"))

	x := (m.width - w) / 2
	y := m.height / 3
	if y < 1 {
		y = 1
	}
	return overlay(frame, strings.Join(box, "\n"), x, y)
}
