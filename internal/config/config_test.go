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
	if want, _ := filepath.Abs("/tmp/ws"); cfg.Workspace.Root != want {
		t.Errorf("known key not applied, root = %q, want %q", cfg.Workspace.Root, want)
	}
}

func TestExpandForms(t *testing.T) {
	h, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	t.Setenv("RIVU_TEST_WS", filepath.Join(t.TempDir(), "ws"))

	tests := []struct{ in, want string }{
		{"~", h},
		{"~/proj", filepath.Join(h, "proj")},
		{"$HOME/proj", filepath.Join(h, "proj")},
		{"${RIVU_TEST_WS}", os.Getenv("RIVU_TEST_WS")},
		{"relative/dir", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := expand(tt.in)
			if !filepath.IsAbs(got) {
				t.Errorf("expand(%q) = %q, not absolute", tt.in, got)
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("expand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"defaults pass", func(*Config) {}, false},
		{"zero max_depth fails", func(c *Config) { c.Scanner.MaxDepth = 0 }, true},
		{"zero stale days fails", func(c *Config) { c.Flow.StaleThresholdDays = 0 }, true},
		{"zero source sla fails", func(c *Config) { c.Flow.SourceSLADays = 0 }, true},
		{"max_depth 1 passes", func(c *Config) { c.Scanner.MaxDepth = 1 }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default()
			tt.mutate(&c)
			err := Validate(c)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	c := Default()
	c.Workspace.Root = filepath.Join(home, "missing")
	if err := ValidateRoot(c); err == nil {
		t.Error("missing root accepted, want error")
	}
	if err := os.MkdirAll(c.Workspace.Root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRoot(c); err != nil {
		t.Errorf("existing root rejected: %v", err)
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
