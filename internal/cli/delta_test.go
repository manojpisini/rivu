package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDeltaCommand(t *testing.T) {
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
	run := func(args ...string) int {
		t.Helper()
		return Run("test", "dev", "unknown", args)
	}

	if got := run("source", "to-delta", "--git=false"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}
	src := filepath.Join(ws, "00_Source", "to-delta")

	// Without --yes: exit 4 and nothing moves.
	if got := run("delta", "to-delta"); got != 4 {
		t.Errorf("delta without --yes exit = %d, want 4", got)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("project moved without --yes: %v", err)
	}
	// Dry-run previews with exit 0, still no move.
	if got := run("delta", "to-delta", "--dry-run"); got != 0 {
		t.Errorf("delta --dry-run exit = %d, want 0", got)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("dry-run moved the project: %v", err)
	}
	// archive alias performs the move.
	if got := run("archive", "to-delta", "--yes"); got != 0 {
		t.Errorf("archive --yes exit = %d, want 0", got)
	}
	dst := filepath.Join(ws, "90_Delta", "to-delta")
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("project not moved to 90_Delta: %v", err)
	}
	// No-op second delta: still exit 0.
	if got := run("delta", "to-delta", "--yes"); got != 0 {
		t.Errorf("delta no-op exit = %d, want 0", got)
	}
	// Unknown project: exit 3.
	if got := run("delta", "ghost", "--yes"); got != 3 {
		t.Errorf("delta ghost exit = %d, want 3", got)
	}
}
