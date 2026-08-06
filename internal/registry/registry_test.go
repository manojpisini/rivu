package registry

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUpsertAllowsDuplicateProjectNamesAtDifferentPaths(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	one := Project{ID: "one", Name: "admin", Slug: "admin", Path: filepath.Join("root", "web", "admin"), Channel: "04_Languages", FlowStage: "source", CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: true}
	two := Project{ID: "two", Name: "admin", Slug: "admin", Path: filepath.Join("root", "rust", "admin"), Channel: "04_Languages", FlowStage: "source", CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: true}

	if err := r.Upsert(one); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(two); err != nil {
		t.Fatalf("second duplicate-name project should be accepted: %v", err)
	}

	projects, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
	if projects[0].Slug == projects[1].Slug {
		t.Fatalf("slugs must be unique, both were %q", projects[0].Slug)
	}
}

func TestUpsertKeepsStableSlugWhenRescanningSamePath(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	path := filepath.Join("root", "web", "admin")
	first := Project{ID: "one", Name: "admin", Slug: "admin", Path: path, Channel: "04_Languages", FlowStage: "source", CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: true}
	rescan := first
	rescan.ID = "new-random-id"

	if err := r.Upsert(first); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(rescan); err != nil {
		t.Fatal(err)
	}

	projects, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if projects[0].ID != "one" {
		t.Fatalf("rescan replaced stable id: got %q", projects[0].ID)
	}
	if projects[0].Slug != "admin" {
		t.Fatalf("rescan changed stable slug: got %q", projects[0].Slug)
	}
}
