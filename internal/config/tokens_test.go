package config

import (
	"os"
	"testing"
)

func TestTokensResolveEnvOverridesFile(t *testing.T) {
	t.Setenv("SLACK_USER_TOKEN", "xoxp-env")
	t.Setenv("SLACK_APP_TOKEN", "")
	t.Setenv("SLACK_BOT_TOKEN", "")
	got := Tokens{User: "xoxp-file", App: "xapp-file", Bot: "xoxb-file"}.Resolve()
	if got.User != "xoxp-env" {
		t.Errorf("env should override user token, got %q", got.User)
	}
	if got.App != "xapp-file" {
		t.Errorf("file app token should remain, got %q", got.App)
	}
}

func TestTokensRoundTripAndPerms(t *testing.T) {
	isolateConfigDir(t)
	want := Tokens{User: "u", App: "a", Bot: "b"}
	if err := SaveTokens(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTokens()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
	path, _ := TokensPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("tokens file perms = %o, want 600", perm)
	}
}

// isolateConfigDir points config.Dir() at a temp dir for the duration of a test.
// Overriding only HOME is not enough: config.Dir() checks XDG_CONFIG_HOME first,
// so on any desktop that sets it (most Linux distros) the test would read and
// write the real ~/.config/slack-tui.
func isolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// A refresh must not disturb the Socket Mode tokens: they are pasted by hand,
// and OAuth never reissues them, so clobbering them would silently kill live
// unread on every token rotation.
func TestSaveRefreshedKeepsSocketTokens(t *testing.T) {
	isolateConfigDir(t)
	if err := SaveWorkspace(Workspace{Name: "acme", TeamID: "T1", Tokens: Tokens{
		User: "old", Refresh: "old-r", ExpiresAt: 10, App: "xapp-keep", Bot: "xoxb-keep",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveRefreshed(Tokens{User: "new", Refresh: "new-r", ExpiresAt: 99}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTokens()
	if err != nil {
		t.Fatal(err)
	}
	if got.User != "new" || got.Refresh != "new-r" || got.ExpiresAt != 99 {
		t.Errorf("rotating creds not updated: %+v", got)
	}
	if got.App != "xapp-keep" || got.Bot != "xoxb-keep" {
		t.Errorf("socket tokens must survive a refresh: %+v", got)
	}
}
