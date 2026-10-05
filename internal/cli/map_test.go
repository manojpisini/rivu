package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentSyncFlags(t *testing.T) {
	home := t.TempDir()
	// Windows 8.3 short paths: registry canonicalises via EvalSymlinks,
	// so the test must use the same spelling to find created files.
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
	if got := run("source", "mapme", "--no-git"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}
	proj := filepath.Join(ws, "00_Source", "mapme")
	agents := filepath.Join(proj, ".metadata", "agent", "AGENTS.md")

	// Source builds the Map, so a fresh project is already up to date.
	if got := run("agent", "sync", "mapme", "--check"); got != 0 {
		t.Errorf("--check fresh exit = %d, want 0", got)
	}
	out := captureStdout(t, func() {
		if code := run("agent", "sync", "mapme", "--dry-run"); code != 0 {
			t.Errorf("--dry-run fresh exit = %d", code)
		}
	})
	if !strings.Contains(out, "up to date") {
		t.Errorf("--dry-run fresh = %q, want up to date", out)
	}

	// A new top-level file makes PROJECT_MAP.md stale, and removing
	// AGENTS.md makes the check report both needs.
	if err := os.WriteFile(filepath.Join(proj, "notes.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(agents); err != nil {
		t.Fatal(err)
	}
	if got := run("agent", "sync", "mapme", "--check"); got != 5 {
		t.Errorf("--check stale exit = %d, want 5", got)
	}
	out = captureStdout(t, func() {
		if code := run("agent", "sync", "mapme", "--dry-run"); code != 0 {
			t.Errorf("--dry-run stale exit = %d", code)
		}
	})
	if !strings.Contains(out, "create .metadata/agent/AGENTS.md") || !strings.Contains(out, "update .metadata/agent/PROJECT_MAP.md") {
		t.Errorf("--dry-run stale = %q, want both changes", out)
	}
	if _, err := os.Stat(agents); err == nil {
		t.Error("--dry-run wrote AGENTS.md")
	}

	// Build, then everything is up to date again.
	if got := run("agent", "sync", "mapme"); got != 0 {
		t.Fatalf("sync exit = %d", got)
	}
	if got := run("agent", "sync", "mapme", "--check"); got != 0 {
		t.Errorf("--check after sync exit = %d, want 0", got)
	}

	// AGENTS.md is create-if-missing: sync never clobbers it.
	hand := "hand-written rules"
	if err := os.WriteFile(agents, []byte(hand), 0644); err != nil {
		t.Fatal(err)
	}
	if got := run("agent", "sync", "mapme"); got != 0 {
		t.Fatalf("resync exit = %d", got)
	}
	got, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != hand {
		t.Error("AGENTS.md was clobbered")
	}

	// --all syncs every project; --all with a project arg is usage 2.
	if got := run("agent", "sync", "--all"); got != 0 {
		t.Errorf("--all exit = %d", got)
	}
	if got := run("agent", "sync", "mapme", "--all"); got != 2 {
		t.Errorf("--all with project exit = %d, want 2", got)
	}
}
