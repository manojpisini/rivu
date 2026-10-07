package service

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSourceClassificationFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	sr, err := a.Source("Repo Doctor", SourceOpts{
		Flow:        "source",
		Domain:      "devtools",
		Type:        "app",
		Language:    "Go",
		Template:    "go-cli",
		Description: "Keeps repos healthy",
		Confluence:  []string{"ship", "brand"},
	})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	wantPath := filepath.Join(home, "ws", "00_Source", "devtools", "repo-doctor")
	if sr.Project.Path != wantPath {
		t.Errorf("path = %s, want %s", sr.Project.Path, wantPath)
	}
	if sr.Project.Language != "Go" {
		t.Errorf("language = %q, want Go", sr.Project.Language)
	}
	// project.toml carries the classification fields.
	b, err := os.ReadFile(filepath.Join(sr.Project.Path, ".metadata", "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`domain = "devtools"`, `type = "app"`, `template = "go-cli"`, `description = "Keeps repos healthy"`, `confluences = ["ship", "brand"]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("project.toml missing %q:\n%s", want, b)
		}
	}
	// Both confluences are linked in the registry.
	for _, c := range []string{"ship", "brand"} {
		ids, err := a.Registry.ConfluenceProjectIDs(c)
		if err != nil || len(ids) != 1 || ids[0] != sr.Project.ID {
			t.Errorf("confluence %s members = %v, %v; want the new project", c, ids, err)
		}
	}
}

func TestSourceBridgeFlagOwnsGitInit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	sr, err := a.Source("bridged-by-flag", SourceOpts{Flow: "source", Git: true, Bridge: true})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if !strings.Contains(strings.Join(sr.Warnings, "\n"), "--bridge") {
		t.Errorf("warnings = %v, want --bridge ownership warning", sr.Warnings)
	}
	if _, err := os.Stat(filepath.Join(sr.Project.Path, ".git")); err == nil {
		t.Error("git init ran even though --bridge owns it")
	}
	b, err := os.ReadFile(filepath.Join(sr.Project.Path, ".metadata", "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `git_init_owner = "bridge"`) {
		t.Errorf("project.toml owner = %s, want bridge", b)
	}
}

// TestSourceOptsOverrideAutomation (P4.23): the wizard's Create Bank /
// Build Map rows override [automation] for one run — nil keeps the
// config value, and the dry plan shows the same outcome as the apply.
func TestSourceOptsOverrideAutomation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")
	off, on := false, true

	// config defaults say yes; the run opts say no
	dry, err := a.Source("no-bank", SourceOpts{Dry: true, CreateBank: &off, BuildMap: &off})
	if err != nil {
		t.Fatalf("dry source: %v", err)
	}
	if len(dry.Plan.Bank) != 0 || len(dry.Plan.Write) != 0 {
		t.Errorf("plan = %+v, want no Bank and no Map writes with the rows off", dry.Plan)
	}
	sr, err := a.Source("no-bank", SourceOpts{CreateBank: &off, BuildMap: &off})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if sr.Project.HasBank || sr.Project.HasMap {
		t.Errorf("flags bank=%v map=%v, want both off", sr.Project.HasBank, sr.Project.HasMap)
	}
	for _, f := range []string{filepath.Join(".metadata", "project.toml"), filepath.Join(".metadata", "agent", "PROJECT_MAP.md")} {
		if _, err := os.Stat(filepath.Join(sr.Project.Path, f)); err == nil {
			t.Errorf("%s exists although its row was off", f)
		}
	}

	// config defaults say no; the run opts say yes
	a.Config.Automation.CreateBank = false
	a.Config.Automation.BuildMap = false
	dry, err = a.Source("with-bank", SourceOpts{Dry: true, CreateBank: &on, BuildMap: &on})
	if err != nil {
		t.Fatalf("dry source: %v", err)
	}
	if len(dry.Plan.Bank) != 4 {
		t.Errorf("plan.Bank = %v, want the four Bank files", dry.Plan.Bank)
	}
	if !slices.Contains(dry.Plan.Write, ".metadata/agent/PROJECT_MAP.md") {
		t.Errorf("plan.Write = %v, want the Map files", dry.Plan.Write)
	}
	sr, err = a.Source("with-bank", SourceOpts{CreateBank: &on, BuildMap: &on})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if !sr.Project.HasBank || !sr.Project.HasMap {
		t.Errorf("flags bank=%v map=%v, want both on", sr.Project.HasBank, sr.Project.HasMap)
	}
	for _, f := range []string{filepath.Join(".metadata", "project.toml"), filepath.Join(".metadata", "agent", "PROJECT_MAP.md")} {
		if _, err := os.Stat(filepath.Join(sr.Project.Path, f)); err != nil {
			t.Errorf("%s missing although its row was on: %v", f, err)
		}
	}

	// nil opts keep the config value (the CLI path)
	a.Config.Automation.CreateBank = true
	a.Config.Automation.BuildMap = true
	sr, err = a.Source("config-defaults", SourceOpts{})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if !sr.Project.HasBank || !sr.Project.HasMap {
		t.Errorf("flags bank=%v map=%v, want the config defaults honoured", sr.Project.HasBank, sr.Project.HasMap)
	}
}
