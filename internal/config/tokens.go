package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Tokens holds the Slack credentials, persisted separately from prefs with
// restrictive permissions. User is required for real Slack; App + Bot enable
// Socket Mode (live channel unread).
type Tokens struct {
	User string `json:"user"`
	App  string `json:"app"`
	Bot  string `json:"bot"`
}

// TokensPath returns the tokens file location (~/.config/slack-tui/tokens.json).
func TokensPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "slack-tui", "tokens.json"), nil
}

// LoadTokens reads the saved tokens (zero value if none).
func LoadTokens() (Tokens, error) {
	var t Tokens
	path, err := TokensPath()
	if err != nil {
		return t, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return t, err
	}
	return t, json.Unmarshal(b, &t)
}

// SaveTokens writes the tokens file with owner-only permissions (0600).
func SaveTokens(t Tokens) error {
	path, err := TokensPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// Resolve returns the effective tokens: an environment variable overrides the
// stored value per-token (env for dev/override, file for persistence).
func (t Tokens) Resolve() Tokens {
	if v := os.Getenv("SLACK_USER_TOKEN"); v != "" {
		t.User = v
	}
	if v := os.Getenv("SLACK_APP_TOKEN"); v != "" {
		t.App = v
	}
	if v := os.Getenv("SLACK_BOT_TOKEN"); v != "" {
		t.Bot = v
	}
	return t
}
