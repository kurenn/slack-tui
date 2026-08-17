package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kurenn/slack-tui/internal/config"
)

func TestAuthorizeURL(t *testing.T) {
	u := AuthorizeURL("CID", "STATE", "")
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
	u := AuthorizeURL("CID", "STATE", "CHAL")
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
			form := exchangeForm(creds, "CODE", tc.verifier)
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
		`{"ok":true,"access_token":"xoxb-bot","team":{"id":"T1","name":"Coba"},"authed_user":{"access_token":"xoxp-user"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if toks.User != "xoxp-user" || toks.Bot != "xoxb-bot" {
		t.Errorf("got %+v", toks)
	}
	if toks.Rotating() {
		t.Errorf("plain grant should not look rotating: %+v", toks)
	}
	if team.ID != "T1" || team.Name != "Coba" {
		t.Errorf("team = %+v, want T1/Coba", team)
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
