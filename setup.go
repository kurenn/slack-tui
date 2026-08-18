package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"

	"github.com/kurenn/slack-tui/internal/auth"
	"github.com/kurenn/slack-tui/internal/config"
)

// manifest is embedded so the app you create always matches the binary you're
// running — most importantly its redirect URLs, which must cover every loopback
// port auth may bind.
//
//go:embed slack-app-manifest.json
var manifest string

// setup walks the user through creating their own Slack app and signing in
// against it. It exists because Slack has no way to hand someone a ready-made
// app: the manifest must be pasted by a human into their own workspace. The
// command removes everything around that one unavoidable step.
func setup() error {
	fmt.Println("slack-tui setup — create your own Slack app, then sign in against it.")
	fmt.Println()
	if config.DefaultClientID != "" {
		fmt.Println("Note: this build already has a Slack app baked in, so `slack-tui login`")
		fmt.Println("works on its own. Continue only if you'd rather use your own app.")
		fmt.Println()
	}
	if existing := config.LoadOAuthCreds(); existing.ClientID != "" && existing.ClientID != config.DefaultClientID {
		fmt.Printf("You already have a client ID configured (%s). Continuing replaces it.\n\n", existing.ClientID)
	}

	// 1. Get the manifest somewhere the user can paste from.
	fmt.Println("Step 1 — create the app")
	if err := clipboard.WriteAll(manifest); err == nil {
		fmt.Println("  ✓ the app manifest is on your clipboard")
	} else {
		path, werr := writeManifestFallback()
		if werr != nil {
			fmt.Println("  ! couldn't reach your clipboard; the manifest is in slack-app-manifest.json")
		} else {
			fmt.Printf("  ! couldn't reach your clipboard — manifest written to:\n      %s\n", path)
		}
	}
	fmt.Println("  1. https://api.slack.com/apps  →  Create New App  →  From an app manifest")
	fmt.Println("  2. Pick your workspace, paste the manifest, Next → Create.")
	fmt.Println()
	_ = openBrowser("https://api.slack.com/apps")

	// 2. Collect the client ID. No secret: sign-in is PKCE, which never sends one.
	fmt.Println("Step 2 — copy the Client ID from Basic Information → App Credentials")
	fmt.Println("  (the Client ID only — slack-tui never needs your client secret)")
	id, err := promptClientID()
	if err != nil {
		return err
	}
	if err := config.SaveOAuthCreds(config.OAuthCreds{ClientID: id}); err != nil {
		return fmt.Errorf("saving oauth.json: %w", err)
	}
	dir, _ := config.Dir()
	fmt.Printf("  ✓ saved to %s\n\n", filepath.Join(dir, "oauth.json"))

	// 3. Straight into the browser sign-in — no reason to make them run it.
	fmt.Println("Step 3 — sign in")
	if err := login(); err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("For live channel unread (Socket Mode), also paste the xapp- and xoxb- tokens")
	fmt.Println("from your app's admin page — Slack can't issue those through a browser sign-in.")
	return nil
}

// promptClientID reads and validates the client ID, re-asking on a bad paste
// rather than failing a multi-step flow at the last step.
func promptClientID() (string, error) {
	in := bufio.NewScanner(os.Stdin)
	for attempt := 0; attempt < 5; attempt++ {
		fmt.Print("  Client ID: ")
		if !in.Scan() {
			return "", fmt.Errorf("no client ID entered")
		}
		id := strings.TrimSpace(in.Text())
		switch {
		case id == "":
			continue
		case config.ValidClientID(id):
			return id, nil
		case strings.HasPrefix(id, "A"):
			fmt.Println("  ✗ that looks like the App ID — the Client ID is two number runs, e.g. 1234567890.9876543210")
		default:
			fmt.Println("  ✗ doesn't look like a Client ID (expected e.g. 1234567890.9876543210)")
		}
	}
	return "", fmt.Errorf("no valid client ID after several attempts")
}

// writeManifestFallback drops the manifest next to the config so a user without
// a working clipboard still has one file to open and copy.
func writeManifestFallback() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "slack-app-manifest.json")
	return path, os.WriteFile(path, []byte(manifest), 0o644)
}

// openBrowser mirrors auth's opener; setup only needs the landing page.
func openBrowser(u string) error { return auth.OpenBrowser(u) }
