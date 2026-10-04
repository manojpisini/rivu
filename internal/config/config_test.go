package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirEnvOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if got != home {
		t.Errorf("Dir() = %q, want %q", got, home)
	}

	cfg := Default()
	if want := filepath.Join(home, "rivu.db"); cfg.Data.DBPath != want {
		t.Errorf("Default().Data.DBPath = %q, want %q", cfg.Data.DBPath, want)
	}
}

func TestDirDefaultsToUserHome(t *testing.T) {
	t.Setenv("RIVU_HOME", "")
	t.Setenv("RIVU_CONFIG", "")

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	h, _ := os.UserHomeDir()
	if want := filepath.Join(h, ".rivu"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestPathConfigEnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "custom.toml")
	t.Setenv("RIVU_HOME", dir)
	t.Setenv("RIVU_CONFIG", cfgPath)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got != cfgPath {
		t.Errorf("Path() = %q, want %q", got, cfgPath)
	}
}

func TestLoadUsesEnvHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err != nil {
		t.Errorf("config not written under RIVU_HOME: %v", err)
	}
	if cfg.Workspace.AutoRescan != true {
		t.Error("expected default AutoRescan true")
	}
}
