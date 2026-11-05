package main

import (
	"encoding/json"
	"os"
)

// Settings are the few preferences that must be readable before the vault is unlocked. No secrets here.
type Settings struct {
	IdleMinutes int  `json:"idleMinutes"` // lock after this many idle minutes; 0 means never
	EndOnLock   bool `json:"endOnLock"`   // locking also ends open shells and VPNs
	// NoReconnect turns off redialling a dropped connection. It is stored inverted so a settings file written
	// before this option existed still means "on".
	NoReconnect bool `json:"noAutoReconnect,omitempty"`
}

func defaultSettings() Settings { return Settings{IdleMinutes: 15} }

// loadSettings returns the defaults when the file is missing or unreadable.
func loadSettings(path string) Settings {
	s := defaultSettings()
	if path == "" {
		return s
	}
	if b, err := os.ReadFile(path); err == nil {
		var got Settings
		if json.Unmarshal(b, &got) == nil && got.IdleMinutes >= 0 {
			s = got
		}
	}
	return s
}

func saveSettings(path string, s Settings) error {
	if path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
