package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Tokens holds one workspace's Slack credentials. User is required for real
// Slack; App + Bot enable Socket Mode (live channel unread).
//
// Refresh and ExpiresAt are set only when Slack hands back a rotating user
// token. Nothing refreshes them yet — they exist so a rotating token is
// recorded and reported rather than silently expiring mid-session. Rotation is
// off for the loopback + PKCE flow we use, so these stay empty in practice.
type Tokens struct {
	User      string `json:"user"`
	App       string `json:"app,omitempty"`
	Bot       string `json:"bot,omitempty"`
	Refresh   string `json:"refresh,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"` // unix seconds; 0 = never
}

// Rotating reports whether Slack issued this token with an expiry.
func (t Tokens) Rotating() bool { return t.Refresh != "" || t.ExpiresAt > 0 }

// Workspace is one signed-in workspace: a name to switch by, the Slack team
// id, and its credentials.
type Workspace struct {
	Name   string `json:"name"`
	TeamID string `json:"team_id,omitempty"`
	Tokens
}

// tokensFile is the on-disk shape. The legacy single-workspace fields are
// read for migration; writes always use the workspaces list.
type tokensFile struct {
	Workspaces []Workspace `json:"workspaces,omitempty"`
	Active     string      `json:"active,omitempty"`

	// pre-v0.3.0 single-workspace format
	User string `json:"user,omitempty"`
	App  string `json:"app,omitempty"`
	Bot  string `json:"bot,omitempty"`
}

// ActiveOverride selects a workspace for this process only (--workspace flag)
// without persisting the choice.
var ActiveOverride string

// TokensPath returns the tokens file location (~/.config/slack-tui/tokens.json).
func TokensPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tokens.json"), nil
}

// loadTokensFile reads and migrates the on-disk format: a legacy
// single-workspace file becomes a one-entry list named "default".
func loadTokensFile() (tokensFile, error) {
	var f tokensFile
	b, err := readConfigFile("tokens.json")
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, err
	}
	if len(f.Workspaces) == 0 && (f.User != "" || f.App != "" || f.Bot != "") {
		f.Workspaces = []Workspace{{Name: "default", Tokens: Tokens{User: f.User, App: f.App, Bot: f.Bot}}}
		f.Active = "default"
	}
	f.User, f.App, f.Bot = "", "", ""
	return f, nil
}

func saveTokensFile(f tokensFile) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeConfigFile("tokens.json", b, 0o600)
}

// activeIndex picks the effective workspace: ActiveOverride, then the saved
// active name, then the first entry. Returns -1 when there are none.
func activeIndex(f tokensFile) int {
	want := f.Active
	if ActiveOverride != "" {
		want = ActiveOverride
	}
	for i, w := range f.Workspaces {
		if w.Name == want {
			return i
		}
	}
	if len(f.Workspaces) > 0 {
		return 0
	}
	return -1
}

// LoadWorkspaces returns all signed-in workspaces and the active one's name.
func LoadWorkspaces() ([]Workspace, string, error) {
	f, err := loadTokensFile()
	if err != nil {
		return nil, "", err
	}
	i := activeIndex(f)
	if i < 0 {
		return nil, "", nil
	}
	return f.Workspaces, f.Workspaces[i].Name, nil
}

// LoadTokens returns the active workspace's tokens (zero value if none) —
// the single-workspace API every consumer already uses.
func LoadTokens() (Tokens, error) {
	f, err := loadTokensFile()
	if err != nil {
		return Tokens{}, err
	}
	if i := activeIndex(f); i >= 0 {
		return f.Workspaces[i].Tokens, nil
	}
	return Tokens{}, nil
}

// SaveTokens updates the active workspace's tokens (creating a "default"
// workspace when none exist) — keeps the onboarding token form working.
func SaveTokens(t Tokens) error {
	f, _ := loadTokensFile()
	if i := activeIndex(f); i >= 0 {
		f.Workspaces[i].Tokens = t
	} else {
		f.Workspaces = []Workspace{{Name: "default", Tokens: t}}
		f.Active = "default"
	}
	if f.Active == "" {
		f.Active = f.Workspaces[0].Name
	}
	return saveTokensFile(f)
}

// SaveWorkspace upserts a workspace (matched by team id, falling back to
// name) and makes it active — the login flow lands here.
func SaveWorkspace(w Workspace) error {
	f, _ := loadTokensFile()
	idx := -1
	for i, x := range f.Workspaces {
		if (w.TeamID != "" && x.TeamID == w.TeamID) || x.Name == w.Name {
			idx = i
			break
		}
	}
	if idx >= 0 {
		if w.App == "" { // OAuth never issues xapp tokens — keep the stored one
			w.App = f.Workspaces[idx].App
		}
		f.Workspaces[idx] = w
	} else {
		f.Workspaces = append(f.Workspaces, w)
	}
	f.Active = w.Name
	return saveTokensFile(f)
}

// SetActiveWorkspace persists which workspace the app opens.
func SetActiveWorkspace(name string) error {
	f, err := loadTokensFile()
	if err != nil {
		return err
	}
	for _, w := range f.Workspaces {
		if w.Name == name {
			f.Active = name
			return saveTokensFile(f)
		}
	}
	return fmt.Errorf("unknown workspace %q", name)
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
