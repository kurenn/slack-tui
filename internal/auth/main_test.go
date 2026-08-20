package auth

import (
	"os"
	"testing"

	"github.com/kurenn/slack-tui/internal/testenv"
)

// TestMain pins HOME/XDG dirs, clears SLACK_*, fixes TZ and forces a color
// profile — the same isolation every other package in the repo runs under
// (see internal/testenv). auth's tests reach config (SaveWorkspace,
// SaveRefreshed, LoadTokens) to exercise the refresh↔persistence contract, so
// without this a test run could read or write the developer's real
// ~/.config/slack-tui. No TestMain previously existed in this package.
func TestMain(m *testing.M) { os.Exit(testenv.Pin(m)) }
