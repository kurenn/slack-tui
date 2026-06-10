// Command slack-tui is a keyboard-first terminal Slack client — a vim-modal
// three-pane chat client that runs inside your terminal like vim or lazygit.
// Data currently comes from a mock workspace; real Slack integration lands later.
package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/kurenn/slack-tui/internal/app"
	"github.com/kurenn/slack-tui/internal/auth"
	"github.com/kurenn/slack-tui/internal/config"
	"github.com/kurenn/slack-tui/internal/onboarding"
	"github.com/kurenn/slack-tui/internal/root"
)

// version is stamped by goreleaser (-X main.version=v…); `go install` builds
// fall back to the module version from build info.
var version = "dev"

func versionString() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func main() {
	if len(os.Args) >= 2 && (os.Args[1] == "--version" || os.Args[1] == "-v" || os.Args[1] == "version") {
		fmt.Println("slack-tui", versionString())
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "login" {
		if err := login(); err != nil {
			fmt.Fprintln(os.Stderr, "login:", err)
			os.Exit(1)
		}
		return
	}
	// FORCE_COLOR makes --dump emit truecolor even without a TTY (for piping a
	// colored frame into a renderer like `freeze`).
	if os.Getenv("FORCE_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.TrueColor)
	}

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
			if len(os.Args) >= 5 {
				for _, key := range strings.Split(os.Args[4], ",") {
					m = onboarding.Key(m, key)
				}
			}
			fmt.Println(onboarding.Dump(m, w, h))
			return
		}
	}

	p := tea.NewProgram(root.New(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "slack-tui:", err)
		os.Exit(1)
	}
}

// login runs the browser OAuth flow on the command line and saves the tokens.
func login() error {
	creds := config.LoadOAuthCreds()
	if !creds.Ready() {
		return fmt.Errorf("missing app credentials — set SLACK_CLIENT_ID and SLACK_CLIENT_SECRET " +
			"(from your Slack app's Basic Information), or put them in ~/.config/slack-tui/oauth.json")
	}
	fmt.Println("Opening your browser to authorize slack-tui…")
	fmt.Printf("If it doesn't open, visit:\n  %s\n\n", auth.AuthorizeURL(creds.ClientID, "manual"))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	toks, err := auth.Login(ctx, creds)
	if err != nil {
		return err
	}
	saved, _ := config.LoadTokens()
	saved.User, saved.Bot = toks.User, toks.Bot // keep any existing app (xapp) token
	if err := config.SaveTokens(saved); err != nil {
		return err
	}
	fmt.Println("✓ Signed in — tokens saved. Run `slack-tui` to start.")
	if saved.App == "" {
		fmt.Println("  (For live channel unread, also set SLACK_APP_TOKEN=xapp-… — OAuth can't issue it.)")
	}
	return nil
}
