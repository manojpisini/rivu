package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorFlags(t *testing.T) {
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
	if got := run("source", "checky", "--no-git"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}

	// Text mode still prints the health report.
	out := captureStdout(t, func() {
		if code := run("doctor", "checky"); code != 0 {
			t.Errorf("doctor exit = %d", code)
		}
	})
	if !strings.Contains(out, "Health") || !strings.Contains(out, "Bank") {
		t.Errorf("doctor text = %q, want health report", out)
	}

	// --json: schema 1 with checks.
	out = captureStdout(t, func() {
		if code := run("doctor", "checky", "--json"); code != 0 {
			t.Errorf("doctor --json exit = %d", code)
		}
	})
	var payload struct {
		Schema  int `json:"schema"`
		Reports []struct {
			Slug   string `json:"slug"`
			Score  int    `json:"score"`
			Checks []struct {
				Name string `json:"name"`
				OK   bool   `json:"ok"`
			} `json:"checks"`
		} `json:"reports"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("doctor --json %q: %v", out, err)
	}
	if payload.Schema != 1 || len(payload.Reports) != 1 || payload.Reports[0].Slug != "checky" || len(payload.Reports[0].Checks) == 0 {
		t.Errorf("doctor --json = %+v, want schema 1 with checks", payload)
	}

	// --min-score gates: a bare project cannot reach 100, so 100 fails
	// with exit 5 (warnings); 0 disables and passes.
	if got := run("doctor", "checky", "--min-score", "100"); got != 5 {
		t.Errorf("--min-score 100 exit = %d, want 5", got)
	}
	if got := run("doctor", "checky", "--min-score", "0"); got != 0 {
		t.Errorf("--min-score 0 exit = %d, want 0", got)
	}
	// Usage errors: --all with a project, out-of-range score.
	if got := run("doctor", "checky", "--all"); got != 2 {
		t.Errorf("--all with project exit = %d, want 2", got)
	}
	if got := run("doctor", "--min-score", "101"); got != 2 {
		t.Errorf("--min-score 101 exit = %d, want 2", got)
	}
	// --all alone checks every project (here: one).
	out = captureStdout(t, func() {
		if code := run("doctor", "--all", "--json"); code != 0 {
			t.Errorf("doctor --all exit = %d", code)
		}
	})
	if !strings.Contains(out, `"slug":"checky"`) {
		t.Errorf("doctor --all --json = %q, want checky", out)
	}
}
