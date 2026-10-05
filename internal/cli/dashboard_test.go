package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardSnapshot(t *testing.T) {
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
	run := func(args ...string) int {
		t.Helper()
		return Run("test", "dev", "unknown", args)
	}
	if got := run("source", "dash1", "--no-git"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}
	if got := run("source", "dash2", "--no-git"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}

	out := captureStdout(t, func() {
		if code := run("dashboard"); code != 0 {
			t.Errorf("dashboard exit = %d", code)
		}
	})
	for _, want := range []string{
		"RIVU DASHBOARD",
		"Root: " + ws,
		"Roots: 1",
		"Last scan:",
		"Health:",
		"Portfolio",
		"Total projects",
		"source (untriaged)",
		"Needs attention",
		"2 projects missing git",
		"missing README",
		"By language",
		"Recent activity",
		"sourced",
		"dash1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard missing %q in:\n%s", want, out)
		}
	}

	// Empty workspace: honest empty sections, exit 0.
	t.Setenv("RIVU_HOME", t.TempDir())
	emptyHome, _ := filepath.EvalSymlinks(os.Getenv("RIVU_HOME"))
	t.Setenv("RIVU_HOME", emptyHome)
	if err := os.WriteFile(filepath.Join(emptyHome, "config.toml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() {
		if code := run("dashboard"); code != 0 {
			t.Errorf("empty dashboard exit = %d", code)
		}
	})
	if !strings.Contains(out, "nothing needs attention") || !strings.Contains(out, "no activity recorded yet") {
		t.Errorf("empty dashboard = %q, want empty states", out)
	}
}
