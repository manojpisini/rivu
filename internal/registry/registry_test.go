package registry

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOpenAppliesPragmas(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	for _, tc := range []struct{ pragma, want string }{
		{"foreign_keys", "1"},
		{"busy_timeout", "5000"},
		{"journal_mode", "wal"},
	} {
		var got string
		if err := r.DB.QueryRow("PRAGMA " + tc.pragma).Scan(&got); err != nil {
			t.Fatalf("PRAGMA %s: %v", tc.pragma, err)
		}
		if got != tc.want {
			t.Errorf("PRAGMA %s = %q, want %q", tc.pragma, got, tc.want)
		}
	}
}

func TestCanonicalCleansPath(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "ws", "proj")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	want := canonical(sub)
	for _, in := range []string{
		sub + string(os.PathSeparator),
		filepath.Join(base, "ws", "other", "..", "proj"),
	} {
		if got := canonical(in); got != want {
			t.Errorf("canonical(%q) = %q, want %q", in, got, want)
		}
	}
	// Non-existent paths still Clean.
	if got := canonical(filepath.Join(base, "a", "..", "missing")); got != filepath.Join(base, "missing") {
		t.Errorf("canonical on missing path = %q", got)
	}
}

func TestUpsertFoldsEquivalentPaths(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	base := t.TempDir()
	real := filepath.Join(base, "app")
	if err := os.MkdirAll(real, 0755); err != nil {
		t.Fatal(err)
	}

	first := Project{ID: "id-1", Name: "app", Slug: "app", Path: real, Channel: "sandbox", FlowStage: "current"}
	if err := r.Upsert(first); err != nil {
		t.Fatal(err)
	}
	// Trailing separator + parent-jitter + (on case-insensitive FS) case jitter
	// must resolve to the same row, not a duplicate.
	jitter := filepath.Join(base, "sub", "..", "app") + string(os.PathSeparator)
	if runtime.GOOS == "windows" {
		jitter = strings.ToUpper(base[:1]) + jitter[1:]
	}
	second := Project{ID: "id-2", Name: "app2", Slug: "app2", Path: jitter, Channel: "sandbox", FlowStage: "current"}
	if err := r.Upsert(second); err != nil {
		t.Fatal(err)
	}

	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d rows, want 1", len(list))
	}
	if list[0].ID != "id-1" || list[0].Slug != "app" {
		t.Errorf("rescan replaced identity: %+v", list[0])
	}
}

func TestDiscoverNeverTouchesRegistryOwnedFields(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	dir := filepath.Join(t.TempDir(), "myproj")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	full := Project{ID: "id-1", Name: "My Proj", Slug: "my-proj", Path: dir, Channel: "sandbox", FlowStage: "building", Language: "go", CreatedAt: created, OnDisk: true, Registered: true}
	if err := r.Upsert(full); err != nil {
		t.Fatal(err)
	}
	if err := r.SetHealth("id-1", 88); err != nil {
		t.Fatal(err)
	}

	// Scanner output: zero health, folder-derived name/stage, new language.
	scanned := Project{ID: "fresh-id", Name: "folder", Slug: "folder", Path: dir, Channel: "sandbox", FlowStage: "source", Language: "rust", Stack: []string{"cargo"}, HasGit: true, LastScannedAt: time.Now(), OnDisk: true}
	if err := r.Discover(scanned); err != nil {
		t.Fatal(err)
	}

	got, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	p := got[0]
	if p.ID != "id-1" || p.Slug != "my-proj" {
		t.Errorf("identity overwritten: id=%q slug=%q", p.ID, p.Slug)
	}
	if p.Name != "My Proj" || p.FlowStage != "building" {
		t.Errorf("registry-owned fields overwritten: name=%q stage=%q", p.Name, p.FlowStage)
	}
	if p.HealthScore != 88 {
		t.Errorf("health wiped to %d", p.HealthScore)
	}
	if !p.CreatedAt.Equal(created) {
		t.Errorf("created_at overwritten: %v", p.CreatedAt)
	}
	if p.Language != "rust" || !p.HasGit || p.Stack == nil || p.Stack[0] != "cargo" {
		t.Errorf("discovery fields not refreshed: %+v", p)
	}
}

