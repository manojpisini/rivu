package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDryRunRendersFullPlan(t *testing.T) {
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
	if got := run("source", "planned", "--no-git"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}

	// Source dry-run lists every plan section it will perform.
	out := captureStdout(t, func() {
		if code := run("source", "preview", "--no-git", "--dry-run"); code != 0 {
			t.Errorf("source --dry-run exit = %d", code)
		}
	})
	for _, want := range []string{"DRY RUN: create", "create:", "bank:", "registry:", filepath.Join(ws, "00_Source", "preview")} {
		if !strings.Contains(out, want) {
			t.Errorf("source dry-run missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(ws, "00_Source", "preview")); err == nil {
		t.Error("dry-run created the project folder")
	}

	// Flow dry-run renders move, bank and registry sections.
	out = captureStdout(t, func() {
		if code := run("flow", "planned", "--to", "active", "--dry-run"); code != 0 {
			t.Errorf("flow --dry-run exit = %d", code)
		}
	})
	for _, want := range []string{"DRY RUN: move planned", "move:", "bank:", "registry:", "00_Source", "01_Active"} {
		if !strings.Contains(out, want) {
			t.Errorf("flow dry-run missing %q:\n%s", want, out)
		}
	}
	// Still in the source folder — nothing moved.
	if _, err := os.Stat(filepath.Join(ws, "00_Source", "planned")); err != nil {
		t.Errorf("dry-run moved the project: %v", err)
	}
	// delta --dry-run uses the same renderer.
	out = captureStdout(t, func() {
		if code := run("delta", "planned", "--dry-run"); code != 0 {
			t.Errorf("delta --dry-run exit = %d", code)
		}
	})
	if !strings.Contains(out, "DRY RUN: move planned from source to delta") || !strings.Contains(out, "registry:") {
		t.Errorf("delta dry-run = %s, want full plan", out)
	}
}
