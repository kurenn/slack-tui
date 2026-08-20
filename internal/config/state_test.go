package config

import (
	"os"
	"testing"
)

// TestStateRoundTrip: SaveState then LoadState must reproduce drafts, recent
// order, and hidden-with-baseline exactly — this is the only persistence for
// an unsent draft, so losing a field here loses the user's typed message.
func TestStateRoundTrip(t *testing.T) {
	fxIsolateConfigDir(t)
	want := State{
		Drafts: map[string]string{"C1": "hello there", "C2": ""},
		Recent: []string{"C2", "C1", "D1"},
		Hidden: map[string]int{"C3": 4},
	}
	if err := SaveState(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if got.Drafts["C1"] != want.Drafts["C1"] || len(got.Drafts) != len(want.Drafts) {
		t.Errorf("Drafts = %+v, want %+v", got.Drafts, want.Drafts)
	}
	if len(got.Recent) != 3 || got.Recent[0] != "C2" || got.Recent[2] != "D1" {
		t.Errorf("Recent = %+v, want order preserved %+v", got.Recent, want.Recent)
	}
	if got.Hidden["C3"] != 4 {
		t.Errorf("Hidden = %+v, want C3:4", got.Hidden)
	}
}

// TestStateFilePermissions: drafts are message text a user may not have sent
// yet, so the file must be private (0600), not world/group-readable like
// prefs.json.
func TestStateFilePermissions(t *testing.T) {
	fxIsolateConfigDir(t)
	if err := SaveState(State{Drafts: map[string]string{"C1": "secret draft"}}); err != nil {
		t.Fatal(err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("state.json perms = %o, want 600", perm)
	}
}

// TestLoadStateMissingFile: a fresh install has no state.json — LoadState
// must hand back the zero value and an error, not panic, so the app can
// treat "no saved session" the same way it treats a corrupt one.
func TestLoadStateMissingFile(t *testing.T) {
	fxIsolateConfigDir(t)
	got, err := LoadState()
	if err == nil {
		t.Error("expected an error when state.json doesn't exist")
	}
	if got.Drafts != nil || got.Recent != nil || got.Hidden != nil {
		t.Errorf("got %+v, want the zero value", got)
	}
}

// TestSetActiveWorkspaceNoTokensFile: switching workspaces before any
// tokens.json exists must error rather than silently doing nothing — the
// caller (msgactions workspace switch) needs to know the switch didn't
// happen.
func TestSetActiveWorkspaceNoTokensFile(t *testing.T) {
	fxIsolateConfigDir(t)
	if err := SetActiveWorkspace("acme"); err == nil {
		t.Error("expected an error switching workspace with no tokens.json")
	}
}
