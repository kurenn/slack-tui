package config

import "encoding/json"

// State is small session state that should survive restarts: unsent drafts
// and the conversation-recency order behind the palette. Saved on quit.
type State struct {
	Drafts map[string]string `json:"drafts,omitempty"`
	Recent []string          `json:"recent,omitempty"`
}

// LoadState reads the saved session state (zero value if none).
func LoadState() (State, error) {
	var s State
	b, err := readConfigFile("state.json")
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// SaveState writes the session state (0600 — drafts are message text).
func SaveState(s State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeConfigFile("state.json", b, 0o600)
}
