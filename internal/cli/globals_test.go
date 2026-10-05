package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// newGlobalsHome writes a config with an unknown key (which triggers a
// config warning on every withApp command) and an existing workspace.
func newGlobalsHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("[workspace]\nroot = %q\nbogus_key = true\n", ws)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalJSONBeforeSubcommand(t *testing.T) {
	newGlobalsHome(t)
	out := captureStdout(t, func() {
		if got := Run("test", "dev", "unknown", []string{"--json", "version"}); got != 0 {
			t.Errorf("--json version exit = %d", got)
		}
	})
	if !strings.Contains(out, `"schema"`) {
		t.Errorf("--json version output = %q, want schema field", out)
	}
}

func TestGlobalQuietSuppressesWarnings(t *testing.T) {
	newGlobalsHome(t)
	// The bogus config key makes withApp print a warning — and still
	// exit 5, quiet or not.
	var code int
	errOut := captureStderr(t, func() {
		code = Run("test", "dev", "unknown", []string{"list"})
	})
	if code != 5 || !strings.Contains(errOut, "warning:") {
		t.Fatalf("list exit = %d, stderr = %q, want exit 5 with warning", code, errOut)
	}
	errOut = captureStderr(t, func() {
		code = Run("test", "dev", "unknown", []string{"-q", "list"})
	})
	if code != 5 {
		t.Errorf("-q list exit = %d, want 5 (exit code still reports warnings)", code)
	}
	if strings.Contains(errOut, "warning:") {
		t.Errorf("-q list stderr = %q, want no warnings", errOut)
	}
}

func TestGlobalVerbosePrintsDiagnostics(t *testing.T) {
	newGlobalsHome(t)
	errOut := captureStderr(t, func() {
		if got := Run("test", "dev", "unknown", []string{"-v", "list"}); got != 5 {
			t.Errorf("-v list exit = %d, want 5 (bogus config key)", got)
		}
	})
	if !strings.Contains(errOut, "rivu: config ") || !strings.Contains(errOut, "rivu: root ") {
		t.Errorf("-v stderr = %q, want config and root diagnostics", errOut)
	}
	// Quiet run of the same command stays silent about diagnostics too:
	// -v off means no rivu: lines.
	errOut = captureStderr(t, func() {
		Run("test", "dev", "unknown", []string{"list"})
	})
	if strings.Contains(errOut, "rivu: ") {
		t.Errorf("stderr without -v = %q, want no diagnostics", errOut)
	}
}

func TestGlobalNoColorForcesAsciiProfile(t *testing.T) {
	newGlobalsHome(t)
	orig := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(orig) })
	lipgloss.SetColorProfile(termenv.TrueColor)
	if got := Run("test", "dev", "unknown", []string{"--no-color", "list"}); got != 5 {
		t.Fatalf("--no-color list exit = %d, want 5 (bogus config key)", got)
	}
	if p := lipgloss.ColorProfile(); p != termenv.Ascii {
		t.Errorf("profile after --no-color = %v, want Ascii", p)
	}
}

func TestGlobalYesAndDryRunBeforeSubcommand(t *testing.T) {
	newGlobalsHome(t)
	// Baseline: reset without confirmation exits 4.
	if got := Run("test", "dev", "unknown", []string{"config", "reset"}); got != 4 {
		t.Errorf("reset exit = %d, want 4", got)
	}
	// Global --dry-run previews without writing.
	out := captureStdout(t, func() {
		if got := Run("test", "dev", "unknown", []string{"--dry-run", "config", "reset"}); got != 0 {
			t.Errorf("--dry-run reset exit = %d", got)
		}
	})
	if !strings.Contains(out, "would reset") {
		t.Errorf("--dry-run output = %q, want preview", out)
	}
	// Global --yes satisfies the confirm gate; the reset really runs.
	out = captureStdout(t, func() {
		if got := Run("test", "dev", "unknown", []string{"--yes", "config", "reset"}); got != 0 {
			t.Errorf("--yes reset exit = %d", got)
		}
	})
	if !strings.Contains(out, "reset to defaults") {
		t.Errorf("--yes output = %q, want reset confirmation", out)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("RIVU_HOME"), "config.toml.bak")); err != nil {
		t.Errorf("backup missing after reset: %v", err)
	}
}
