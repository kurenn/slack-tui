// Command slack-tui is a keyboard-first terminal Slack client — a vim-modal
// three-pane chat client that runs inside your terminal like vim or lazygit.
// Data currently comes from a mock workspace; real Slack integration lands later.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/abrahamkuri/slack-tui/internal/app"
	"github.com/abrahamkuri/slack-tui/internal/onboarding"
	"github.com/abrahamkuri/slack-tui/internal/root"
)

func main() {
	// Hidden dev flag: `slack-tui --dump 100x30` renders one frame to stdout and
	// exits — used for headless verification and screenshots.
	if len(os.Args) >= 3 && os.Args[1] == "--dump" {
		var w, h int
		if _, err := fmt.Sscanf(os.Args[2], "%dx%d", &w, &h); err == nil {
			m := app.WithSize(app.New(), w, h)
			if len(os.Args) >= 4 { // replay comma-separated keys, e.g. "k,k,t" or "ctrl+k,d,e"
				for _, key := range strings.Split(os.Args[3], ",") {
					m = app.Key(m, key)
				}
			}
			fmt.Println(app.Dump(m, w, h))
			return
		}
	}
	// `slack-tui --dump-ob 90x28 wizard:theme` renders an onboarding phase.
	if len(os.Args) >= 4 && os.Args[1] == "--dump-ob" {
		var w, h int
		if _, err := fmt.Sscanf(os.Args[2], "%dx%d", &w, &h); err == nil {
			m := onboarding.Goto(onboarding.New(), os.Args[3])
			fmt.Println(onboarding.Dump(m, w, h))
			return
		}
	}

	p := tea.NewProgram(root.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "slack-tui:", err)
		os.Exit(1)
	}
}
