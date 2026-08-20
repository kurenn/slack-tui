package config

import (
	"os"
	"path/filepath"
	"testing"
)

// fxWriteTokensFile writes a raw tokens.json under an isolated config dir —
// the on-disk shape a real (possibly older) install would have, not one this
// test constructs by calling the save functions it's trying to test.
func fxWriteTokensFile(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	sub := filepath.Join(dir, "slack-tui")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "tokens.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLegacyMigrationDoesNotDoubleRunWhenWorkspacesExist: a file that somehow
// carries both a "workspaces" list and leftover top-level legacy fields (e.g.
// hand-edited, or written by an in-between version) must not manufacture a
// second, spurious workspace from the stale top-level fields — only the
// zero-workspaces case triggers migration.
func TestLegacyMigrationDoesNotDoubleRunWhenWorkspacesExist(t *testing.T) {
	fxWriteTokensFile(t, `{
		"workspaces": [{"name": "real", "team_id": "T1", "user": "xoxp-real"}],
		"active": "real",
		"user": "xoxp-stale-leftover"
	}`)
	all, active, err := LoadWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("stale top-level fields spawned an extra workspace: %+v", all)
	}
	if active != "real" || all[0].User != "xoxp-real" {
		t.Errorf("got active=%q workspaces=%+v, want the real one untouched", active, all)
	}
}

// TestLegacyMigrationOnlyUser: a pre-v0.3.0 file might have only a user token
// (no app/bot) — migration must not require all three legacy fields.
func TestLegacyMigrationOnlyUser(t *testing.T) {
	fxWriteTokensFile(t, `{"user": "xoxp-solo"}`)
	tok, err := LoadTokens()
	if err != nil {
		t.Fatal(err)
	}
	if tok.User != "xoxp-solo" || tok.App != "" || tok.Bot != "" {
		t.Errorf("got %+v, want only User populated", tok)
	}
}

// TestLoadTokensNoFile: a brand new install has no tokens.json at all.
// LoadTokens must surface that as an error (not panic, not a fabricated
// zero-value success) since the caller (onboarding routing) distinguishes
// "no tokens yet" from "tokens present but empty".
func TestLoadTokensNoFile(t *testing.T) {
	fxIsolateConfigDir(t)
	if _, err := LoadTokens(); err == nil {
		t.Error("expected an error reading tokens.json when none exists")
	}
	if _, _, err := LoadWorkspaces(); err == nil {
		t.Error("expected an error from LoadWorkspaces when none exists")
	}
}

// TestLoadWorkspacesEmptyList: a tokens.json that parses fine but lists zero
// workspaces (e.g. every workspace was removed) must report "none active"
// rather than indexing into an empty slice.
func TestLoadWorkspacesEmptyList(t *testing.T) {
	fxWriteTokensFile(t, `{"workspaces": [], "active": ""}`)
	all, active, err := LoadWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	if all != nil || active != "" {
		t.Errorf("got workspaces=%+v active=%q, want nil/empty", all, active)
	}
	if tok, err := LoadTokens(); err != nil || tok != (Tokens{}) {
		t.Errorf("LoadTokens with zero workspaces = %+v, %v, want zero value, nil", tok, err)
	}
}

