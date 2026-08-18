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
	"github.com/kurenn/slack-tui/internal/doctor"
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
	if len(os.Args) >= 2 && (os.Args[1] == "doctor" || os.Args[1] == "--doctor") {
		os.Exit(doctor.Run(versionString()))
	}
	if len(os.Args) >= 2 && os.Args[1] == "login" {
		if err := login(); err != nil {
			fmt.Fprintln(os.Stderr, "login:", err)
			os.Exit(1)
		}
		return
	}
	// `slack-tui --workspace <name>` opens a specific workspace for this run
	// only (the in-app switcher persists; the flag deliberately doesn't).
	if len(os.Args) >= 3 && os.Args[1] == "--workspace" {
		config.ActiveOverride = os.Args[2]
		os.Args = append(os.Args[:1], os.Args[3:]...)
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
		return fmt.Errorf("missing app credentials — this build has no built-in Slack app, so set " +
			"SLACK_CLIENT_ID (from your own Slack app's Basic Information), or put it in " +
			"~/.config/slack-tui/oauth.json")
	}
	if creds.ClientSecret != "" {
		fmt.Println("note: a client secret is configured but no longer used — Slack requires PKCE")
		fmt.Println("      for loopback redirects, and a PKCE client must not send one. Safe to remove.")
	}
	fmt.Println("Opening your browser to authorize slack-tui…")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	toks, team, err := auth.Login(ctx, creds, func(u string) {
		fmt.Printf("If it doesn't open, visit:\n  %s\n\n", u)
	})
	if err != nil {
		return err
	}
	name := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(team.Name), " ", "-"))
	if name == "" {
		name = "default"
	}
	ws := config.Workspace{Name: name, TeamID: team.ID, Tokens: toks}
	if err := config.SaveWorkspace(ws); err != nil { // upserts; keeps a stored xapp token for this team
		return err
	}
	fmt.Printf("✓ Signed in to %s — saved as workspace %q (now active). Run `slack-tui` to start.\n", team.Name, name)
	if all, _, _ := config.LoadWorkspaces(); len(all) > 1 {
		fmt.Println("  Switch workspaces in-app via Ctrl-K → \"Switch workspace\", or `slack-tui --workspace <name>`.")
	}
	if toks.Rotating() {
		fmt.Println("  ! Slack issued a rotating user token — slack-tui can't refresh it yet, so you'll")
		fmt.Println("    need to run `slack-tui login` again when it expires. Please report this.")
	}
	fmt.Println("  (For live channel unread, provide the app-level xapp AND bot xoxb tokens from your")
	fmt.Println("   app's admin page — Slack issues neither through a loopback sign-in.)")
	return nil
}
