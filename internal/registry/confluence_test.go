package registry

import (
	"errors"
	"path/filepath"
	"testing"
)

func confluenceTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func confluenceSeedProjects(t *testing.T, r *Registry) {
	t.Helper()
	for _, p := range []Project{
		{ID: "p1", Name: "Site", Slug: "site", Path: filepath.Join(t.TempDir(), "site"), FlowStage: "active", Channel: "01_Active"},
		{ID: "p2", Name: "Brand", Slug: "brand", Path: filepath.Join(t.TempDir(), "brand"), FlowStage: "source", Channel: "00_Source"},
	} {
		if err := r.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}
}

// TestConfluenceCRUD (P5.01): create slug-normalises and rejects
// duplicates, list counts members by name, rename frees the old name,
// delete unlinks the confluence only — projects survive.
func TestConfluenceCRUD(t *testing.T) {
	r := confluenceTestRegistry(t)
	confluenceSeedProjects(t, r)

	c, err := r.CreateConfluence("Heap & Stack", "publication family")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.Name != "heap-stack" || c.ID == "" {
		t.Errorf("created = %+v, want slug name heap-stack with an id", c)
	}
	if _, err := r.CreateConfluence("Heap & Stack", ""); err == nil {
		t.Error("duplicate confluence must fail")
	}
	if _, err := r.CreateConfluence("   ", ""); err == nil {
		t.Error("blank confluence must fail")
	}
	if _, err := r.CreateConfluence("a/b", ""); err == nil {
		t.Error("path separator in confluence name must fail")
	}

	if err := r.AttachConfluence("p1", "heap-stack"); err != nil {
		t.Fatal(err)
	}
	if err := r.AttachConfluence("p2", "Heap & Stack"); err != nil {
		t.Fatalf("attach normalises like create: %v", err)
	}

	list, err := r.ListConfluences()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Members != 2 || list[0].Notes != "publication family" {
		t.Fatalf("list = %+v, want heap-stack with 2 members", list)
	}

	// lookup by name and by id agree
	byName, err := r.Confluence("heap-stack")
	if err != nil || byName.ID != c.ID {
		t.Errorf("by name = %+v, %v; want id %s", byName, err, c.ID)
	}
	if _, err := r.Confluence("missing"); !errors.Is(err, ErrConfluenceNotFound) {
		t.Errorf("missing confluence err = %v, want ErrConfluenceNotFound", err)
	}

	// rename frees the old name and the new one is normalised
	if n, err := r.RenameConfluence("heap-stack", "H&S"); err != nil || n != "h-s" {
		t.Fatalf("rename = %q, %v; want stored h-s", n, err)
	}
	got, err := r.Confluence("h-s")
	if err != nil || got.ID != c.ID {
		t.Errorf("after rename = %+v, %v; want the same confluence as h-s", got, err)
	}
	if _, err := r.Confluence("heap-stack"); !errors.Is(err, ErrConfluenceNotFound) {
		t.Errorf("old name still resolves: %v", err)
	}
	if n, err := r.RenameConfluence("h-s", "h-s"); err != nil || n != "h-s" {
		t.Errorf("rename to the same name must be a no-op, got %q, %v", n, err)
	}
	if _, err := r.RenameConfluence("ghost", "x"); !errors.Is(err, ErrConfluenceNotFound) {
		t.Errorf("rename missing = %v, want ErrConfluenceNotFound", err)
	}
	if _, err := r.CreateConfluence("devtools", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenameConfluence("h-s", "devtools"); err == nil {
		t.Error("rename onto an existing confluence must fail")
	}

	// delete removes the confluence and its links, never the projects
	if err := r.DeleteConfluence("h-s"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if left, _ := r.ListConfluences(); len(left) != 1 || left[0].Name != "devtools" {
		t.Errorf("confluences after delete = %+v, want only devtools", left)
	}
	if n, _ := r.ProjectConfluences("p1"); len(n) != 0 {
		t.Errorf("project links after delete = %v, want none", n)
	}
	if ps, err := r.List(); err != nil || len(ps) != 2 {
		t.Errorf("projects after delete = %d, %v; want both intact", len(ps), err)
	}
	if err := r.DeleteConfluence("h-s"); !errors.Is(err, ErrConfluenceNotFound) {
		t.Errorf("delete missing = %v, want ErrConfluenceNotFound", err)
	}
}

// TestConfluenceManyToMany (P5.01): membership runs both ways — a
// project sits in several confluences, a confluence holds several
// projects; detach unlinks one edge and reports it.
func TestConfluenceManyToMany(t *testing.T) {
	r := confluenceTestRegistry(t)
	confluenceSeedProjects(t, r)

	for _, link := range [][2]string{{"p1", "ship"}, {"p1", "brand"}, {"p2", "ship"}} {
		if err := r.AttachConfluence(link[0], link[1]); err != nil {
			t.Fatalf("attach %v: %v", link, err)
		}
	}

	ship, err := r.Confluence("ship")
	if err != nil || ship.Members != 2 {
		t.Fatalf("ship = %+v, %v, want 2 members", ship, err)
	}
	members, err := r.ConfluenceMembers("ship")
	if err != nil || len(members) != 2 || members[0].Slug != "brand" || members[1].Slug != "site" {
		t.Errorf("members = %+v, %v; want lifecycle order [brand site]", members, err)
	}

	if names, err := r.ProjectConfluences("p1"); err != nil || len(names) != 2 || names[0] != "brand" || names[1] != "ship" {
		t.Errorf("project 1 confluences = %v, %v; want [brand ship]", names, err)
	}
	if ids, err := r.ConfluenceProjectIDs("ship"); err != nil || len(ids) != 2 {
		t.Errorf("ship ids = %v, %v; want 2", ids, err)
	}

	// detach unlinks one edge only
	if ok, err := r.DetachConfluence("p1", "ship"); err != nil || !ok {
		t.Fatalf("detach = %v, %v; want true", ok, err)
	}
	if ok, err := r.DetachConfluence("p1", "ship"); err != nil || ok {
		t.Errorf("second detach = %v, %v; want false, nil (already unlinked)", ok, err)
	}
	if names, _ := r.ProjectConfluences("p1"); len(names) != 1 || names[0] != "brand" {
		t.Errorf("project 1 after detach = %v, want [brand]", names)
	}
	if ship, _ = r.Confluence("ship"); ship.Members != 1 {
		t.Errorf("ship members after detach = %d, want 1", ship.Members)
	}
	if ok, err := r.DetachConfluence("p1", "ghost"); !errors.Is(err, ErrConfluenceNotFound) || ok {
		t.Errorf("detach unknown = %v, %v; want ErrConfluenceNotFound", ok, err)
	}
	if ps, _ := r.List(); len(ps) != 2 {
		t.Errorf("projects = %d, want both intact", len(ps))
	}
}