// TestSaveTokensUpdatesExistingActiveWorkspace: SaveTokens's other branch —
// once a workspace exists, a second call must update it in place rather than
// appending a duplicate "default" entry.
func TestSaveTokensUpdatesExistingActiveWorkspace(t *testing.T) {
	fxIsolateConfigDir(t)
	if err := SaveTokens(Tokens{User: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveTokens(Tokens{User: "second"}); err != nil {
		t.Fatal(err)
	}
	all, _, err := LoadWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("second SaveTokens should update in place, got %d workspaces: %+v", len(all), all)
	}
	if all[0].User != "second" {
		t.Errorf("User = %q, want the second save's value", all[0].User)
	}
}

// TestSaveWorkspaceUpsertByNameWhenNoTeamID: the upsert match is "team id, or
// else name" — a workspace saved without a team id (can happen if Team.ID
// were ever empty) must still be found and updated by name on a second save,
// not duplicated.
func TestSaveWorkspaceUpsertByNameWhenNoTeamID(t *testing.T) {
	fxIsolateConfigDir(t)
	if err := SaveWorkspace(Workspace{Name: "solo", Tokens: Tokens{User: "u1"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorkspace(Workspace{Name: "solo", Tokens: Tokens{User: "u2"}}); err != nil {
		t.Fatal(err)
	}
	all, _, err := LoadWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("same-name save with no team id should match by name, got %+v", all)
	}
	if all[0].User != "u2" {
		t.Errorf("User = %q, want u2 (the second save)", all[0].User)
	}
}

// TestSaveRefreshedNoActiveWorkspace: refreshing before any workspace has
// ever been saved must error, not silently create one with half-populated
// fields (App/Bot would be empty forever, quietly breaking Socket Mode for a
// workspace that was never properly signed in).
func TestSaveRefreshedNoActiveWorkspace(t *testing.T) {
	fxIsolateConfigDir(t)
	err := SaveRefreshed(Tokens{User: "new", Refresh: "r", ExpiresAt: 1})
	if err == nil {
		t.Error("expected an error refreshing with no active workspace")
	}
}

// TestTokensResolveFullTable: every one of the three env overrides is
// independent — setting one must not touch the other two, and with nothing
// set the file's values pass through unchanged.
func TestTokensResolveFullTable(t *testing.T) {
	file := Tokens{User: "xoxp-file", App: "xapp-file", Bot: "xoxb-file"}

	t.Run("no env sees the file values", func(t *testing.T) {
		t.Setenv("SLACK_USER_TOKEN", "")
		t.Setenv("SLACK_APP_TOKEN", "")
		t.Setenv("SLACK_BOT_TOKEN", "")
		if got := file.Resolve(); got != file {
			t.Errorf("got %+v, want the file values untouched: %+v", got, file)
		}
	})

	t.Run("app token env overrides only app", func(t *testing.T) {
		t.Setenv("SLACK_USER_TOKEN", "")
		t.Setenv("SLACK_APP_TOKEN", "xapp-env")
		t.Setenv("SLACK_BOT_TOKEN", "")
		got := file.Resolve()
		if got.App != "xapp-env" || got.User != "xoxp-file" || got.Bot != "xoxb-file" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("bot token env overrides only bot", func(t *testing.T) {
		t.Setenv("SLACK_USER_TOKEN", "")
		t.Setenv("SLACK_APP_TOKEN", "")
		t.Setenv("SLACK_BOT_TOKEN", "xoxb-env")
		got := file.Resolve()
		if got.Bot != "xoxb-env" || got.User != "xoxp-file" || got.App != "xapp-file" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("all three override together", func(t *testing.T) {
		t.Setenv("SLACK_USER_TOKEN", "xoxp-env")
		t.Setenv("SLACK_APP_TOKEN", "xapp-env")
		t.Setenv("SLACK_BOT_TOKEN", "xoxb-env")
		got := file.Resolve()
		want := Tokens{User: "xoxp-env", App: "xapp-env", Bot: "xoxb-env"}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
}

// TestTokensPath pins the file location, mirroring TestPathUnderIsolatedDir
// for prefs.json.
func TestTokensPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := TokensPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "slack-tui", "tokens.json")
	if got != want {
		t.Errorf("TokensPath() = %q, want %q", got, want)
	}
}

// TestTokensRotating is the boundary case called out in the coverage plan:
// Rotating() is real boolean logic (Refresh != "" || ExpiresAt > 0), not an
// echo, so its truth table is a legitimate, independently meaningful test.
func TestTokensRotating(t *testing.T) {
	for name, tc := range map[string]struct {
		tok  Tokens
		want bool
	}{
		"neither":      {Tokens{User: "u"}, false},
		"refresh only": {Tokens{User: "u", Refresh: "r"}, true},
		"expiry only":  {Tokens{User: "u", ExpiresAt: 100}, true},
		"both":         {Tokens{User: "u", Refresh: "r", ExpiresAt: 100}, true},
		"zero expiry":  {Tokens{User: "u", ExpiresAt: 0}, false},
	} {
		if got := tc.tok.Rotating(); got != tc.want {
			t.Errorf("%s: Rotating() = %v, want %v (%+v)", name, got, tc.want, tc.tok)
		}
	}
}
