package service

import (
	"os"
	"testing"
)

// TestExportImportReplacesRowsWithBackup backs `rivu db export|import`
// (P5.13): the dump captures the registry, Import rolls it back over
// newer rows and always snapshots the previous database first.
func TestExportImportReplacesRowsWithBackup(t *testing.T) {
	a := openTestApp(t)
	if _, err := a.Source("first", SourceOpts{Flow: "source"}); err != nil {
		t.Fatalf("Source: %v", err)
	}
	d, err := a.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(d.Projects) != 1 {
		t.Fatalf("exported projects = %d, want 1", len(d.Projects))
	}

	if _, err := a.Source("second", SourceOpts{Flow: "source"}); err != nil {
		t.Fatalf("Source second: %v", err)
	}
	if ps, _ := a.List(Filter{}); len(ps) != 2 {
		t.Fatalf("projects before restore = %d, want 2", len(ps))
	}

	res, err := a.Import(d)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Backup == "" {
		t.Fatal("Import must report where it backed up the previous registry")
	}
	if _, err := os.Stat(res.Backup); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	ps, err := a.List(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Slug != "first" {
		t.Fatalf("projects after restore = %+v, want just first", ps)
	}
}

// TestImportBadDumpKeepsRegistry: a dump that cannot be applied fails
// after the pre-import snapshot, and the registry stays as it was.
func TestImportBadDumpKeepsRegistry(t *testing.T) {
	a := openTestApp(t)
	if _, err := a.Source("keep", SourceOpts{Flow: "source"}); err != nil {
		t.Fatalf("Source: %v", err)
	}
	if _, err := a.Source("other", SourceOpts{Flow: "source"}); err != nil {
		t.Fatalf("Source other: %v", err)
	}
	d, err := a.Export()
	if err != nil {
		t.Fatal(err)
	}
	d.Projects[1].Slug = d.Projects[0].Slug // UNIQUE(slug) violation on restore
	if _, err := a.Import(d); err == nil {
		t.Fatal("corrupt dump must fail the import")
	}
	ps, err := a.List(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("projects after failed import = %+v, want both untouched", ps)
	}
}
