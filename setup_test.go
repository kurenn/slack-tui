package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurenn/slack-tui/internal/auth"
	"github.com/kurenn/slack-tui/internal/config"
)

func TestValidClientID(t *testing.T) {
	for _, ok := range []string{"1234567890.9876543210", "2707126420452.11291972393302"} {
		if !config.ValidClientID(ok) {
			t.Errorf("%q should be a valid client ID", ok)
		}
	}
	// The App ID and the signing secret are the two things people paste by
	// mistake; both must be rejected before we write them to oauth.json.
	for _, bad := range []string{
		"A0B8KULBK8W", "", "1234567890", "1234567890.", ".9876543210",
		"xoxp-123", "abcdef0123456789abcdef0123456789",
	} {
		if config.ValidClientID(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

// The manifest is embedded so a freshly created app always matches the binary
// that told the user to create it — above all on redirect URLs, since a missing
// port only breaks for the user whose 9899 was busy.
func TestEmbeddedManifestRedirectsMatchCode(t *testing.T) {
	var m struct {
		OAuthConfig struct {
			RedirectURLs []string `json:"redirect_urls"`
		} `json:"oauth_config"`
	}
	if err := json.Unmarshal([]byte(manifest), &m); err != nil {
		t.Fatalf("embedded manifest is not valid JSON: %v", err)
	}
	want := auth.RedirectURIs()
	if len(m.OAuthConfig.RedirectURLs) != len(want) {
		t.Fatalf("manifest has %d redirect URLs, code uses %d", len(m.OAuthConfig.RedirectURLs), len(want))
	}
	for i := range want {
		if m.OAuthConfig.RedirectURLs[i] != want[i] {
			t.Errorf("redirect %d: manifest %q, code %q", i, m.OAuthConfig.RedirectURLs[i], want[i])
		}
	}
}

// A bad first paste (the App ID, not the Client ID) must not fail the whole
// flow — it should re-prompt, print the specific "looks like the App ID"
// hint, and accept the next valid line.
func TestPromptClientIDRetriesAfterAppID(t *testing.T) {
	in := strings.NewReader("A0B8KULBK8W\n1234567890.9876543210\n")
	out := smCaptureStdout(t, func() {
		id, err := promptClientID(in)
		if err != nil {
			t.Fatalf("promptClientID returned an error: %v", err)
		}
		if id != "1234567890.9876543210" {
			t.Errorf("promptClientID = %q, want the second (valid) line", id)
		}
	})
	if !strings.Contains(out, "looks like the App ID") {
		t.Errorf("expected the App-ID-specific hint after a bad first paste, got:\n%s", out)
	}
}

// Five garbage lines in a row must give up rather than looping forever or
// hanging on stdin.
func TestPromptClientIDGivesUpAfterFiveBadAttempts(t *testing.T) {
	in := strings.NewReader("nope\nstill no\nxoxp-123\nabc\ndef\n")
	_, err := promptClientID(in)
	if err == nil {
		t.Fatal("expected an error after five invalid attempts, got nil")
	}
}

// EOF (an empty/closed reader) must fail immediately, not hang scanning for
// a line that will never come.
func TestPromptClientIDFailsOnEOF(t *testing.T) {
	_, err := promptClientID(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected an error on immediate EOF, got nil")
	}
}

// The fallback manifest file must land in the isolated config dir with
// byte-identical content to the embedded manifest — it's the thing a user
// without clipboard access is told to open and paste from.
func TestWriteManifestFallback(t *testing.T) {
	smIsolateConfigDir(t)
	path, err := writeManifestFallback()
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := config.Dir()
	if filepath.Dir(path) != dir {
		t.Errorf("manifest written to %q, want inside config dir %q", path, dir)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != manifest {
		t.Error("written manifest content does not match the embedded manifest byte-for-byte")
	}
}
