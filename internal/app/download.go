package app

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kurenn/slack-tui/internal/data"
)

type downloadMsg struct {
	paths []string
	err   error
}

// downloadDir is where saved files land (~/Downloads, created on demand by the source).
func downloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// downloadFiles saves the selected message's file(s) to ~/Downloads.
func (m *Model) downloadFiles() tea.Cmd {
	msg, ok := m.selectedMsg()
	if !ok || len(msg.Files) == 0 {
		return m.flash(fmt.Errorf("no file in this message"))
	}
	return m.downloadCmd(msg.Files)
}

func (m Model) downloadCmd(files []data.File) tea.Cmd {
	src, dir := m.src, downloadDir()
	return func() tea.Msg {
		var paths []string
		for _, f := range files {
			p, err := src.Download(f, dir)
			if err != nil {
				return downloadMsg{err: err}
			}
			paths = append(paths, p)
		}
		return downloadMsg{paths: paths}
	}
}

// savedToastLabel builds the confirmation text for a completed download.
func savedToastLabel(paths []string) string {
	if len(paths) == 1 {
		return "✓ saved " + filepath.Base(paths[0])
	}
	return fmt.Sprintf("✓ saved %d files to ~/Downloads", len(paths))
}
