package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func confluenceTestApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	a.Config.Workspace.Root = filepath.Join(home, "ws")
	return a
}

func projectToml(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(path, ".metadata", "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestConfluenceProjectTomlMirror (P5.03): every confluence mutation
// rewrites [rivu].confluences in the affected projects' Banks from the
// registry — add, rename across all members, remove one, delete the
// rest.
// TestConfluenceServiceReads (P5.01): the read-side service methods —
// Confluences with member counts, ConfluenceShow with its projects,
// ProjectConfluences per project, and typed not-found errors.
func TestConfluenceServiceReads(t *testing.T) {
	a := confluenceTestApp(t)
	for _, name := range []string{"alpha", "beta"} {
		if _, err := a.Source(name, SourceOpts{}); err != nil {
			t.Fatalf("Source %s: %v", name, err)
		}
	}
	if _, err := a.ConfluenceNew("ship", "notes"); err != nil {
		t.Fatalf("ConfluenceNew: %v", err)
	}
	if err := a.ConfluenceAdd("alpha", "ship"); err != nil {
		t.Fatalf("ConfluenceAdd: %v", err)
	}

	cs, err := a.Confluences()
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Name != "ship" || cs[0].Members != 1 {
		t.Fatalf("Confluences = %+v, want ship with 1 member", cs)
	}

	c, members, err := a.ConfluenceShow("ship")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "ship" || c.Notes != "notes" || len(members) != 1 || members[0].Slug != "alpha" {
		t.Fatalf("ConfluenceShow = %+v / %+v, want ship/notes + alpha", c, members)
	}

	names, err := a.ProjectConfluences("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "ship" {
		t.Fatalf("ProjectConfluences = %v, want [ship]", names)
	}

	if _, _, err := a.ConfluenceShow("nope"); err == nil {
		t.Error("unknown confluence must surface an error")
	}
	if _, err := a.ProjectConfluences("nope"); err == nil {
		t.Error("unknown project must surface an error")
	}
}

func TestConfluenceProjectTomlMirror(t *testing.T) {
	a := confluenceTestApp(t)
	paths := map[string]string{}
	for _, name := range []string{"site", "brand"} {
		sr, err := a.Source(name, SourceOpts{})
		if err != nil {
			t.Fatalf("source %s: %v", name, err)
		}
		paths[name] = sr.Project.Path
	}

	// add writes the slug-normalised name into both Banks.
	if _, err := a.ConfluenceNew("Heap & Stack", ""); err != nil {
		t.Fatalf("new: %v", err)
	}
	for _, q := range []string{"site", "brand"} {
		if err := a.ConfluenceAdd(q, "Heap & Stack"); err != nil {
			t.Fatalf("add %s: %v", q, err)
		}
	}
	for name, path := range paths {
		if body := projectToml(t, path); !strings.Contains(body, `confluences = ["heap-stack"]`) {
			t.Errorf("%s project.toml = %s\nwant confluences = [\"heap-stack\"]", name, body)
		}
	}

	// rename refreshes every member's Bank.
	if _, err := a.ConfluenceRename("heap-stack", "H&S"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	for name, path := range paths {
		body := projectToml(t, path)
		if !strings.Contains(body, `confluences = ["h-s"]`) || strings.Contains(body, "heap-stack") {
			t.Errorf("%s after rename = %s\nwant confluences = [\"h-s\"]", name, body)
		}
	}

	// remove clears only that project.
	if linked, err := a.ConfluenceRemove("site", "h-s"); err != nil || !linked {
		t.Fatalf("remove = %v, %v; want true", linked, err)
	}
	if body := projectToml(t, paths["site"]); !strings.Contains(body, `confluences = []`) {
		t.Errorf("site after remove = %s, want cleared", body)
	}
	if body := projectToml(t, paths["brand"]); !strings.Contains(body, `confluences = ["h-s"]`) {
		t.Errorf("brand after remove = %s, want h-s kept", body)
	}

	// delete clears the remaining member; the projects stay.
	if err := a.ConfluenceDelete("h-s"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if body := projectToml(t, paths["brand"]); !strings.Contains(body, `confluences = []`) {
		t.Errorf("brand after delete = %s, want cleared", body)
	}
	ps, err := a.List(Filter{})
	if err != nil || len(ps) != 2 {
		t.Errorf("projects after delete = %d, %v; want both intact", len(ps), err)
	}
}

// TestConfluenceMirrorSkipsProjectsWithoutBank: the mirror never
// creates a Bank — no folder, no file, no registry drift (safety rule
// 1).
func TestConfluenceMirrorSkipsProjectsWithoutBank(t *testing.T) {
	a := confluenceTestApp(t)
	off := false
	sr, err := a.Source("plain", SourceOpts{CreateBank: &off})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := a.ConfluenceNew("devtools", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.ConfluenceAdd("plain", "devtools"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sr.Project.Path, ".metadata", "project.toml")); !os.IsNotExist(err) {
		t.Errorf("mirror created a Bank for a project without one: %v", err)
	}
	// The registry link still happened — bank.Sync picks it up later.
	if names, err := a.Registry.ProjectConfluences(sr.Project.ID); err != nil || len(names) != 1 {
		t.Errorf("registry confluences = %v, %v; want [devtools]", names, err)
	}
}

// TestSourceConfluenceMirror: --confluence lands as slugs in the Bank
// even though the flag may carry a display name (P5.03).
func TestSourceConfluenceMirror(t *testing.T) {
	a := confluenceTestApp(t)
	sr, err := a.Source("Site", SourceOpts{Confluence: []string{"Heap & Stack"}})
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if body := projectToml(t, sr.Project.Path); !strings.Contains(body, `confluences = ["heap-stack"]`) {
		t.Errorf("project.toml = %s\nwant slug-normalised confluences", body)
	}
}
