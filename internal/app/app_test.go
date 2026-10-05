package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanRejectsMissingWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	a.Config.Workspace.Root = filepath.Join(home, "never-created")
	if _, err := a.Scan(); err == nil {
		t.Fatal("Scan accepted a missing workspace root, want error")
	}
}

func TestScanIncludesSecondaryRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	mkGo := func(root, name string) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module x"), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	root1 := filepath.Join(home, "ws1")
	root2 := filepath.Join(home, "ws2")
	p1 := mkGo(root1, "main")
	p2 := mkGo(root2, "extra")

	a.Config.Workspace.Root = root1
	a.Config.Workspace.SecondaryRoots = []string{root2, filepath.Join(home, "gone")}
	ps, err := a.Scan()
	if err != nil {
		t.Fatalf("a missing secondary root must warn, not fail: %v", err)
	}
	paths := map[string]bool{}
	for _, p := range ps {
		paths[p.Path] = true
	}
	if !paths[p1] || !paths[p2] {
		t.Errorf("projects from both roots expected, got %v", paths)
	}
	found := false
	for _, w := range a.ScanWarnings {
		if strings.Contains(w, "secondary root skipped") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a secondary-root warning, got %v", a.ScanWarnings)
	}
}

func TestSourceSlugsNamesAndRejectsUnsafeOnes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	if _, err := a.Source("../../evil", "source", false, false); err == nil {
		t.Error("Source accepted a traversal name, want error")
	}
	if _, err := a.Source("CON", "source", false, false); err == nil {
		t.Error("Source accepted a reserved device name, want error")
	}
	p, err := a.Source("My Project", "source", false, false)
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if p.Slug != "my-project" {
		t.Errorf("slug = %q, want my-project", p.Slug)
	}
	want := filepath.Join(a.Config.Workspace.Root, "00_Source", "my-project")
	if p.Path != want {
		t.Errorf("path = %q, want %q", p.Path, want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Errorf("project dir not created: %v", err)
	}
}

func TestScanRejectsInvalidConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	body := "[scanner]\nmax_depth = 0\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(); err == nil {
		t.Fatal("Open accepted max_depth = 0, want error")
	}
}
