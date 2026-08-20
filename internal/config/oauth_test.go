package config

import "testing"

// A stock install has no oauth.json and signs in against the built-in app —
// client ID only, so the flow runs as a PKCE public client.
func TestLoadOAuthCredsFallsBackToBuiltIn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SLACK_CLIENT_ID", "")
	t.Setenv("SLACK_CLIENT_SECRET", "")
	setDefaultClientID(t, "CBUILTIN")

	got := LoadOAuthCreds()
	if got.ClientID != "CBUILTIN" {
		t.Errorf("clientID = %q, want the built-in app", got.ClientID)
	}
	if !got.Ready() {
		t.Errorf("built-in app should be ready to sign in, got %+v", got)
	}
}

// Without a built-in ID (plain `go build`) there is nothing to sign in with, so
// onboarding must be able to tell and route to the token form.
func TestLoadOAuthCredsUnconfigured(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SLACK_CLIENT_ID", "")
	t.Setenv("SLACK_CLIENT_SECRET", "")
	setDefaultClientID(t, "")

	if got := LoadOAuthCreds(); got.Ready() {
		t.Errorf("expected not ready, got %+v", got)
	}
}

// A user's own app wins over the built-in one. The secret is still parsed (so a
// pre-existing oauth.json loads, and login can say it is now ignored) but it is
// never sent — see TestExchangeFormNeverSendsSecret.
func TestLoadOAuthCredsUserAppKeepsSecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SLACK_CLIENT_ID", "")
	t.Setenv("SLACK_CLIENT_SECRET", "")
	setDefaultClientID(t, "CBUILTIN")

	if err := SaveOAuthCreds(OAuthCreds{ClientID: "CMINE", ClientSecret: "SHH"}); err != nil {
		t.Fatal(err)
	}
	got := LoadOAuthCreds()
	if got.ClientID != "CMINE" || got.ClientSecret != "SHH" {
		t.Errorf("got %+v, want the user's own app", got)
	}
}

// Pointing at a different app via env must not pair that app with the stored
// secret — the mismatch would just fail at oauth.v2.access.
func TestLoadOAuthCredsEnvIDDropsStoredSecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SLACK_CLIENT_SECRET", "")
	setDefaultClientID(t, "CBUILTIN")

	if err := SaveOAuthCreds(OAuthCreds{ClientID: "CFILE", ClientSecret: "FILESECRET"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLACK_CLIENT_ID", "CENV")

	got := LoadOAuthCreds()
	if got.ClientID != "CENV" {
		t.Errorf("clientID = %q, want CENV", got.ClientID)
	}
	if got.ClientSecret != "" {
		t.Errorf("stored secret must not follow a different client ID, got %q", got.ClientSecret)
	}

	// An env secret is still read (so it can be reported as unused), never dropped
	// silently into a mismatched pairing.
	t.Setenv("SLACK_CLIENT_SECRET", "ENVSECRET")
	if got := LoadOAuthCreds(); got.ClientSecret != "ENVSECRET" {
		t.Errorf("got %+v, want the env secret read", got)
	}
}

// setDefaultClientID stands in for the build-time stamp, restoring it after.
func setDefaultClientID(t *testing.T, v string) {
	t.Helper()
	prev := DefaultClientID
	DefaultClientID = v
	t.Cleanup(func() { DefaultClientID = prev })
}
