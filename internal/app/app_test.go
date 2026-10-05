package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
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

func TestMapAndFlowSetAssetFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")
	a.Config.Automation.CreateBank = false
	a.Config.Automation.BuildMap = false

	flags := func(t *testing.T, q string) (bank, m bool) {
		t.Helper()
		list, err := a.Registry.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range list {
			if p.Slug == q {
				return p.HasBank, p.HasMap
			}
		}
		t.Fatalf("project %q not found", q)
		return
	}

	if _, err := a.Source("demo", "source", false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	if b, m := flags(t, "demo"); b || m {
		t.Errorf("auto-creation off: has_bank=%v has_map=%v", b, m)
	}
	if err := a.Map("demo"); err != nil {
		t.Fatalf("Map: %v", err)
	}
	if b, m := flags(t, "demo"); !b || !m {
		t.Errorf("after Map: has_bank=%v has_map=%v, want true/true", b, m)
	}

	if _, err := a.Source("other", "source", false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	if _, _, err := a.Flow("other", "active", false); err != nil {
		t.Fatalf("Flow: %v", err)
	}
	if b, m := flags(t, "other"); !b || m {
		t.Errorf("after Flow: has_bank=%v has_map=%v, want true/false", b, m)
	}
}

func TestFlowRefreshesBankStage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")
	a.Config.Automation.CreateBank = true

	if _, err := a.Source("demo", "source", false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	p, _, err := a.Flow("demo", "active", false)
	if err != nil {
		t.Fatalf("Flow: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(p.Path, ".metadata", "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]any
	if _, err := toml.Decode(string(b), &doc); err != nil {
		t.Fatalf("moved project.toml must parse: %v", err)
	}
	if doc["rivu"]["flow_stage"] != "active" || doc["rivu"]["channel"] != "01_Active" {
		t.Errorf("stale project.toml after Flow: %v", doc["rivu"])
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
