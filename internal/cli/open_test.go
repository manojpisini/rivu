package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenDryRunAndEditor(t *testing.T) {
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
	// Registry canonicalises paths (Windows 8.3, symlinks).
	if x, err := filepath.EvalSymlinks(ws); err == nil {
		ws = x
	}
	run := func(args ...string) int {
		t.Helper()
		return Run("test", "dev", "unknown", args)
	}
	// Second source becomes Current (source sets Current each time).
	for _, n := range []string{"first", "second"} {
		if got := run("source", n, "--no-git"); got != 0 {
			t.Fatalf("source %s exit = %d", n, got)
		}
	}

	// --dry-run prints the argv with the target path, launches nothing.
	out := captureStdout(t, func() {
		if code := run("open", "first", "--dry-run", "--editor", "nvim"); code != 0 {
			t.Errorf("open --dry-run exit = %d", code)
		}
	})
	out = strings.TrimSpace(out)
	wantPath := filepath.Join(ws, "00_Source", "first")
	if !strings.HasPrefix(out, "nvim ") || !strings.Contains(out, wantPath) {
		t.Errorf("dry-run = %q, want nvim + %s", out, wantPath)
	}
	// Dry-run wrote nothing to the registry: Current is still "second".
	out = captureStdout(t, func() {
		if code := run("path"); code != 0 {
			t.Errorf("path exit = %d", code)
		}
	})
	if !strings.Contains(out, filepath.Join(ws, "00_Source", "second")) {
		t.Errorf("Current after dry-run = %q, want second unchanged", out)
	}
	// Unknown project: exit 3 in dry-run mode too.
	if code := run("open", "ghost", "--dry-run"); code != 3 {
		t.Errorf("open ghost --dry-run exit = %d, want 3", code)
	}
}
