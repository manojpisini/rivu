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

func TestNewSectionsDefaults(t *testing.T) {
	c := Default()
	if sum := c.Health.Weights["readme"] + c.Health.Weights["git"] + c.Health.Weights["bank"] +
		c.Health.Weights["map"] + c.Health.Weights["tests"] + c.Health.Weights["ci"] +
		c.Health.Weights["license"] + c.Health.Weights["deps_fresh"]; sum != 100 {
		t.Errorf("default health weights sum to %d, want 100", sum)
	}
	if c.Git.DefaultBranch != "main" {
		t.Errorf("git.default_branch = %q, want main", c.Git.DefaultBranch)
	}
	if c.Templates.Default != "empty" {
		t.Errorf("templates.default = %q, want empty", c.Templates.Default)
	}
	if c.Keybindings.Profile != "default" {
		t.Errorf("keybindings.profile = %q, want default", c.Keybindings.Profile)
	}
	if c.Bridge.Enabled {
		t.Error("bridge.enabled should default to false")
	}
	if len(c.Editors.GUI) == 0 || c.Editors.GUI[0] != "code" {
		t.Errorf("editors.gui default = %v, want a list starting with code", c.Editors.GUI)
	}
}

func TestPartialFileKeepsDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	body := "[git]\ndefault_branch = \"trunk\"\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Git.DefaultBranch != "trunk" {
		t.Errorf("override not applied, branch = %q", cfg.Git.DefaultBranch)
	}
	if cfg.Scanner.MaxDepth != 6 {
		t.Errorf("untouched section lost defaults, max_depth = %d", cfg.Scanner.MaxDepth)
	}
	if len(cfg.Health.Weights) != 8 {
		t.Errorf("health weights lost defaults, got %v", cfg.Health.Weights)
	}
}

func TestValidateWeightsSum(t *testing.T) {
	c := Default()
	c.Health.Weights["readme"] = 50
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "health.weights") {
		t.Errorf("bad weights accepted: %v", err)
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

func TestRoundTripAllFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	c := Default()
	c.Workspace.Root = filepath.Join(home, "ws")
	c.Workspace.SecondaryRoots = []string{filepath.Join(home, "ws2")}
	c.Workspace.AutoRescan = false
	c.Editors.Default = "nvim"
	c.Editors.PerLanguage = map[string]string{"go": "nvim"}
	c.Automation.CreateBank = false
	c.Flow.StaleThresholdDays = 30
	c.Flow.SourceSLADays = 7
	c.Scanner.Ignore = []string{"tmp"}
	c.Scanner.MaxDepth = 3
	c.Health.Weights = map[string]int{"readme": 25, "git": 25, "bank": 25, "map": 25}
	c.Git.DefaultBranch = "trunk"
	c.Bridge.Enabled = true
	c.Templates.Default = "go-cli"
	c.Keybindings.Profile = "vim"
	c.Appearance.Theme = "midnight"
	c.Appearance.Density = "compact"
	c.Data.DBPath = filepath.Join(home, "custom.db")
	c.Data.SnapshotRetentionDays = 14

	if err := Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, warns, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if got.Workspace.Root != c.Workspace.Root ||
		got.Workspace.AutoRescan != c.Workspace.AutoRescan ||
		got.Editors.Default != c.Editors.Default ||
		got.Editors.PerLanguage["go"] != "nvim" ||
		got.Automation.CreateBank != c.Automation.CreateBank ||
		got.Flow.StaleThresholdDays != 30 ||
		got.Flow.SourceSLADays != 7 ||
		got.Scanner.MaxDepth != 3 ||
		len(got.Scanner.Ignore) != 1 || got.Scanner.Ignore[0] != "tmp" ||
		got.Health.Weights["readme"] != 25 ||
		got.Git.DefaultBranch != "trunk" ||
		got.Bridge.Enabled != true ||
		got.Templates.Default != "go-cli" ||
		got.Keybindings.Profile != "vim" ||
		got.Appearance.Theme != "midnight" ||
		got.Appearance.Density != "compact" ||
		got.Data.DBPath != c.Data.DBPath ||
		got.Data.SnapshotRetentionDays != 14 {
		t.Errorf("round-trip lost fields:\n got %+v\nwant %+v", got, c)
	}
}

func TestLoadRejectsBrokenToml(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[[[not toml"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(); err == nil {
		t.Fatal("Load accepted broken TOML, want error")
	}
}

func TestSaveFailsUnderAFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	// config path under a regular file → MkdirAll must fail, nothing written
	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RIVU_CONFIG", filepath.Join(blocker, "nested", "config.toml"))

	if err := Save(Default()); err == nil {
		t.Fatal("Save succeeded under a file, want error")
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
