package config

import (
	"encoding/json"
	"os"
)

// DefaultClientID is the client ID of the published slack-tui Slack app, so a
// plain install can sign in with nothing configured. A client ID is public
// information and there is no secret to go with it — the shipped app uses PKCE,
// which is exactly the flow designed for binaries that can't keep a secret.
//
// Stamped at build time by goreleaser:
//
//	-X github.com/kurenn/slack-tui/internal/config.DefaultClientID=…
//
// Empty in `go build` / `go install` builds, which fall back to a user-supplied
// app in oauth.json — same as before this existed.
var DefaultClientID = ""

// OAuthCreds are the Slack app credentials needed for the browser sign-in flow
// (from the app's Basic Information page). They're separate from the issued
// tokens — these identify the app, the tokens authenticate the user.
//
// A secret is optional. Without one we run PKCE as a public client (the shipped
// path); with one we run the confidential flow, which is the only way to be
// granted bot scopes for Socket Mode.
type OAuthCreds struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// Ready reports whether a browser sign-in can be attempted at all.
func (c OAuthCreds) Ready() bool { return c.ClientID != "" }

// PKCE reports whether to run the public-client flow (no secret to send).
func (c OAuthCreds) PKCE() bool { return c.ClientSecret == "" }

// LoadOAuthCreds reads app credentials from oauth.json, falling back to the
// built-in app, with env vars (SLACK_CLIENT_ID / SLACK_CLIENT_SECRET)
// overriding. A user-supplied client ID drops the built-in one entirely rather
// than pairing it with a foreign secret.
func LoadOAuthCreds() OAuthCreds {
	var c OAuthCreds
	if b, err := readConfigFile("oauth.json"); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if v := os.Getenv("SLACK_CLIENT_ID"); v != "" {
		c.ClientID, c.ClientSecret = v, ""
	}
	if v := os.Getenv("SLACK_CLIENT_SECRET"); v != "" {
		c.ClientSecret = v
	}
	if c.ClientID == "" {
		return OAuthCreds{ClientID: DefaultClientID}
	}
	return c
}

// SaveOAuthCreds persists app credentials (0600).
func SaveOAuthCreds(c OAuthCreds) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeConfigFile("oauth.json", b, 0o600)
}
