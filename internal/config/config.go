// Package config loads and saves tmuxgo's user configuration:
// theme, startup defaults, and key bindings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is tmuxgo's user configuration. Load starts from Default and
// overlays the file, so missing keys keep their defaults.
type Config struct {
	// Theme is "auto", "dark", or "light".
	Theme string `json:"theme"`
	// PreviewDefault enables the pane preview at startup.
	PreviewDefault bool `json:"preview_default"`
	// Mouse enables mouse interactions in the TUI.
	Mouse bool `json:"mouse"`
	// Keys maps action name -> key bindings (bubbletea key strings).
	// An action present here replaces its defaults entirely.
	Keys map[string][]string `json:"keys"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Theme: "auto",
		Mouse: true,
		Keys:  DefaultKeys(),
	}
}

// DefaultKeys returns the built-in action -> keys mapping.
func DefaultKeys() map[string][]string {
	return map[string][]string{
		"up":       {"up", "k"},
		"down":     {"down", "j"},
		"expand":   {"right", "l"},
		"collapse": {"left", "h"},
		"attach":   {"enter"},
		"new":      {"n"},
		"rename":   {"r"},
		"move":     {"m"},
		"kill":     {"d"},
		"filter":   {"/"},
		"preview":  {"p"},
		"help":     {"?"},
		"settings": {","},
		"socket":   {"S"},
		"quit":     {"q", "ctrl+c"},
	}
}

// DefaultPath is ~/.config/tmuxgo/config.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tmuxgo", "config.json"), nil
}

// Load reads the config at path, overlaying it on the defaults. A missing
// file yields the defaults; an invalid theme is rejected.
func Load(path string) (Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	switch cfg.Theme {
	case "", "auto", "dark", "light":
	default:
		return cfg, fmt.Errorf("invalid theme %q (want auto, dark, or light)", cfg.Theme)
	}
	if cfg.Theme == "" {
		cfg.Theme = "auto"
	}
	return cfg, nil
}

// Save writes the config to path, creating parent directories.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
