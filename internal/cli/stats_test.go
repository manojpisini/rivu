package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatsOutput(t *testing.T) {
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
	if got := run("source", "stat1", "--no-git"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}

	out := captureStdout(t, func() {
		if code := run("stats"); code != 0 {
			t.Errorf("stats exit = %d", code)
		}
	})
	for _, want := range []string{"Projects: 1", "Median health", "Missing README", "By language:"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats text missing %q in %q", want, out)
		}
	}

	out = captureStdout(t, func() {
		if code := run("stats", "--json"); code != 0 {
			t.Errorf("stats --json exit = %d", code)
		}
	})
	var payload struct {
		Schema       int    `json:"schema"`
		Range        string `json:"range"`
		Total        int    `json:"total"`
		MedianHealth int    `json:"median_health"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("stats --json %q: %v", out, err)
	}
	if payload.Schema != 1 || payload.Range != "all" || payload.Total != 1 {
		t.Errorf("stats --json = %+v, want schema 1/all/1", payload)
	}

	out = captureStdout(t, func() {
		if code := run("stats", "--csv"); code != 0 {
			t.Errorf("stats --csv exit = %d", code)
		}
	})
	if !strings.HasPrefix(out, "metric,value\n") || !strings.Contains(out, "median_health,") || !strings.Contains(out, "flow_source,") {
		t.Errorf("stats --csv = %q, want metric,value rows", out)
	}

	// Range window is echoed and validated in the args stage.
	out = captureStdout(t, func() {
		if code := run("stats", "--range", "30d"); code != 0 {
			t.Errorf("--range 30d exit = %d", code)
		}
	})
	if !strings.Contains(out, "Range: 30d") {
		t.Errorf("--range 30d = %q, want Range line", out)
	}
	if got := run("stats", "--range", "5d"); got != 2 {
		t.Errorf("--range 5d exit = %d, want 2", got)
	}
	if got := run("stats", "--json", "--csv"); got != 2 {
		t.Errorf("--json --csv exit = %d, want 2", got)
	}
	if got := run("stats", "extra"); got != 2 {
		t.Errorf("positional arg exit = %d, want 2", got)
	}
}
