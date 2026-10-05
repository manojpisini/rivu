package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListCommand(t *testing.T) {
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
	if got := run("source", "listed", "--git=false"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}

	// Header row appears; first data line is the seeded slug.
	out := captureStdout(t, func() {
		if code := run("list"); code != 0 {
			t.Errorf("list exit = %d", code)
		}
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "SLUG") || !strings.Contains(lines[1], "listed") {
		t.Errorf("list output = %q, want SLUG header then listed", out)
	}

	// Bad flag values are usage errors (exit 2), nothing on stdout.
	for _, args := range [][]string{{"list", "--flow", "bogus"}, {"list", "--sort", "bogus"}} {
		code := 0
		out := captureStdout(t, func() { code = run(args...) })
		if code != 2 {
			t.Errorf("%v exit = %d, want 2", args, code)
		}
		if out != "" {
			t.Errorf("%v wrote %q to stdout", args, out)
		}
	}

	// Valid filters run clean; unknown values narrow to nothing (exit 0).
	if got := run("list", "--flow", "source", "--lang", "no-such-lang"); got != 0 {
		t.Errorf("list filtered exit = %d", got)
	}

	// --json carries schema 1 and the seeded project.
	out = captureStdout(t, func() {
		if code := run("list", "--json"); code != 0 {
			t.Errorf("list --json exit = %d", code)
		}
	})
	var payload struct {
		Schema   int `json:"schema"`
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("list --json %q: %v", out, err)
	}
	if payload.Schema != 1 || len(payload.Projects) != 1 || payload.Projects[0].Slug != "listed" {
		t.Errorf("list --json = %+v, want schema 1 with one listed project", payload)
	}
}
