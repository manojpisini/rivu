package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/manojpisini/rivu/internal/cli"
)

func TestHomeFlagSetsEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", "")
	t.Setenv("RIVU_CONFIG", "")

	r := cli.Root("test", "dev", "unknown")
	r.SetArgs([]string{"--home", home, "config", "path"})
	if err := r.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := os.Getenv("RIVU_HOME"); got != home {
		t.Errorf("RIVU_HOME = %q, want %q", got, home)
	}
}

func TestConfigFlagOverridesPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "alt.toml")
	t.Setenv("RIVU_HOME", dir)
	t.Setenv("RIVU_CONFIG", "")

	r := cli.Root("test", "dev", "unknown")
	r.SetArgs([]string{"--config", cfgPath, "config", "path"})
	if err := r.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := os.Getenv("RIVU_CONFIG"); got != cfgPath {
		t.Errorf("RIVU_CONFIG = %q, want %q", got, cfgPath)
	}
}
