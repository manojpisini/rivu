package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFlowBulk(t *testing.T) {
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
	for _, n := range []string{"alpha", "beta", "gamma", "delta-kid"} {
		if got := run("source", n, "--no-git"); got != 0 {
			t.Fatalf("source %s exit = %d", n, got)
		}
	}

	// Multiple positional args move together.
	if got := run("flow", "alpha", "beta", "--to", "active", "--yes"); got != 0 {
		t.Errorf("bulk flow exit = %d", got)
	}
	for _, n := range []string{"alpha", "beta"} {
		if _, err := os.Stat(filepath.Join(ws, "01_Active", n)); err != nil {
			t.Errorf("%s not moved: %v", n, err)
		}
	}
	// Mixed success and a ghost: the good one moves, exit 3 (not found).
	if got := run("flow", "gamma", "ghost", "--to", "maintenance", "--yes"); got != 3 {
		t.Errorf("bulk with ghost exit = %d, want 3", got)
	}
	if _, err := os.Stat(filepath.Join(ws, "02_Maintenance", "gamma")); err != nil {
		t.Errorf("gamma should still have moved: %v", err)
	}
	// Confirmation gate applies to bulk too.
	if got := run("flow", "delta-kid", "--to", "delta"); got != 4 {
		t.Errorf("bulk without --yes exit = %d, want 4", got)
	}

	// Piped stdin supplies the queries (newline list, # comment skipped).
	list := filepath.Join(home, "queries.txt")
	if err := os.WriteFile(list, []byte("# move these\nalpha\nbeta\n"), 0644); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(list)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = in
	defer func() { os.Stdin = old }()
	if got := run("flow", "--to", "research", "--yes"); got != 0 {
		t.Errorf("stdin bulk exit = %d", got)
	}
	_ = in.Close()
	os.Stdin = old
	for _, n := range []string{"alpha", "beta"} {
		if _, err := os.Stat(filepath.Join(ws, "03_Research", n)); err != nil {
			t.Errorf("%s not moved from stdin: %v", n, err)
		}
	}
	// Unknown target stage still fails fast (exit 1) with nothing moved.
	if got := run("flow", "delta-kid", "--to", "bogus", "--yes"); got != 1 {
		t.Errorf("bad target exit = %d, want 1", got)
	}
}