func TestDuplicateFolderNamesGetStableDistinctSlugs(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	root := t.TempDir()
	for _, ch := range []string{"sandbox", "archive"} {
		if err := os.MkdirAll(filepath.Join(root, ch, "app"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	for i, ch := range []string{"sandbox", "archive"} {
		p := Project{ID: fmt.Sprintf("id-%d", i), Name: "app", Slug: "app", Path: filepath.Join(root, ch, "app"), Channel: ch, FlowStage: "source", CreatedAt: now, OnDisk: true}
		if err := r.Discover(p); err != nil {
			t.Fatalf("discover %s: %v", ch, err)
		}
	}
	// Rescan must not shuffle the persisted slugs.
	for i, ch := range []string{"sandbox", "archive"} {
		p := Project{ID: fmt.Sprintf("id-%d", i), Name: "app", Slug: "app", Path: filepath.Join(root, ch, "app"), Channel: ch, FlowStage: "source", CreatedAt: now, OnDisk: true}
		if err := r.Discover(p); err != nil {
			t.Fatalf("rescan %s: %v", ch, err)
		}
	}

	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d rows, want 2", len(list))
	}
	slugs := map[string]bool{}
	for _, p := range list {
		slugs[p.Slug] = true
	}
	if !slugs["app"] || !slugs["app-2"] {
		t.Errorf("slugs = %v, want app and app-2", slugs)
	}
}

func TestApplyDiscoveryWarnsInsteadOfAborting(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	// Same primary key on both rows: second insert must warn, not abort.
	ps := []Project{
		{ID: "dup", Name: "a", Slug: "a", Path: a, Channel: "sandbox", FlowStage: "source", CreatedAt: now, OnDisk: true},
		{ID: "dup", Name: "b", Slug: "b", Path: b, Channel: "sandbox", FlowStage: "source", CreatedAt: now, OnDisk: true},
	}
	warns, err := r.ApplyDiscovery(ps)
	if err != nil {
		t.Fatalf("ApplyDiscovery fatal: %v", err)
	}
	if len(warns) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warns))
	}
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Slug != "a" {
		t.Errorf("first project not committed: %+v", list)
	}
}

func TestMigrateSetsUserVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rivu.db")
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := r.DB.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) {
		t.Errorf("user_version = %d, want %d", v, len(migrations))
	}
	r.Close()

	// Re-open must be a no-op.
	if r, err = Open(path); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	r.Close()
}

func TestMigrateAdoptsLegacyDatabaseWithBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rivu.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Pre-versioning layout: tables present, user_version still 0.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,name TEXT NOT NULL,slug TEXT NOT NULL UNIQUE,path TEXT NOT NULL UNIQUE,channel TEXT NOT NULL,flow_stage TEXT NOT NULL,language TEXT,stack TEXT,has_git INTEGER DEFAULT 0,has_bank INTEGER DEFAULT 0,has_map INTEGER DEFAULT 0,health_score INTEGER DEFAULT 0,created_at DATETIME,last_opened_at DATETIME,last_scanned_at DATETIME,on_disk INTEGER DEFAULT 1,registered INTEGER DEFAULT 1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open legacy db: %v", err)
	}
	r.Close()
	if _, err := os.Stat(path + ".v0.bak"); err != nil {
		t.Errorf("expected backup before migrating legacy db: %v", err)
	}
}

func TestMigrateRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rivu.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if _, err := Open(path); err == nil {
		t.Fatal("Open accepted database from newer schema, want error")
	}
}

func TestForeignKeysCascade(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if _, err := r.DB.Exec(`INSERT INTO confluences(id,name) VALUES('c1','one')`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DB.Exec(`INSERT INTO health_snapshots(id,project_id,score,taken_at) VALUES('h1','missing-project',50,?)`, time.Now()); err == nil {
		t.Fatal("insert with unknown project_id succeeded, want FK violation")
	}
}

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
