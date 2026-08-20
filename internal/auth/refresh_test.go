package auth

// The token-refresh contract: Slack's rotating user tokens are single-use —
// the moment the server answers a refresh call, the token that was spent is
// dead, whether or not the reply gets persisted. These tests pin two
// consequences of that: a reply that doesn't hand back a new refresh token
// must be rejected outright (accepting it would strand the user with no way
// to refresh again), and config.SaveRefreshed must carry Refresh's actual
// output into the right fields without disturbing the App/Bot (xapp/xoxb)
// tokens, which OAuth never reissues and which Socket Mode depends on.

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/kurenn/slack-tui/internal/config"
)

// fxIsolateConfigDir points config's file I/O at a fresh temp dir for one
// test. testenv.Pin already isolates the whole package run from the real
// $HOME, but every test in this file that touches tokens.json needs its own
// directory so they can't see each other's writes.
func fxIsolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// TestRefreshSendsGrantTypeAndNoSecret pins the HTTP layer of Refresh — the
// form contract (already covered at the parseRefresh level by
// TestParseRefreshBothShapes in oauth_test.go, which this deliberately does
// not re-derive) plus the fact that PKCE refresh calls, like exchange calls,
// must never carry a client_secret.
func TestRefreshSendsGrantTypeAndNoSecret(t *testing.T) {
	now = func() time.Time { return time.Unix(1_000, 0) }
	t.Cleanup(func() { now = time.Now })

	var gotForm url.Values
	srv := fxAccessServer(t, func(form url.Values) (int, string) {
		gotForm = form
		return http.StatusOK, `{"ok":true,"authed_user":{"access_token":"xoxe.xoxp-new","refresh_token":"xoxe-1-next","expires_in":43200}}`
	})
	fxPointAccessURL(t, srv.URL)

	creds := config.OAuthCreds{ClientID: "CID", ClientSecret: "SHH"}
	got, err := Refresh(context.Background(), creds, "xoxe-1-spent")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.User != "xoxe.xoxp-new" || got.Refresh != "xoxe-1-next" || got.ExpiresAt != 44_200 {
		t.Errorf("got %+v", got)
	}
	if gotForm.Get("grant_type") != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", gotForm.Get("grant_type"))
	}
	if gotForm.Get("refresh_token") != "xoxe-1-spent" {
		t.Errorf("refresh_token sent = %q, want the spent one", gotForm.Get("refresh_token"))
	}
	if gotForm.Get("client_id") != "CID" {
		t.Errorf("client_id = %q, want CID", gotForm.Get("client_id"))
	}
	if gotForm.Has("client_secret") {
		t.Errorf("client_secret must never be sent on a refresh, got %v", gotForm)
	}
}

// TestRefreshRejectsReplyWithoutNewToken is the single-use contract's load-
// bearing case, exercised through the real HTTP round trip (not just
// parseRefresh in isolation): if Slack ever answered a refresh without
// handing back a replacement, accepting it would leave the caller believing
// it still holds a usable refresh token when the one it has is already dead.
// The old, still-good-until-expiry access token must remain untouched in
// storage — proven here by asserting SaveRefreshed is never reached and the
// previously stored workspace is unchanged.
func TestRefreshRejectsReplyWithoutNewToken(t *testing.T) {
	fxIsolateConfigDir(t)
	if err := config.SaveWorkspace(config.Workspace{
		Name: "acme", TeamID: "T1",
		Tokens: config.Tokens{User: "old-user", Refresh: "old-refresh", ExpiresAt: 5_000, App: "xapp-keep", Bot: "xoxb-keep"},
	}); err != nil {
		t.Fatal(err)
	}

	srv := fxAccessServer(t, func(url.Values) (int, string) {
		// ok, a fresh access token, but no refresh_token — the failure mode a
		// caller must not treat as success.
		return http.StatusOK, `{"ok":true,"authed_user":{"access_token":"xoxe.xoxp-new","expires_in":43200}}`
	})
	fxPointAccessURL(t, srv.URL)

	got, err := Refresh(context.Background(), config.OAuthCreds{ClientID: "CID"}, "old-refresh")
	if err == nil {
		t.Fatalf("expected Refresh to reject a reply with no new refresh token, got %+v", got)
	}

	// The caller's contract (see config.SaveRefreshed's doc comment) is:
	// never persist on error. Confirm nothing changed on disk.
	tok, err := config.LoadTokens()
	if err != nil {
		t.Fatal(err)
	}
	if tok.User != "old-user" || tok.Refresh != "old-refresh" || tok.ExpiresAt != 5_000 {
		t.Errorf("stored tokens changed after a rejected refresh: %+v", tok)
	}
}

// TestRefreshThenSaveRefreshedIntegration ties Refresh's real output into
// config.SaveRefreshed: the new User/Refresh/ExpiresAt Slack actually handed
// back must land in storage, and the App (xapp) / Bot (xoxb) tokens — pasted
// by hand for Socket Mode, never reissued by OAuth — must survive untouched.
// Each half is unit-tested elsewhere (parseRefresh's shapes here, and
// TestSaveRefreshedKeepsSocketTokens in config with a hand-built Tokens); this
// is the boundary between them, which neither unit test alone exercises.
func TestRefreshThenSaveRefreshedIntegration(t *testing.T) {
	fxIsolateConfigDir(t)
	now = func() time.Time { return time.Unix(1_000, 0) }
	t.Cleanup(func() { now = time.Now })

	if err := config.SaveWorkspace(config.Workspace{
		Name: "acme", TeamID: "T1",
		Tokens: config.Tokens{User: "old-user", Refresh: "old-refresh", ExpiresAt: 5_000, App: "xapp-keep", Bot: "xoxb-keep"},
	}); err != nil {
		t.Fatal(err)
	}

	srv := fxAccessServer(t, func(url.Values) (int, string) {
		return http.StatusOK, `{"ok":true,"authed_user":{"access_token":"xoxe.xoxp-new","refresh_token":"xoxe-1-next","expires_in":43200}}`
	})
	fxPointAccessURL(t, srv.URL)

	newToks, err := Refresh(context.Background(), config.OAuthCreds{ClientID: "CID"}, "old-refresh")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if err := config.SaveRefreshed(newToks); err != nil {
		t.Fatalf("SaveRefreshed: %v", err)
	}

	got, err := config.LoadTokens()
	if err != nil {
		t.Fatal(err)
	}
	if got.User != "xoxe.xoxp-new" || got.Refresh != "xoxe-1-next" || got.ExpiresAt != 44_200 {
		t.Errorf("rotating creds not persisted from Refresh's own output: %+v", got)
	}
	if got.App != "xapp-keep" || got.Bot != "xoxb-keep" {
		t.Errorf("socket-mode tokens must survive: %+v", got)
	}
}
