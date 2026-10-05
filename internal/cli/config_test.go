package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/manojpisini/rivu/internal/config"
)

func TestConfigSubcommands(t *testing.T) {
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
	run := func(args ...string) int {
		t.Helper()
		return Run("test", "dev", "unknown", args)
	}
	if got := run("init", "--root", ws); got != 0 {
		t.Fatalf("init exit = %d", got)
	}

	// show prints the path header and every effective value.
	out := captureStdout(t, func() {
		if code := run("config", "show"); code != 0 {
			t.Errorf("config show exit = %d", code)
		}
	})
	if !strings.Contains(out, "# ") || !strings.Contains(out, "[flow]") || !strings.Contains(out, "stale_threshold_days") {
		t.Errorf("config show = %q, want full TOML dump", out)
	}

	// get reads an effective value; unknown keys exit 3.
	out = captureStdout(t, func() {
		if code := run("config", "get", "flow.stale_threshold_days"); code != 0 {
			t.Errorf("config get exit = %d", code)
		}
	})
	if strings.TrimSpace(out) != "45" {
		t.Errorf("config get = %q, want 45", out)
	}
	if got := run("config", "get", "flow.stale_days"); got != 3 {
		t.Errorf("get unknown key exit = %d, want 3", got)
	}
	if got := run("config", "get", "flow"); got != 1 {
		t.Errorf("get section exit = %d, want 1", got)
	}

	// set writes and the value sticks; bad values and unknown keys
	// write nothing.
	if got := run("config", "set", "flow.stale_threshold_days", "30"); got != 0 {
		t.Fatalf("config set exit = %d", got)
	}
	out = captureStdout(t, func() {
		if code := run("config", "get", "flow.stale_threshold_days"); code != 0 {
			t.Errorf("config get after set exit = %d", code)
		}
	})
	if strings.TrimSpace(out) != "30" {
		t.Errorf("value after set = %q, want 30", out)
	}
	if got := run("config", "set", "flow.stale_threshold_days", "abc"); got != 1 {
		t.Errorf("set bad int exit = %d, want 1", got)
	}
	if got := run("config", "set", "flow.stale_days", "5"); got != 3 {
		t.Errorf("set unknown key exit = %d, want 3", got)
	}
	// dry-run reports old -> new and writes nothing.
	out = captureStdout(t, func() {
		if code := run("config", "set", "flow.stale_threshold_days", "99", "--dry-run"); code != 0 {
			t.Errorf("set --dry-run exit = %d", code)
		}
	})
	if !strings.Contains(out, `"30" -> "99"`) {
		t.Errorf("set --dry-run = %q, want old -> new", out)
	}
	out = captureStdout(t, func() {
		if code := run("config", "get", "flow.stale_threshold_days"); code != 0 {
			t.Errorf("config get exit = %d", code)
		}
	})
	if strings.TrimSpace(out) != "30" {
		t.Errorf("dry-run changed the value: %q", out)
	}

	// validate: good config passes; an unknown key is a warning (5);
	// invalid values are errors (1).
	if got := run("config", "validate"); got != 0 {
		t.Errorf("validate exit = %d, want 0", got)
	}
	p, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	// Partial files keep the workspace root so only the key under test
	// can fail validation.
	partial := func(body string) string {
		return fmt.Sprintf("[workspace]\nroot = %q\n%s", ws, body)
	}
	if err := os.WriteFile(p, []byte(partial("unknown_section = true\n[flow]\nstale_threshold_days = 30\n")), 0644); err != nil {
		t.Fatal(err)
	}
	if got := run("config", "validate"); got != 5 {
		t.Errorf("validate with unknown key exit = %d, want 5", got)
	}
	if err := os.WriteFile(p, []byte(partial("[flow]\nstale_threshold_days = 0\n[scanner]\nmax_depth = 0\n")), 0644); err != nil {
		t.Fatal(err)
	}
	if got := run("config", "validate"); got != 1 {
		t.Errorf("validate with bad values exit = %d, want 1", got)
	}

	// reset needs --yes (4); --dry-run previews; with --yes the
	// defaults come back and the old file is backed up.
	if got := run("config", "reset"); got != 4 {
		t.Errorf("reset without --yes exit = %d, want 4", got)
	}
	out = captureStdout(t, func() {
		if code := run("config", "reset", "--dry-run"); code != 0 {
			t.Errorf("reset --dry-run exit = %d", code)
		}
	})
	if !strings.Contains(out, "would reset") || !strings.Contains(out, ".bak") {
		t.Errorf("reset --dry-run = %q, want preview", out)
	}
	if got := run("config", "reset", "--yes"); got != 0 {
		t.Fatalf("reset --yes exit = %d", got)
	}
	if _, err := os.Stat(p + ".bak"); err != nil {
		t.Errorf("backup missing: %v", err)
	}
	out = captureStdout(t, func() {
		if code := run("config", "get", "flow.stale_threshold_days"); code != 0 {
			t.Errorf("config get exit = %d", code)
		}
	})
	if strings.TrimSpace(out) != "45" {
		t.Errorf("value after reset = %q, want 45", out)
	}

	// edit runs the resolved editor; echo prints its argument (cmd
	// builtin on Windows, binary elsewhere) and is harmless.
	editor := "echo"
	if runtime.GOOS == "windows" {
		editor = "cmd /c echo"
	}
	if got := run("config", "set", "editors.default", editor); got != 0 {
		t.Fatalf("set editors.default exit = %d", got)
	}
	out = captureStdout(t, func() {
		if code := run("config", "edit"); code != 0 {
			t.Errorf("config edit exit = %d", code)
		}
	})
	if !strings.Contains(out, p) {
		t.Errorf("config edit = %q, want editor invoked with %s", out, p)
	}
}
