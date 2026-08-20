package app

import (
	"os"
	"testing"

	"github.com/kurenn/slack-tui/internal/testenv"
)

func TestMain(m *testing.M) {
	// The yank path writes the clipboard for real; capture it instead. Kept
	// here rather than in testenv because only this package has the seam.
	writeClipboard = func(s string) error { clipboardStub = s; return nil }
	os.Exit(testenv.Pin(m))
}

// clipboardStub holds whatever the last yank "copied", for tests to assert on.
var clipboardStub string
