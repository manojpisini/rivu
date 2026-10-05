package service

import (
	"os"
	"path/filepath"
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
