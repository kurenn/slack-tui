package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kurenn/slack-tui/internal/theme"
)

// fxIsolateConfigDir points config.Dir() at a fresh temp dir. Overriding only
// HOME is not enough — Dir() checks XDG_CONFIG_HOME first, so on any desktop
// that sets it the test would read/write the real ~/.config/slack-tui. Same
// pattern as isolateConfigDir in tokens_test.go, duplicated here (not
// imported) because it's an unexported same-package helper and this file
// already needs its own fx-prefixed helpers alongside it.
func fxIsolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// fxOmarchyFixture points the theme lookup at a fixture colors.toml, the way
// onboarding's withOmarchyTheme does (that helper lives in another package's
// _test.go, so it can't be imported — this is the same five-line pattern).
func fxOmarchyFixture(t *testing.T, colors string) {
	t.Helper()
	dir := t.TempDir()
	themeDir := filepath.Join(dir, "omarchy", "current", "theme")
	if err := os.MkdirAll(themeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themeDir, "colors.toml"), []byte(colors), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", dir)
	if !theme.OmarchyAvailable() {
		t.Fatal("fixture theme should be detected")
	}
}

// TestLoadNoFileReturnsDefaults: a fresh install (no prefs.json at all) must
// route to onboarding (ok=false) while still handing back a fully-populated
// default Prefs the app can render with meanwhile.
func TestLoadNoFileReturnsDefaults(t *testing.T) {
	fxIsolateConfigDir(t)
	prefs, ok := Load()
	if ok {
		t.Error("no prefs file should report ok=false")
	}
	want := Defaults()
	if prefs != want {
		t.Errorf("got %+v, want Defaults() %+v", prefs, want)
	}
}

// TestSaveLoadRoundTrip: a full Save then Load must reproduce every field,
// with ok=true once onboarding has actually completed.
func TestSaveLoadRoundTrip(t *testing.T) {
	fxIsolateConfigDir(t)
	want := Prefs{
		Handle: "ada", Theme: "midnight", Accent: "violet", Font: "Iosevka",
		Density: "compact", Status: "away", GroupDMs: true, Notify: NotifyOff,
		Onboarded: true, TS: 123456, ThreadWidth: 72,
	}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, ok := Load()
	if !ok {
		t.Error("saved prefs with onboarded:true should report ok=true")
	}
	if got != want {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
}

// TestLoadMergeContract pins the exact field the doc comment on Prefs.Notify
// warns about: a prefs.json written before Notify existed (or one that
// simply hasn't touched it) must keep notifications on by default. Notify is
// a string rather than a bool for precisely this reason — merge only
// overrides *non-empty* strings, so an absent key can't be confused with an
// explicit "off". The JSON here is written by hand, not marshaled from a
// struct, so this test can't pass by construction the way an echo would.
func TestLoadMergeContract(t *testing.T) {
	fxIsolateConfigDir(t)

	write := func(t *testing.T, body string) {
		t.Helper()
		dir, err := Dir()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "prefs.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("missing notify key keeps the default on", func(t *testing.T) {
		write(t, `{"handle":"ada","onboarded":true}`)
		prefs, _ := Load()
		if prefs.Notify != NotifyMentions {
			t.Errorf("Notify = %q, want the default %q preserved", prefs.Notify, NotifyMentions)
		}
		if !prefs.Notifications() {
			t.Error("an old prefs.json without notify must not silently disable notifications")
		}
	})

	t.Run("explicit off is honored", func(t *testing.T) {
		write(t, `{"handle":"ada","onboarded":true,"notify":"off"}`)
		prefs, _ := Load()
		if prefs.Notify != NotifyOff || prefs.Notifications() {
			t.Errorf("Notify = %q, Notifications() = %v, want off/false", prefs.Notify, prefs.Notifications())
		}
	})

	t.Run("group_dms true is honored even though it's a bool", func(t *testing.T) {
		write(t, `{"handle":"ada","onboarded":true,"group_dms":true}`)
		prefs, _ := Load()
		if !prefs.GroupDMs {
			t.Error("group_dms:true in the file should override the default")
		}
	})

	t.Run("empty handle keeps the default handle", func(t *testing.T) {
		write(t, `{"onboarded":true}`)
		prefs, _ := Load()
		if prefs.Handle != Defaults().Handle {
			t.Errorf("Handle = %q, want the default %q preserved by an empty override", prefs.Handle, Defaults().Handle)
		}
	})
}

