// Package config handles loading, saving, and defaults for all user configuration,
// including server credentials and key bindings.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Server holds Navidrome connection details.
type Server struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Hotkeys maps action names to key combo strings (e.g. "alt+space").
type Hotkeys struct {
	PlayPause   string `json:"play_pause"`
	Next        string `json:"next"`
	Prev        string `json:"prev"`
	VolumeUp    string `json:"volume_up"`
	VolumeDown  string `json:"volume_down"`
	ToggleMute  string `json:"toggle_mute"`
}

// Config is the root configuration object persisted to disk.
type Config struct {
	Server  Server  `json:"server"`
	Hotkeys Hotkeys `json:"hotkeys"`
	AutoDJ  bool    `json:"auto_dj"`
}

// Defaults returns a Config with blank custom hotkeys. Media keys (⏯ ⏭ ⏮)
// are handled automatically via the OS — custom hotkeys are opt-in.
func Defaults() Config {
	return Config{
		Hotkeys: Hotkeys{
			PlayPause:  "",
			Next:       "",
			Prev:       "",
			VolumeUp:   "",
			VolumeDown: "",
			ToggleMute: "",
		},
	}
}

// configPath returns the OS-appropriate path for the config file.
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kagura", "config.json"), nil
}

// Load reads config from disk. Returns Defaults() if no file exists yet.
func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Defaults(), err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), err
	}

	cfg := Defaults()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Defaults(), err
	}
	return cfg, nil
}

// Save writes cfg to disk, creating the config directory if needed.
func Save(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
