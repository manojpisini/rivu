package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDBExportImport drives `rivu db export|import` end to end (P5.13):
// export to stdout and to a file, dry-run previews, the confirm gate
// exits 4, and --yes restores with a .bak snapshot.
func TestDBExportImport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("[workspace]\nroot = %q\n", ws)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		if got := Run("test", "dev", "unknown", []string{"source", name, "--git=false"}); got != 0 {
			t.Fatalf("source %s exit = %d", name, got)
		}
	}

	// export -> stdout is a schema-1 document with both projects.
	type dumpShape struct {
		Schema   int `json:"schema"`
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	var out string
	code := 0
	out = captureStdout(t, func() {
		code = Run("test", "dev", "unknown", []string{"db", "export"})
	})
	if code != 0 {
		t.Fatalf("db export exit = %d", code)
	}
	var d dumpShape
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("export stdout is not JSON: %v", err)
	}
	if d.Schema != 1 || len(d.Projects) != 2 {
		t.Fatalf("schema=%d projects=%d, want 1 and 2", d.Schema, len(d.Projects))
	}

	// export to a file.
	bak := filepath.Join(home, "backup.json")
	if got := Run("test", "dev", "unknown", []string{"db", "export", bak}); got != 0 {
		t.Fatalf("db export file exit = %d", got)
	}
	if _, err := os.Stat(bak); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	// A third project lands after the backup.
	if got := Run("test", "dev", "unknown", []string{"source", "gamma", "--git=false"}); got != 0 {
		t.Fatalf("source gamma exit = %d", got)
	}

	// dry-run previews without touching the registry (exit 0).
	out = captureStdout(t, func() {
		code = Run("test", "dev", "unknown", []string{"db", "import", bak, "--dry-run"})
	})
	if code != 0 {
		t.Fatalf("dry-run exit = %d", code)
	}
	if !containsAll(out, "projects 2", "Restore") {
		t.Fatalf("dry-run plan = %q, want the counts", out)
	}
	if got := projectCount(t); got != 3 {
		t.Fatalf("dry-run changed the registry: %d projects, want 3", got)
	}

	// Without --yes the restore refuses (exit 4) and changes nothing.
	if got := Run("test", "dev", "unknown", []string{"db", "import", bak}); got != 4 {
		t.Fatalf("import without --yes exit = %d, want 4", got)
	}
	if got := projectCount(t); got != 3 {
		t.Fatalf("refused import changed the registry: %d projects, want 3", got)
	}

	// --yes restores the two-project state and keeps a .bak of it.
	out = captureStdout(t, func() {
		code = Run("test", "dev", "unknown", []string{"db", "import", bak, "--yes"})
	})
	if code != 0 {
		t.Fatalf("import --yes exit = %d", code)
	}
	if !containsAll(out, "backed up to") {
		t.Fatalf("import summary = %q, want the backup path", out)
	}
	if got := projectCount(t); got != 2 {
		t.Fatalf("projects after restore = %d, want 2", got)
	}
	baks, err := filepath.Glob(filepath.Join(home, "*.db.preimport-*.bak"))
	if err != nil || len(baks) == 0 {
		t.Fatalf("pre-import backup glob = %v (err %v), want one file", baks, err)
	}

	// A foreign file is an error, not an empty restore.
	junk := filepath.Join(home, "junk.json")
	if err := os.WriteFile(junk, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := Run("test", "dev", "unknown", []string{"db", "import", junk, "--yes"}); got != 1 {
		t.Fatalf("foreign file exit = %d, want 1", got)
	}
	// Missing argument is a usage error (exit 2).
	if got := Run("test", "dev", "unknown", []string{"db", "import"}); got != 2 {
		t.Fatalf("no-arg import exit = %d, want 2", got)
	}
}

// projectCount re-reads the registry through `db export --json`-free
// stdout: export is the read path the restore must agree with.
func projectCount(t *testing.T) int {
	t.Helper()
	out := captureStdout(t, func() {
		if code := Run("test", "dev", "unknown", []string{"db", "export"}); code != 0 {
			t.Fatalf("db export exit = %d", code)
		}
	})
	var d struct {
		Projects []json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("export: %v", err)
	}
	return len(d.Projects)
}

func containsAll(s string, wants ...string) bool {
	for _, w := range wants {
		if !strings.Contains(s, w) {
			return false
		}
	}
	return true
}
