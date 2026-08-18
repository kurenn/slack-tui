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

// Slack rejects a loopback authorization outright without a challenge ("Must
// use PKCE to redirect to a non-web URI") and again if bot scopes are present
// ("Bot scopes are not allowed when redirecting to a non-web URI"). Both were
// confirmed against a live app, so both are asserted here.
func TestAuthorizeURL(t *testing.T) {
	u := AuthorizeURL("CID", "STATE", "CHAL", "http://localhost:9899/callback")
	q, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	v := q.Query()
	for _, want := range []string{"client_id", "state", "user_scope", "redirect_uri"} {
		if v.Get(want) == "" {
			t.Errorf("authorize URL missing %s:\n%s", want, u)
		}
	}
	if !strings.Contains(v.Get("user_scope"), "users.profile:write") {
		t.Errorf("user_scope = %q", v.Get("user_scope"))
	}
	if v.Get("code_challenge") != "CHAL" {
		t.Errorf("code_challenge = %q, want CHAL", v.Get("code_challenge"))
	}
	if v.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", v.Get("code_challenge_method"))
	}
	if v.Has("scope") {
		t.Errorf("bot scopes are never allowed on a loopback redirect, got %q", v.Get("scope"))
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

// A PKCE client must omit client_secret — Slack rejects a call carrying both.
// Even a configured secret (a leftover oauth.json) must never reach the wire.
func TestExchangeFormNeverSendsSecret(t *testing.T) {
	creds := config.OAuthCreds{ClientID: "CID", ClientSecret: "SECRET"}
	form := exchangeForm(creds, "CODE", "VERIF", "http://localhost:9899/callback")
	if form.Has("client_secret") {
		t.Errorf("client_secret must never be sent: %v", form)
	}
	if form.Get("code_verifier") != "VERIF" {
		t.Errorf("code_verifier = %q", form.Get("code_verifier"))
	}
	if form.Get("code") != "CODE" || form.Get("client_id") != "CID" {
		t.Errorf("form = %v", form)
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
