// Package testenv isolates tests from the machine running them. It is imported
// only from _test files, so it never reaches a shipped binary.
//
// This exists because the same bug keeps recurring: a test reads or writes the
// developer's real environment, then passes on CI and fails locally (or worse,
// the reverse). It has happened four times —
//
//   - tests wrote the real ~/.config/slack-tui, because config.Dir() checks
//     XDG_CONFIG_HOME before HOME and only HOME was overridden
//   - the onboarding wizard dropped its colour steps on any Omarchy machine,
//     because theme lookup reads XDG_STATE_HOME
//   - onboarding skipped the auth screen on a machine that had signed in,
//     because New() reads the real tokens.json
//   - `go test ./internal/app/` overwrites the developer's clipboard with mock
//     message text, because the yank path calls clipboard.WriteAll directly
//
// Each was found by accident. Pin makes isolation the default instead.
package testenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Pin isolates the process and runs the tests. Call it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testenv.Pin(m)) }
//
// It redirects every path slack-tui reads config or theme data from, clears the
// credential env vars, fixes the timezone, empties PATH so nothing can shell out
// to a real notifier or clipboard tool, and forces a colour profile.
func Pin(m *testing.M) int {
	dir, err := os.MkdirTemp("", "slack-tui-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	os.Setenv("HOME", dir)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	// Credentials in the developer's shell would otherwise reach real Slack:
	// app.New() builds a live source from SLACK_USER_TOKEN and may spend the
	// single-use refresh token, which costs the developer a re-login.
	for _, v := range []string{
		"SLACK_USER_TOKEN", "SLACK_APP_TOKEN", "SLACK_BOT_TOKEN",
		"SLACK_CLIENT_ID", "SLACK_CLIENT_SECRET",
	} {
		os.Unsetenv(v)
	}

	// Timestamps render in local time, so an unpinned zone makes any assertion
	// on a formatted clock pass in one country and fail in another.
	os.Setenv("TZ", "UTC")

	// notify-send and wl-copy/pbcopy are found via PATH. Emptying it makes a
	// stray notification or clipboard write impossible rather than merely
	// unlikely. Code that needs those paths under test injects a stub instead.
	os.Setenv("PATH", filepath.Join(dir, "empty-path"))

	// go test has no TTY, so lipgloss defaults to the Ascii profile and strips
	// every escape sequence — which would make any assertion about colour pass
	// unconditionally, whatever the palette. Force truecolor so those tests can
	// actually fail.
	lipgloss.SetColorProfile(termenv.TrueColor)

	return m.Run()
}
