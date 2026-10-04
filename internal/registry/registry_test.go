package registry

import (
	"database/sql"
	"errors"
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
	if warns, err := r.ApplyDiscovery([]Project{scanned}); err != nil || len(warns) != 0 {
		t.Fatalf("ApplyDiscovery: warns=%v err=%v", warns, err)
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
	scan := func() []Project {
		var ps []Project
		for i, ch := range []string{"sandbox", "archive"} {
			ps = append(ps, Project{ID: fmt.Sprintf("id-%d", i), Name: "app", Slug: "app", Path: filepath.Join(root, ch, "app"), Channel: ch, FlowStage: "source", CreatedAt: now, OnDisk: true})
		}
		return ps
	}
	if warns, err := r.ApplyDiscovery(scan()); err != nil || len(warns) != 0 {
		t.Fatalf("first scan: warns=%v err=%v", warns, err)
	}
	// Rescan must not shuffle the persisted slugs.
	if warns, err := r.ApplyDiscovery(scan()); err != nil || len(warns) != 0 {
		t.Fatalf("rescan: warns=%v err=%v", warns, err)
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

func TestScanFlagsVanishedProjectsMissing(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	root := t.TempDir()
	gone := filepath.Join(root, "gone")
	stays := filepath.Join(root, "stays")
	now := time.Now()
	for _, d := range []string{gone, stays} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for i, d := range []string{gone, stays} {
		if err := r.Upsert(Project{ID: fmt.Sprintf("id-%d", i), Name: "p", Slug: fmt.Sprintf("p-%d", i), Path: d, Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true, Registered: true}); err != nil {
			t.Fatal(err)
		}
	}

	// Next scan sees only "stays": "gone" must be flagged missing.
	if warns, err := r.ApplyDiscovery([]Project{{ID: "id-1", Name: "p", Slug: "p-1", Path: stays, Channel: "00_Source", FlowStage: "source", LastScannedAt: now, OnDisk: true}}); err != nil || len(warns) != 0 {
		t.Fatalf("ApplyDiscovery: warns=%v err=%v", warns, err)
	}
	st, err := r.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Missing) != 1 || st.Missing[0].Slug != "p-0" {
		t.Errorf("missing = %+v, want p-0", st.Missing)
	}
	if p, _ := r.Find("p-1"); !p.OnDisk {
		t.Errorf("found project flagged missing: %+v", p)
	}
}

func TestStatesCategorizeMismatches(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	rows := []Project{
		{ID: "m", Name: "m", Slug: "m", Path: "/m", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: false, Registered: true},
		{ID: "u", Name: "u", Slug: "u", Path: "/u", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true, Registered: false},
		{ID: "s", Name: "s", Slug: "s", Path: "/s", Channel: "00_Source", FlowStage: "active", CreatedAt: now, OnDisk: true, Registered: true},
		{ID: "ok", Name: "ok", Slug: "ok", Path: "/ok", Channel: "01_Active", FlowStage: "active", CreatedAt: now, OnDisk: true, Registered: true},
	}
	if _, err := r.ApplyDiscovery(rows); err != nil {
		t.Fatal(err)
	}
	st, err := r.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Missing) != 1 || st.Missing[0].ID != "m" {
		t.Errorf("missing = %+v", st.Missing)
	}
	if len(st.Unregistered) != 1 || st.Unregistered[0].ID != "u" {
		t.Errorf("unregistered = %+v", st.Unregistered)
	}
	if len(st.StageMismatch) != 1 || st.StageMismatch[0].ID != "s" {
		t.Errorf("stageMismatch = %+v", st.StageMismatch)
	}
}

func TestDiscoveryRefreshesChannelForStageMismatch(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	dir := filepath.Join(t.TempDir(), "moved")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Registered as active; folder now sits in 00_Source.
	p := Project{ID: "id-1", Name: "moved", Slug: "moved", Path: dir, Channel: "01_Active", FlowStage: "active", CreatedAt: now, OnDisk: true, Registered: true}
	if err := r.Upsert(p); err != nil {
		t.Fatal(err)
	}
	p.Channel = "00_Source"
	if warns, err := r.ApplyDiscovery([]Project{p}); err != nil || len(warns) != 0 {
		t.Fatalf("ApplyDiscovery: warns=%v err=%v", warns, err)
	}
	st, err := r.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.StageMismatch) != 1 {
		t.Fatalf("stageMismatch = %+v, want 1", st.StageMismatch)
	}
	got, _ := r.Find("moved")
	if got.Channel != "00_Source" || got.FlowStage != "active" {
		t.Errorf("channel not refreshed or stage changed: ch=%q flow=%q", got.Channel, got.FlowStage)
	}
}

func TestResolveLadder(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	for i, p := range []Project{
		{ID: "1111-aaaa", Name: "Web Shop", Slug: "web-shop", Path: "/w1", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true},
		{ID: "2222-bbbb", Name: "web api", Slug: "web-api", Path: "/w2", Channel: "01_Active", FlowStage: "active", CreatedAt: now, OnDisk: true},
		{ID: "3333-cccc", Name: "mobile", Slug: "mobile", Path: "/w3", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true},
	} {
		if err := r.Upsert(p); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	// Exact slug.
	if p, err := r.Resolve("web-shop"); err != nil || p.Slug != "web-shop" {
		t.Errorf("exact slug: p=%+v err=%v", p, err)
	}
	// Case-insensitive exact name.
	if p, err := r.Resolve("WEB API"); err != nil || p.Slug != "web-api" {
		t.Errorf("ci name: p=%+v err=%v", p, err)
	}
	// Unique id prefix.
	if p, err := r.Resolve("1111"); err != nil || p.Slug != "web-shop" {
		t.Errorf("id prefix: p=%+v err=%v", p, err)
	}
	// Fuzzy substring.
	if p, err := r.Resolve("shop"); err != nil || p.Slug != "web-shop" {
		t.Errorf("fuzzy: p=%+v err=%v", p, err)
	}
	// Not found, with did-you-mean hint.
	_, err = r.Resolve("web-shpp")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "web-shop") {
		t.Errorf("missing did-you-mean hint: %v", err)
	}
}

func TestResolveAmbiguousListsCandidates(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	for i, p := range []Project{
		{ID: "id-1", Name: "web", Slug: "web-a", Path: "/a", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true},
		{ID: "id-2", Name: "web", Slug: "web-b", Path: "/b", Channel: "00_Source", FlowStage: "source", CreatedAt: now, OnDisk: true},
	} {
		if err := r.Upsert(p); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	_, err = r.Resolve("web")
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("want ErrAmbiguous, got %v", err)
	}
	if !strings.Contains(err.Error(), "web-a") || !strings.Contains(err.Error(), "web-b") {
		t.Errorf("candidates not listed: %v", err)
	}
}

func TestListUsesLifecycleOrder(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	now := time.Now()
	// Insert in reverse lifecycle order; alphabetical would differ too.
	for i, flow := range []string{"delta", "research", "maintenance", "active", "source"} {
		p := Project{ID: fmt.Sprintf("id-%d", i), Name: "p", Slug: flow, Path: "/" + flow, Channel: "00_Source", FlowStage: flow, CreatedAt: now, OnDisk: true}
		if err := r.Upsert(p); err != nil {
			t.Fatalf("seed %s: %v", flow, err)
		}
	}
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"source", "active", "maintenance", "research", "delta"}
	for i, w := range want {
		if list[i].FlowStage != w {
			t.Fatalf("order[%d] = %s, want %s (full: %v)", i, list[i].FlowStage, w, list)
		}
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
