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

// run dispatches on argv and returns the process exit code plus whether the
// interactive TUI should be launched. It never constructs a tea.Program
// itself — main() is the sole caller of tea.NewProgram — so every branch
// below is exercisable in tests without starting a real Bubble Tea program.
// A malformed --dump/--dump-ob size falls through to launch=true, matching
// today's behavior of starting the TUI rather than erroring.
func run(args []string) (code int, launch bool) {
	// The onboarding app-setup screen offers the manifest for copying; go:embed
	// only reaches files inside this package, so hand it over here.
	onboarding.AppManifest = manifest

	if len(args) >= 2 && (args[1] == "--version" || args[1] == "-v" || args[1] == "version") {
		fmt.Println("slack-tui", versionString())
		return 0, false
	}
	if len(args) >= 2 && (args[1] == "doctor" || args[1] == "--doctor") {
		return doctor.Run(versionString()), false
	}
	if len(args) >= 2 && args[1] == "setup" {
		if err := setup(); err != nil {
			fmt.Fprintln(os.Stderr, "setup:", err)
			return 1, false
		}
		return 0, false
	}
	if len(args) >= 2 && args[1] == "login" {
		if err := login(); err != nil {
			fmt.Fprintln(os.Stderr, "login:", err)
			return 1, false
		}
		return 0, false
	}
	// `slack-tui --workspace <name>` opens a specific workspace for this run
	// only (the in-app switcher persists; the flag deliberately doesn't).
	if len(args) >= 3 && args[1] == "--workspace" {
		config.ActiveOverride = args[2]
		args = append(args[:1], args[3:]...)
	}
	// FORCE_COLOR makes --dump emit truecolor even without a TTY (for piping a
	// colored frame into a renderer like `freeze`).
	if os.Getenv("FORCE_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.TrueColor)
	}

	// Hidden dev flag: `slack-tui --dump 100x30` renders one frame to stdout and
	// exits — used for headless verification and screenshots.
	if len(args) >= 3 && args[1] == "--dump" {
		var w, h int
		if _, err := fmt.Sscanf(args[2], "%dx%d", &w, &h); err == nil {
			m := app.WithSize(app.New(), w, h)
			if len(args) >= 4 { // replay comma-separated keys, e.g. "k,k,t" or "ctrl+k,d,e"
				for _, key := range strings.Split(args[3], ",") {
					m = app.Key(m, key)
				}
			}
			fmt.Println(app.Dump(m, w, h))
			return 0, false
		}
	}
	// `slack-tui --dump-ob 90x28 wizard:theme` renders an onboarding phase.
	if len(args) >= 4 && args[1] == "--dump-ob" {
		var w, h int
		if _, err := fmt.Sscanf(args[2], "%dx%d", &w, &h); err == nil {
			m := onboarding.Goto(onboarding.New(), args[3])
			if len(args) >= 5 {
				for _, key := range strings.Split(args[4], ",") {
					m = onboarding.Key(m, key)
				}
			}
			fmt.Println(onboarding.Dump(m, w, h))
			return 0, false
		}
	}

	return 0, true
}

func main() {
	code, launch := run(os.Args)
	if !launch {
		os.Exit(code)
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
		return fmt.Errorf("no Slack app configured — run `slack-tui setup`, which walks you " +
			"through creating one (or set SLACK_CLIENT_ID if you already have its client ID)")
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
		// Expected: Slack forces rotation for a loopback redirect. Say what
		// happens next rather than sounding an alarm — this is the normal path.
		when := time.Unix(toks.ExpiresAt, 0)
		fmt.Printf("  This token expires %s and is refreshed automatically — no action needed.\n",
			when.Format("Mon 15:04"))
	}
	fmt.Println("  (For live channel unread, provide the app-level xapp AND bot xoxb tokens from your")
	fmt.Println("   app's admin page — Slack issues neither through a loopback sign-in.)")
	return nil
}
