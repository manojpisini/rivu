package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	cfg, warns, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err != nil {
		t.Errorf("config not written under RIVU_HOME: %v", err)
	}
	if cfg.Workspace.AutoRescan != true {
		t.Error("expected default AutoRescan true")
	}
}

func TestLoadWarnsOnUnknownKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	p := filepath.Join(home, "config.toml")
	body := "[workspace]\nroot = \"/tmp/ws\"\nwokspace_typo = true\n"
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, warns, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warns)
	}
	if !strings.Contains(warns[0], "workspace.wokspace_typo") && !strings.Contains(warns[0], "wokspace_typo") {
		t.Errorf("warning %q does not name the typo key", warns[0])
	}
	if cfg.Workspace.Root != filepath.Clean("/tmp/ws") {
		t.Errorf("known key not applied, root = %q", cfg.Workspace.Root)
	}
}

func TestSaveAtomicAndPrivate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	c := Default()
	c.Workspace.Root = filepath.Join(home, "ws")
	if err := Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p, _ := Path()
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0600 {
			t.Errorf("config perms = %o, want 600", perm)
		}
	}
	got, _, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if got.Workspace.Root != c.Workspace.Root {
		t.Errorf("round-trip root = %q, want %q", got.Workspace.Root, c.Workspace.Root)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}
