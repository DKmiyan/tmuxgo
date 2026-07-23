package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "auto" || !cfg.Mouse || cfg.PreviewDefault {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.Keys["quit"][0] != "q" {
		t.Fatalf("default keys = %v", cfg.Keys["quit"])
	}
}

func TestLoadOverlayKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","preview_default":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "dark" || !cfg.PreviewDefault {
		t.Fatalf("overlay = %+v", cfg)
	}
	if !cfg.Mouse {
		t.Fatal("unset mouse key must keep default true")
	}
	if len(cfg.Keys) == 0 {
		t.Fatal("unset keys must keep defaults")
	}
}

func TestLoadRejectsBadTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"theme":"solarized"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("invalid theme must be rejected")
	}
}

func TestSaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	cfg := Default()
	cfg.Theme = "light"
	cfg.Keys["new"] = []string{"N"}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Theme != "light" || got.Keys["new"][0] != "N" {
		t.Fatalf("reloaded = %+v", got)
	}
}
