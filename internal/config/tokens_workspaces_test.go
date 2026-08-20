package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTokensFile(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	sub := filepath.Join(dir, "slack-tui")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "tokens.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLegacyTokensMigrate: a pre-v0.3.0 single-workspace file reads as one
// "default" workspace, and the single-workspace API keeps working.
func TestLegacyTokensMigrate(t *testing.T) {
	writeTokensFile(t, `{"user":"xoxp-1","app":"xapp-1","bot":"xoxb-1"}`)
	all, active, err := LoadWorkspaces()
	if err != nil || len(all) != 1 || active != "default" {
		t.Fatalf("migration failed: %v %v %v", all, active, err)
	}
	tok, err := LoadTokens()
	if err != nil || tok.User != "xoxp-1" || tok.App != "xapp-1" || tok.Bot != "xoxb-1" {
		t.Fatalf("LoadTokens after migration = %+v (%v)", tok, err)
	}
}

// TestSaveWorkspaceUpsert: same team updates in place (keeping a stored xapp
// token), a new team appends; the saved workspace becomes active.
func TestSaveWorkspaceUpsert(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SaveWorkspace(Workspace{Name: "acme", TeamID: "T1", Tokens: Tokens{User: "u1", App: "xapp-1"}}); err != nil {
		t.Fatal(err)
	}
	// Re-login to the same team: no app token in the OAuth result — keep stored.
	if err := SaveWorkspace(Workspace{Name: "acme", TeamID: "T1", Tokens: Tokens{User: "u2"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorkspace(Workspace{Name: "personal", TeamID: "T2", Tokens: Tokens{User: "u3"}}); err != nil {
		t.Fatal(err)
	}
	all, active, _ := LoadWorkspaces()
	if len(all) != 2 {
		t.Fatalf("want 2 workspaces, got %+v", all)
	}
	if all[0].User != "u2" || all[0].App != "xapp-1" {
		t.Errorf("upsert should update tokens and keep the xapp token, got %+v", all[0])
	}
	if active != "personal" {
		t.Errorf("last saved workspace should be active, got %q", active)
	}
}

// TestSetActiveAndOverride: SetActiveWorkspace persists; ActiveOverride wins
// for the process without persisting.
func TestSetActiveAndOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_ = SaveWorkspace(Workspace{Name: "a", TeamID: "T1", Tokens: Tokens{User: "ua"}})
	_ = SaveWorkspace(Workspace{Name: "b", TeamID: "T2", Tokens: Tokens{User: "ub"}})
	if err := SetActiveWorkspace("a"); err != nil {
		t.Fatal(err)
	}
	if tok, _ := LoadTokens(); tok.User != "ua" {
		t.Errorf("active should be a, got %+v", tok)
	}
	ActiveOverride = "b"
	defer func() { ActiveOverride = "" }()
	if tok, _ := LoadTokens(); tok.User != "ub" {
		t.Errorf("override should win, got %+v", tok)
	}
	if _, active, _ := LoadWorkspaces(); active != "b" {
		t.Errorf("LoadWorkspaces should report the effective active, got %q", active)
	}
	if err := SetActiveWorkspace("nope"); err == nil {
		t.Error("unknown workspace should error")
	}
}
