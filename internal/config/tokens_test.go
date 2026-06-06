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
	t.Setenv("HOME", t.TempDir())
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
