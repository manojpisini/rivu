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

// TestDoctorFixFlow (P6.03): --fix prints the plan, --dry-run stops
// there, applying needs --yes (exit 4), and only missing files are
// created; a bogus --fix id is a usage error.
func TestDoctorFixFlow(t *testing.T) {
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
	if got := run("source", "fixme", "--no-git"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}
	out := captureStdout(t, func() {
		if code := run("path", "fixme"); code != 0 {
			t.Errorf("path exit = %d", code)
		}
	})
	path := strings.TrimSpace(out)
	readme := filepath.Join(path, "README.md")
	if _, err := os.Stat(readme); err == nil {
		t.Fatal("source must not create README.md here")
	}

	// Unknown id: usage error before anything runs.
	if got := run("doctor", "fixme", "--fix", "license"); got != 2 {
		t.Errorf("bogus --fix exit = %d, want 2", got)
	}

	// --dry-run prints the plan and changes nothing.
	out = captureStdout(t, func() {
		if code := run("doctor", "fixme", "--fix", "readme", "--dry-run"); code != 0 {
			t.Errorf("dry-run exit = %d", code)
		}
	})
	if !strings.Contains(out, "create README.md") || !strings.Contains(out, "fixme:") {
		t.Errorf("dry-run plan = %q, want the readme line", out)
	}
	if _, err := os.Stat(readme); err == nil {
		t.Error("dry-run must not create README.md")
	}

	// Without --yes the apply is refused (exit 4).
	if got := run("doctor", "fixme", "--fix", "readme"); got != 4 {
		t.Errorf("no --yes exit = %d, want 4", got)
	}
	if _, err := os.Stat(readme); err == nil {
		t.Error("refused apply must not create README.md")
	}

	// --yes applies, reports it on stderr, and the re-run report shows
	// the README check passing.
	errOut := captureStderr(t, func() {
		out = captureStdout(t, func() {
			if code := run("doctor", "fixme", "--fix", "readme", "--yes"); code != 0 {
				t.Errorf("--yes exit = %d", code)
			}
		})
	})
	if !strings.Contains(errOut, "applied 1 fix(es)") {
		t.Errorf("stderr = %q, want the applied count", errOut)
	}
	if _, err := os.Stat(readme); err != nil {
		t.Fatalf("README.md missing after --yes: %v", err)
	}
	if !strings.Contains(out, "Health") {
		t.Errorf("report after fix = %q, want the health report", out)
	}

	// Second run: nothing left to fix.
	errOut = captureStderr(t, func() {
		if code := run("doctor", "fixme", "--fix", "readme", "--yes"); code != 0 {
			t.Errorf("second --fix exit = %d", code)
		}
	})
	if !strings.Contains(errOut, "nothing to fix") {
		t.Errorf("stderr = %q, want nothing-to-fix notice", errOut)
	}
}

// TestDoctorMissingProject (D-07): a registered folder that vanished
// reports MISSING — not Health 0/100 — in text, in --json, and the
// failing on-disk check marks with ✗ (error severity).
func TestDoctorMissingProject(t *testing.T) {
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
	if got := run("source", "gone", "--no-git"); got != 0 {
		t.Fatalf("source exit = %d", got)
	}
	out := captureStdout(t, func() {
		if code := run("path", "gone"); code != 0 {
			t.Errorf("path exit = %d", code)
		}
	})
	if err := os.RemoveAll(strings.TrimSpace(out)); err != nil {
		t.Fatal(err)
	}

	out = captureStdout(t, func() {
		if code := run("doctor", "gone"); code != 0 {
			t.Errorf("doctor exit = %d", code)
		}
	})
	if !strings.Contains(out, "MISSING") || !strings.Contains(out, "✗") {
		t.Errorf("text report = %q, want MISSING with the ✗ mark", out)
	}
	if strings.Contains(out, "Health 0/100") {
		t.Errorf("must not fake a 0/100 score: %q", out)
	}

	out = captureStdout(t, func() {
		if code := run("doctor", "gone", "--json"); code != 0 {
			t.Errorf("doctor --json exit = %d", code)
		}
	})
	if !strings.Contains(out, `"missing":true`) || !strings.Contains(out, `"slug":"gone"`) {
		t.Errorf("json = %q, want missing:true for gone", out)
	}
}
