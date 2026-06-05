// Package config persists user preferences. It replaces the prototype's
// localStorage['slacktui_prefs'] handoff with a JSON file at
// ~/.config/slack-tui/prefs.json. Onboarding writes it; the app reads it on launch.
// The field set is the integration seam between the two screens.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Prefs is the persisted preference set (the slacktui_prefs contract).
type Prefs struct {
	Handle    string `json:"handle"`
	Theme     string `json:"theme"`
	Accent    string `json:"accent"`
	Font      string `json:"font"`
	Density   string `json:"density"`
	Status    string `json:"status"`
	Onboarded bool   `json:"onboarded"`
	TS        int64  `json:"ts"`
}

// Defaults mirrors the app's TWEAK_DEFAULTS for a first run with no saved prefs.
func Defaults() Prefs {
	return Prefs{
		Handle:    "you",
		Theme:     "charcoal",
		Accent:    "auto",
		Font:      "JetBrains Mono",
		Density:   "comfortable",
		Status:    "online",
		Onboarded: false,
	}
}

// Path returns the prefs file location, honoring XDG_CONFIG_HOME.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "slack-tui", "prefs.json"), nil
}

// Load reads saved prefs, merging over Defaults. A missing file is not an error:
// it returns Defaults with ok=false so callers can route to onboarding.
func Load() (prefs Prefs, ok bool) {
	prefs = Defaults()
	path, err := Path()
	if err != nil {
		return prefs, false
	}
	b, err := os.ReadFile(path)
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
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
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
	dst.Onboarded = src.Onboarded
	dst.TS = src.TS
}
