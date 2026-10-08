package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// seedBackup writes one small registry: two projects, one confluence
// with membership, a snapshot, an activity row and Current.
func seedBackup(t *testing.T) *Registry {
	t.Helper()
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	for _, p := range []Project{
		{ID: "p1", Name: "One", Slug: "one", Path: filepath.Join(t.TempDir(), "one"), FlowStage: "source", Channel: "00_Source", Stack: []string{"go"}, HealthScore: 77},
		{ID: "p2", Name: "Two", Slug: "two", Path: filepath.Join(t.TempDir(), "two"), FlowStage: "active", Channel: "01_Active"},
	} {
		if err := r.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.SetCurrent("p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateConfluence("ship", "notes here"); err != nil {
		t.Fatal(err)
	}
	if err := r.AttachConfluence("p1", "ship"); err != nil {
		t.Fatal(err)
	}
	if err := r.AddSnapshot("p1", 77, time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := r.LogActivity("p1", "opened"); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestExportImportRoundTrip(t *testing.T) {
	src := seedBackup(t)
	d, err := src.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if d.Schema != DumpSchema || d.DBVersion < 1 {
		t.Fatalf("schema=%d db_version=%d, want v%d and >=1", d.Schema, d.DBVersion, DumpSchema)
	}
	if len(d.Projects) != 2 || len(d.Confluences) != 1 || len(d.Links) != 1 ||
		len(d.Snapshots) != 1 || len(d.Activity) != 1 || len(d.Settings) == 0 {
		t.Fatalf("counts = %d/%d/%d/%d/%d/%d, want 2/1/1/1/1/>0",
			len(d.Projects), len(d.Confluences), len(d.Links), len(d.Snapshots), len(d.Activity), len(d.Settings))
	}

	// A fresh registry restores the dump exactly.
	dst, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if err := dst.Import(d); err != nil {
		t.Fatalf("Import: %v", err)
	}
	ps, err := dst.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("projects after restore = %d, want 2", len(ps))
	}
	for _, p := range ps {
		if p.Slug == "one" && (p.HealthScore != 77 || len(p.Stack) != 1 || p.Stack[0] != "go") {
			t.Errorf("one restored as %+v, want health 77 and stack [go]", p)
		}
	}
	cur, err := dst.Current()
	if err != nil || cur.ID != "p1" {
		t.Errorf("Current = %v (err %v), want p1", cur.ID, err)
	}
	ids, err := dst.ConfluenceProjectIDs("ship")
	if err != nil || len(ids) != 1 || ids[0] != "p1" {
		t.Errorf("membership = %v (err %v), want [p1]", ids, err)
	}
	if snaps, err := dst.HealthSnapshots("p1", 30); err != nil || len(snaps) != 1 || snaps[0].Score != 77 {
		t.Errorf("snapshots = %+v (err %v), want one score 77", snaps, err)
	}
	if act, err := dst.RecentActivity(10); err != nil || len(act) != 1 || act[0].Event != "opened" {
		t.Errorf("activity = %+v (err %v), want one opened row", act, err)
	}
	// Importing again over the restored data is idempotent.
	if err := dst.Import(d); err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if ps, _ := dst.List(); len(ps) != 2 {
		t.Errorf("projects after re-import = %d, want 2", len(ps))
	}
}

func TestImportRollsBackOnInvalidDump(t *testing.T) {
	r := seedBackup(t)
	d, err := r.Export()
	if err != nil {
		t.Fatal(err)
	}
	// A dump with duplicate slugs violates UNIQUE(slug): the second
	// insert fails and the whole restore must roll back, leaving the
	// original registry untouched.
	d.Projects[1].Slug = d.Projects[0].Slug
	if err := r.Import(d); err == nil {
		t.Fatal("duplicate slug must fail the import")
	}
	ps, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("projects after failed import = %d, want the original 2 (rollback)", len(ps))
	}
	for _, p := range ps {
		if p.Slug != "one" && p.Slug != "two" {
			t.Errorf("row %s survived the rollback", p.Slug)
		}
	}
}

// TestImportRollsBackOnEachTableError corrupts one table of an
// otherwise valid dump and proves the restore fails and rolls back —
// one guard per insert loop (safety: a half-restored registry is
// never committed).
func TestImportRollsBackOnEachTableError(t *testing.T) {
	cases := []struct {
		name    string
		corrupt func(*Dump)
	}{
		{"duplicate confluence", func(d *Dump) {
			d.Confluences = append(d.Confluences, d.Confluences[0])
		}},
		{"duplicate link", func(d *Dump) {
			d.Links = append(d.Links, d.Links[0])
		}},
		{"duplicate snapshot", func(d *Dump) {
			d.Snapshots = append(d.Snapshots, d.Snapshots[0])
		}},
		{"duplicate activity", func(d *Dump) {
			d.Activity = append(d.Activity, d.Activity[0])
		}},
		{"duplicate setting", func(d *Dump) {
			d.Settings = append(d.Settings, d.Settings[0])
		}},
		{"link to unknown project", func(d *Dump) {
			d.Links[0].ProjectID = "missing"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := seedBackup(t)
			d, err := r.Export()
			if err != nil {
				t.Fatal(err)
			}
			tc.corrupt(&d)
			if err := r.Import(d); err == nil {
				t.Fatalf("%s: import must fail", tc.name)
			}
			ps, err := r.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(ps) != 2 {
				t.Fatalf("projects after failed import = %d, want the original 2 (rollback)", len(ps))
			}
			if cs, _ := r.ListConfluences(); len(cs) != 1 {
				t.Fatalf("confluences after failed import = %d, want 1", len(cs))
			}
		})
	}
}

func TestExportAndBackupFailOnClosedRegistry(t *testing.T) {
	r := seedBackup(t)
	r.Close()
	if _, err := r.Export(); err == nil {
		t.Error("Export on a closed registry must error")
	}
	if _, err := r.BackupTo("preimport-closed"); err == nil {
		t.Error("BackupTo on a closed registry must error")
	}
}

// TestExportFailsLoudlyOnUnreadableData: a backup must error rather
// than silently skip rows it cannot read — corrupt values and missing
// tables both abort the export.
func TestExportFailsLoudlyOnUnreadableData(t *testing.T) {
	cases := []struct {
		name    string
		corrupt func(*testing.T, *Registry)
	}{
		{"corrupt snapshot score", func(t *testing.T, r *Registry) {
			if _, err := r.DB.Exec(`UPDATE health_snapshots SET score='not-a-number'`); err != nil {
				t.Fatal(err)
			}
		}},
		{"corrupt activity time", func(t *testing.T, r *Registry) {
			if _, err := r.DB.Exec(`UPDATE activity_log SET occurred_at='not-a-date'`); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing projects table", func(t *testing.T, r *Registry) {
			if _, err := r.DB.Exec(`DROP TABLE projects`); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing confluences table", func(t *testing.T, r *Registry) {
			if _, err := r.DB.Exec(`DROP TABLE confluences`); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing membership table", func(t *testing.T, r *Registry) {
			if _, err := r.DB.Exec(`DROP TABLE project_confluences`); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing settings table", func(t *testing.T, r *Registry) {
			if _, err := r.DB.Exec(`DROP TABLE settings`); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := seedBackup(t)
			tc.corrupt(t, r)
			if _, err := r.Export(); err == nil {
				t.Fatalf("%s: Export must fail instead of writing a partial backup", tc.name)
			}
		})
	}
}

func TestParseDumpRejectsForeignFiles(t *testing.T) {
	if _, err := ParseDump([]byte("not json")); err == nil {
		t.Error("garbage must be rejected")
	}
	if _, err := ParseDump([]byte("{}")); err == nil {
		t.Error("a file without schema must be rejected, not read as empty")
	}
	if _, err := ParseDump([]byte(`{"schema":99}`)); err == nil {
		t.Error("an unknown schema must be rejected")
	}
	if d, err := ParseDump([]byte(`{"schema":1}`)); err != nil || d.Schema != 1 {
		t.Errorf("schema 1 = %+v, %v — must parse", d, err)
	}
}

func TestBackupToSnapshotsDatabaseFile(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	path, err := r.BackupTo("preimport-test")
	if err != nil {
		t.Fatalf("BackupTo: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	// VACUUM INTO refuses to clobber an existing file.
	if _, err := r.BackupTo("preimport-test"); err == nil {
		t.Error("a second backup to the same tag must fail instead of overwriting")
	}
}
