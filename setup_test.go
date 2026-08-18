package main

import (
	"encoding/json"
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
