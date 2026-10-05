package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexCommand(t *testing.T) {
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
	if got := run("source", "idx1", "--no-git"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}

	// index is its own command now (X-06): it rebuilds, it does not
	// print list rows.
	out := captureStdout(t, func() {
		if code := run("index"); code != 0 {
			t.Errorf("index exit = %d", code)
		}
	})
	if !strings.Contains(out, "Indexed 1 project(s)") || strings.Contains(out, "SLUG") {
		t.Errorf("index = %q, want rebuild summary, not list rows", out)
	}
	// A fresh workspace has no mismatches: attention line is absent.
	if strings.Contains(out, "Attention:") {
		t.Errorf("index = %q, want no attention line", out)
	}

	// list still lists (alias removal did not break it).
	out = captureStdout(t, func() {
		if code := run("list"); code != 0 {
			t.Errorf("list exit = %d", code)
		}
	})
	if !strings.Contains(out, "idx1") {
		t.Errorf("list = %q, want idx1 row", out)
	}

	// --json: schema 1, reconciled always an array, stage_mismatch empty.
	out = captureStdout(t, func() {
		if code := run("index", "--json"); code != 0 {
			t.Errorf("index --json exit = %d", code)
		}
	})
	var payload struct {
		Schema        int             `json:"schema"`
		Reconciled    []reconcileJSON `json:"reconciled"`
		StageMismatch []string        `json:"stage_mismatch"`
		Missing       []string        `json:"missing"`
		Projects      []projectJSON   `json:"projects"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("index --json %q: %v", out, err)
	}
	if payload.Schema != 1 || payload.Reconciled == nil || payload.StageMismatch == nil || payload.Missing == nil {
		t.Errorf("index --json = %+v, want schema 1 with non-nil arrays", payload)
	}
	if len(payload.Projects) != 1 {
		t.Errorf("projects = %d, want 1", len(payload.Projects))
	}
}
