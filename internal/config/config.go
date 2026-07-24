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
	// Theme is "auto", a builtin theme name ("dark", "light",
	// "catppuccin-mocha", ...). Unknown names fall back to the default.
	Theme string `json:"theme"`
	// Language is "auto", "en", or "zh". "auto" follows LC_ALL/LANG.
	Language string `json:"language"`
	// Colors overrides individual semantic colors of the theme (keys:
	// accent, success, attached, danger, muted, selFg; values: any
	// lipgloss color spec like "63" or "#cba6f7").
	Colors map[string]string `json:"colors,omitempty"`
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
		Theme:    "auto",
		Language: "auto",
		Mouse:    true,
		Keys:     DefaultKeys(),
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
	// Theme names are resolved by the app (unknown ones fall back to the
	// default), so any non-empty string passes here.
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