// TestLoadCorruptJSON: a truncated/corrupt prefs.json must not crash or wedge
// the user out of the app — it degrades to Defaults with ok=false, same as a
// missing file, so the app falls back to onboarding rather than panicking.
func TestLoadCorruptJSON(t *testing.T) {
	fxIsolateConfigDir(t)
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prefs.json"), []byte(`{"handle": not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	prefs, ok := Load()
	if ok {
		t.Error("corrupt prefs.json should report ok=false")
	}
	if prefs != Defaults() {
		t.Errorf("corrupt prefs.json should fall back to Defaults(), got %+v", prefs)
	}
}

// TestPathUnderIsolatedDir: Path() must resolve inside the isolated config
// dir, not the developer's real one — a regression here would mean every
// other isolated test is silently touching the real filesystem too.
func TestPathUnderIsolatedDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "slack-tui", "prefs.json")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// TestDirFallsBackToHomeConfig: without XDG_CONFIG_HOME set (uncommon but
// real — minimal containers, some macOS shells), Dir() must fall back to
// $HOME/.config/slack-tui rather than erroring or defaulting to CWD.
func TestDirFallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	got, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "slack-tui")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

// TestReadConfigFileFallsBackToLegacyOnMiss: when the primary read fails,
// readConfigFile must still attempt the legacy directory rather than
// returning early. On Linux, os.UserConfigDir() resolves the same way Dir()
// does (both follow $XDG_CONFIG_HOME, else $HOME/.config), so legacyDir()
// and Dir() are the same path here — this test cannot exercise the
// macOS-only case where they diverge and a legacy file is actually found; it
// only proves the fallback attempt happens and still fails cleanly (no
// panic, ordinary os.ErrNotExist-class error) rather than silently invoking
// a nil path. See the comment on legacyDir.
func TestReadConfigFileFallsBackToLegacyOnMiss(t *testing.T) {
	fxIsolateConfigDir(t)
	if _, err := readConfigFile("prefs.json"); err == nil {
		t.Error("expected an error when neither the primary nor legacy path has the file")
	}
}

// TestDefaultThemeFollowsOmarchy: with a fixture Omarchy theme published,
// Defaults() must pick it up automatically so a fresh install on such a
// desktop matches without the user being asked.
func TestDefaultThemeFollowsOmarchy(t *testing.T) {
	fxOmarchyFixture(t, "background = \"#000000\"\nforeground = \"#ffffff\"\n")
	if got := defaultTheme(); got != theme.OmarchyName {
		t.Errorf("defaultTheme() = %q, want %q", got, theme.OmarchyName)
	}
	if got := Defaults().Theme; got != theme.OmarchyName {
		t.Errorf("Defaults().Theme = %q, want %q", got, theme.OmarchyName)
	}
}

// TestDefaultThemeFallsBackToCharcoal: with no desktop theme published,
// Defaults() must not merely fail — it names the built-in charcoal palette.
func TestDefaultThemeFallsBackToCharcoal(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) // empty: no omarchy/current/theme
	if got := defaultTheme(); got != "charcoal" {
		t.Errorf("defaultTheme() = %q, want charcoal", got)
	}
}

// TestValidClientID pins the shape both `slack-tui setup` and onboarding
// share: Slack's client ID is two digit runs separated by a dot. The
// most common real-world mistake — pasting the App ID (starts with "A")
// instead of the Client ID that sits right below it — must be rejected.
func TestValidClientID(t *testing.T) {
	for id, want := range map[string]bool{
		"123456.789012":        true,
		"1234567890123.987654": true,
		"A01B2C3D4E5":          false, // App ID, the classic mis-paste
		"123456":               false, // missing the dot pair
		"":                     false,
		"123456.789012 ":       true, // ValidClientID trims whitespace
		" 123456.789012":       true,
	} {
		if got := ValidClientID(id); got != want {
			t.Errorf("ValidClientID(%q) = %v, want %v", id, got, want)
		}
	}
}
