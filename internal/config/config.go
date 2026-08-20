// Package config persists user preferences. It replaces the prototype's
// localStorage['slacktui_prefs'] handoff with a JSON file at
// ~/.config/slack-tui/prefs.json. Onboarding writes it; the app reads it on launch.
// The field set is the integration seam between the two screens.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/kurenn/slack-tui/internal/theme"
)

// Prefs is the persisted preference set (the slacktui_prefs contract).
type Prefs struct {
	Handle      string `json:"handle"`
	Theme       string `json:"theme"`
	Accent      string `json:"accent"`
	Font        string `json:"font"`
	Density     string `json:"density"`
	Status      string `json:"status"`
	GroupDMs    bool   `json:"group_dms"` // include group DMs (mpims) in the sidebar
	Onboarded   bool   `json:"onboarded"`
	TS          int64  `json:"ts"`
	ThreadWidth int    `json:"thread_width,omitempty"`
}

// Defaults mirrors the app's TWEAK_DEFAULTS for a first run with no saved prefs.
func Defaults() Prefs {
	return Prefs{
		Handle:      "you",
		Theme:       defaultTheme(),
		Accent:      "auto",
		Font:        "JetBrains Mono",
		Density:     "comfortable",
		Status:      "online",
		Onboarded:   false,
		ThreadWidth: 60,
	}
}

// Dir returns slack-tui's config directory: $XDG_CONFIG_HOME/slack-tui, or
// ~/.config/slack-tui. That's the conventional home for terminal tools on
// every platform — Go's os.UserConfigDir points at Library/Application
// Support on macOS, where nobody looks for a TUI's config.
func Dir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "slack-tui"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "slack-tui"), nil
}

// legacyDir is the pre-v0.1.7 location (os.UserConfigDir — Library/Application
// Support on macOS). Reads fall back to it so existing installs keep working;
// writes always go to Dir.
func legacyDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "slack-tui"), nil
}

// readConfigFile loads name from the config dir, falling back to the legacy
// dir for files written by older versions.
func readConfigFile(name string) ([]byte, error) {
	if dir, err := Dir(); err == nil {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			return b, nil
		}
	}
	dir, err := legacyDir()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, name))
}

// writeConfigFile saves name into the config dir with the given permissions.
func writeConfigFile(name string, b []byte, perm os.FileMode) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), b, perm)
}

// Path returns the prefs file location.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "prefs.json"), nil
}

// Load reads saved prefs, merging over Defaults. A missing file is not an error:
// it returns Defaults with ok=false so callers can route to onboarding.
func Load() (prefs Prefs, ok bool) {
	prefs = Defaults()
	b, err := readConfigFile("prefs.json")
	if err != nil {
		return prefs, false
	}
	var saved Prefs
	if err := json.Unmarshal(b, &saved); err != nil {
		return prefs, false
	}
	merge(&prefs, saved)
	return prefs, saved.Onboarded
}

// Save writes prefs to the config file, creating the directory if needed.
func Save(p Prefs) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeConfigFile("prefs.json", b, 0o644)
}

func merge(dst *Prefs, src Prefs) {
	if src.Handle != "" {
		dst.Handle = src.Handle
	}
	if src.Theme != "" {
		dst.Theme = src.Theme
	}
	if src.Accent != "" {
		dst.Accent = src.Accent
	}
	if src.Font != "" {
		dst.Font = src.Font
	}
	if src.Density != "" {
		dst.Density = src.Density
	}
	if src.Status != "" {
		dst.Status = src.Status
	}
	dst.GroupDMs = src.GroupDMs
	dst.Onboarded = src.Onboarded
	dst.TS = src.TS
	if src.ThreadWidth > 0 {
		dst.ThreadWidth = src.ThreadWidth
	}
}

// defaultTheme follows the desktop palette when one is published, so a fresh
// install on such a system matches the rest of the desktop without being asked.
// An explicit choice made later in Settings overrides it and is what gets saved.
func defaultTheme() string {
	if theme.OmarchyAvailable() {
		return theme.OmarchyName
	}
	return "charcoal"
}
