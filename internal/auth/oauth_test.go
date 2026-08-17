package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kurenn/slack-tui/internal/config"
)

func TestAuthorizeURL(t *testing.T) {
	u := AuthorizeURL("CID", "STATE", "", "http://localhost:9899/callback")
	for _, want := range []string{"client_id=CID", "state=STATE", "user_scope=", "redirect_uri=", "users.profile%3Awrite"} {
		if !strings.Contains(u, want) {
			t.Errorf("authorize URL missing %q:\n%s", want, u)
		}
	}
	if !strings.Contains(u, "scope=channels") {
		t.Errorf("confidential flow should request bot scopes:\n%s", u)
	}
	if strings.Contains(u, "code_challenge") {
		t.Errorf("no challenge given, but URL carries PKCE params:\n%s", u)
	}
}

// PKCE mode must add the challenge and drop the bot scopes — Slack rejects bot
// scopes on a desktop (loopback) redirect once PKCE is on.
func TestAuthorizeURLPKCE(t *testing.T) {
	u := AuthorizeURL("CID", "STATE", "CHAL", "http://localhost:9899/callback")
	q, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	v := q.Query()
	if v.Get("code_challenge") != "CHAL" {
		t.Errorf("code_challenge = %q, want CHAL", v.Get("code_challenge"))
	}
	if v.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", v.Get("code_challenge_method"))
	}
	if v.Has("scope") {
		t.Errorf("PKCE flow must not request bot scopes, got %q", v.Get("scope"))
	}
	if v.Get("user_scope") == "" {
		t.Error("PKCE flow still needs user scopes")
	}
}

func TestVerifierAndChallenge(t *testing.T) {
	v, err := verifier()
	if err != nil {
		t.Fatal(err)
	}
	// RFC 7636: 43–128 chars, unreserved set only (base64url without padding).
	if len(v) < 43 || len(v) > 128 {
		t.Errorf("verifier length %d, want 43–128", len(v))
	}
	if strings.ContainsAny(v, "+/=") {
		t.Errorf("verifier %q is not URL-safe base64", v)
	}
	sum := sha256.Sum256([]byte(v))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); challengeFor(v) != want {
		t.Errorf("challengeFor = %q, want %q", challengeFor(v), want)
	}
	other, err := verifier()
	if err != nil {
		t.Fatal(err)
	}
	if other == v {
		t.Error("verifier is not random")
	}
}

// The two flows are mutually exclusive on the wire: Slack rejects a call that
// carries both a client secret and a verifier.
func TestExchangeForm(t *testing.T) {
	creds := config.OAuthCreds{ClientID: "CID", ClientSecret: "SECRET"}
	for _, tc := range []struct {
		name, verifier string
		wantSet        string
		wantUnset      string
	}{
		{"pkce", "VERIF", "code_verifier", "client_secret"},
		{"confidential", "", "client_secret", "code_verifier"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := exchangeForm(creds, "CODE", tc.verifier, "http://localhost:9899/callback")
			if form.Get(tc.wantSet) == "" {
				t.Errorf("%s not set: %v", tc.wantSet, form)
			}
			if form.Has(tc.wantUnset) {
				t.Errorf("%s must not be sent: %v", tc.wantUnset, form)
			}
			if form.Get("code") != "CODE" || form.Get("client_id") != "CID" {
				t.Errorf("form = %v", form)
			}
		})
	}
}

func TestParseAccess(t *testing.T) {
	toks, team, err := parseAccess(strings.NewReader(
		`{"ok":true,"access_token":"xoxb-bot","team":{"id":"T1","name":"Acme"},"authed_user":{"access_token":"xoxp-user"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if toks.User != "xoxp-user" || toks.Bot != "xoxb-bot" {
		t.Errorf("got %+v", toks)
	}
	if toks.Rotating() {
		t.Errorf("plain grant should not look rotating: %+v", toks)
	}
	if team.ID != "T1" || team.Name != "Acme" {
		t.Errorf("team = %+v, want T1/Acme", team)
	}
	if _, _, err := parseAccess(strings.NewReader(`{"ok":false,"error":"invalid_code"}`)); err == nil {
		t.Error("expected error when ok:false")
	}
	if _, _, err := parseAccess(strings.NewReader(`{"ok":true,"access_token":"xoxb"}`)); err == nil {
		t.Error("expected error when no user token granted")
	}
}

// A rotating grant must be recorded, so login can warn instead of letting the
// token quietly expire later.
func TestParseAccessRotating(t *testing.T) {
	now = func() time.Time { return time.Unix(1_000, 0) }
	defer func() { now = time.Now }()

	toks, _, err := parseAccess(strings.NewReader(
		`{"ok":true,"team":{"id":"T1"},"authed_user":{"access_token":"xoxe-user","refresh_token":"xoxe-1-refresh","expires_in":43200}}`))
	if err != nil {
		t.Fatal(err)
	}
	if toks.Refresh != "xoxe-1-refresh" {
		t.Errorf("refresh = %q", toks.Refresh)
	}
	if toks.ExpiresAt != 44_200 {
		t.Errorf("expiresAt = %d, want 44200", toks.ExpiresAt)
	}
	if !toks.Rotating() {
		t.Error("Rotating() should be true for a refreshable grant")
	}
}

// Every port slack-tui may bind has to be registered in the app manifests: a
// sign-in that falls through to a later port would otherwise die at Slack's
// redirect check, and only for the user whose 9899 happened to be busy.
func TestRedirectURIsMatchManifest(t *testing.T) {
	want := RedirectURIs()

	b, err := os.ReadFile(filepath.Join("..", "..", "slack-app-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		OAuthConfig struct {
			RedirectURLs []string `json:"redirect_urls"`
		} `json:"oauth_config"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	got := manifest.OAuthConfig.RedirectURLs
	if len(got) != len(want) {
		t.Fatalf("manifest lists %d redirect URLs, code uses %d:\n got %v\nwant %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("redirect URL %d: manifest %q, code %q", i, got[i], want[i])
		}
	}

	// The YAML manifest is the copy most people paste; check it by text rather
	// than taking on a YAML dependency for one assertion.
	y, err := os.ReadFile(filepath.Join("..", "..", "slack-app-manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range want {
		if !strings.Contains(string(y), u) {
			t.Errorf("slack-app-manifest.yaml is missing %s", u)
		}
	}
}

// A busy port must roll to the next one instead of failing the sign-in.
func TestListenLoopbackSkipsBusyPort(t *testing.T) {
	busy, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", portFirst))
	if err != nil {
		t.Skipf("port %d already in use by something else: %v", portFirst, err)
	}
	defer busy.Close()

	ln, redirect, err := listenLoopback()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if want := redirectURI(portFirst + 1); redirect != want {
		t.Errorf("redirect = %q, want %q", redirect, want)
	}
	// The URI must describe the socket actually bound — Slack compares the
	// redirect_uri at authorize time with the one at exchange time.
	if _, port, _ := net.SplitHostPort(ln.Addr().String()); !strings.Contains(redirect, port) {
		t.Errorf("redirect %q does not match bound port %s", redirect, port)
	}
}
