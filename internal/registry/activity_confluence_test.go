package registry

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRecentActivityOrderAndLimit(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	for _, p := range []Project{
		{ID: "p1", Name: "One", Slug: "one", Path: filepath.Join(t.TempDir(), "one"), FlowStage: "source", Channel: "00_Source"},
		{ID: "p2", Name: "Two", Slug: "two", Path: filepath.Join(t.TempDir(), "two"), FlowStage: "source", Channel: "00_Source"},
	} {
		if err := r.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.LogActivity("p1", "opened"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := r.LogActivity("p2", "sourced"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := r.LogActivity("p1", "flowed"); err != nil {
		t.Fatal(err)
	}

	all, err := r.RecentActivity(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("RecentActivity = %d events, want 3", len(all))
	}
	if all[0].Slug != "one" || all[0].Event != "flowed" {
		t.Errorf("newest = %s/%s, want one/flowed", all[0].Slug, all[0].Event)
	}
	if all[2].Event != "opened" {
		t.Errorf("oldest = %s, want opened", all[2].Event)
	}

	limited, err := r.RecentActivity(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].Event != "flowed" {
		t.Errorf("limit 1 = %+v, want the newest event", limited)
	}
}

func TestAttachConfluenceAndLookup(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	p := Project{ID: "p1", Name: "One", Slug: "one", Path: filepath.Join(t.TempDir(), "one"), FlowStage: "source", Channel: "00_Source"}
	if err := r.Upsert(p); err != nil {
		t.Fatal(err)
	}

	if err := r.AttachConfluence("p1", "ship"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	// Repeating a membership is a no-op, not a failure.
	if err := r.AttachConfluence("p1", "ship"); err != nil {
		t.Fatalf("re-attach: %v", err)
	}
	if err := r.AttachConfluence("p1", "   "); err == nil {
		t.Error("blank confluence name must fail")
	}

	byName, err := r.ConfluenceProjectIDs("ship")
	if err != nil {
		t.Fatal(err)
	}
	if len(byName) != 1 || byName[0] != "p1" {
		t.Errorf("by name = %v, want [p1]", byName)
	}

	var cid string
	if err := r.DB.QueryRow(`SELECT id FROM confluences WHERE name = ?`, "ship").Scan(&cid); err != nil {
		t.Fatal(err)
	}
	byID, err := r.ConfluenceProjectIDs(cid)
	if err != nil {
		t.Fatal(err)
	}
	if len(byID) != 1 || byID[0] != "p1" {
		t.Errorf("by id = %v, want [p1]", byID)
	}

	// An unknown confluence matches nothing rather than failing.
	none, err := r.ConfluenceProjectIDs("missing")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("unknown confluence = %v, want empty", none)
	}
}
