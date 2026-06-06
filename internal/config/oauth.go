package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// OAuthCreds are the Slack app credentials needed for the browser sign-in flow
// (from the app's Basic Information page). They're separate from the issued
// tokens — these identify the app, the tokens authenticate the user.
type OAuthCreds struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func (c OAuthCreds) Ready() bool { return c.ClientID != "" && c.ClientSecret != "" }

func oauthCredsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "slack-tui", "oauth.json"), nil
}

// LoadOAuthCreds reads app credentials from oauth.json, with env vars
// (SLACK_CLIENT_ID / SLACK_CLIENT_SECRET) overriding.
func LoadOAuthCreds() OAuthCreds {
	var c OAuthCreds
	if path, err := oauthCredsPath(); err == nil {
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &c)
		}
	}
	if v := os.Getenv("SLACK_CLIENT_ID"); v != "" {
		c.ClientID = v
	}
	if v := os.Getenv("SLACK_CLIENT_SECRET"); v != "" {
		c.ClientSecret = v
	}
	return c
}

// SaveOAuthCreds persists app credentials (0600).
func SaveOAuthCreds(c OAuthCreds) error {
	path, err := oauthCredsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
