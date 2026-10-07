package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Error("regular file must not count as a terminal")
	}
}

// TestTUIFallsBackWithoutTTY (P3.25): captured stdout is a pipe, so the
// launcher must print the dashboard instead of starting Bubble Tea.
func TestTUIFallsBackWithoutTTY(t *testing.T) {
	home := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
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
	if code := Run("test", "dev", "unknown", []string{"source", "tty1", "--no-git"}); code != 0 {
		t.Fatalf("source exit = %d", code)
	}

	var out, errs string
	out = captureStdout(t, func() {
		errs = captureStderr(t, func() {
			if code := Run("test", "dev", "unknown", []string{"tui"}); code != 0 {
				t.Errorf("tui exit = %d, want 0", code)
			}
		})
	})
	if !strings.Contains(out, "RIVU DASHBOARD") || !strings.Contains(out, "tty1") {
		t.Errorf("fallback dashboard = %q", out)
	}
	if !strings.Contains(errs, "no terminal attached") {
		t.Errorf("friendly notice missing from stderr: %q", errs)
	}
}
