package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/manojpisini/rivu/internal/registry"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		ran  bool
		want int
	}{
		{"ok", nil, true, 0},
		{"error", errors.New("boom"), true, 1},
		{"usage before run", errors.New("unknown flag"), false, 2},
		{"not found wrapped", fmt.Errorf("resolve: %w", registry.ErrNotFound), true, 3},
		{"ambiguous wrapped", fmt.Errorf("%w: 2 matches", registry.ErrAmbiguous), true, 3},
		{"needs confirm", fmt.Errorf("flow changes: %w", ErrNeedsConfirm), true, 4},
		{"warnings", fmt.Errorf("%w: 2 scan warning(s)", ErrWarnings), true, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err, tt.ran); got != tt.want {
				t.Errorf("ExitCode(%v, ran=%v) = %d, want %d", tt.err, tt.ran, got, tt.want)
			}
		})
	}
}

// runCLI executes the real tree with args in an isolated RIVU_HOME and
// returns the exit code.
func runCLI(t *testing.T, cfgBody string, args ...string) int {
	t.Helper()
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}
	body := cfgBody
	if body == "" {
		body = fmt.Sprintf("[workspace]\nroot = %q\n", ws)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return Run("test", "dev", "unknown", args)
}

func TestRunExitCodes(t *testing.T) {
	if got := runCLI(t, "", "list"); got != 0 {
		t.Errorf("list exit = %d, want 0", got)
	}
	if got := runCLI(t, "", "--definitely-not-a-flag"); got != 2 {
		t.Errorf("bad flag exit = %d, want 2 (usage)", got)
	}
	if got := runCLI(t, "", "nosuchcommand"); got != 2 {
		t.Errorf("unknown command exit = %d, want 2 (usage)", got)
	}
	if got := runCLI(t, "", "flow", "ghost", "--to", "active", "--yes"); got != 3 {
		t.Errorf("missing project exit = %d, want 3 (not found)", got)
	}
	if got := runCLI(t, "", "flow", "ghost", "--to", "active"); got != 4 {
		t.Errorf("flow without --yes exit = %d, want 4 (needs confirm)", got)
	}
}

func TestRunExitCode5OnConfigWarnings(t *testing.T) {
	ws := t.TempDir()
	body := fmt.Sprintf("[workspace]\nroot = %q\nnot_a_real_key = true\n", ws)
	if got := runCLI(t, body, "scan"); got != 5 {
		t.Errorf("scan with config warning exit = %d, want 5 (warnings)", got)
	}
}
