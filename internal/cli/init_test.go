package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCommandExitCodes(t *testing.T) {
	ws := t.TempDir()
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	if got := Run("test", "dev", "unknown", []string{"init", "--root", ws}); got != 0 {
		t.Errorf("init exit = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err != nil {
		t.Errorf("config not created: %v", err)
	}
	// Re-init without --force: exit 1 with a --force hint on stderr.
	if got := Run("test", "dev", "unknown", []string{"init", "--root", ws}); got != 1 {
		t.Errorf("re-init exit = %d, want 1", got)
	}
	if got := Run("test", "dev", "unknown", []string{"init", "--root", ws, "--force"}); got != 0 {
		t.Errorf("init --force exit = %d, want 0", got)
	}
	// Missing --root is a usage error.
	if got := Run("test", "dev", "unknown", []string{"init"}); got != 2 {
		t.Errorf("init without --root exit = %d, want 2 (usage)", got)
	}
	// Nonexistent workspace root: exit 1.
	if got := Run("test", "dev", "unknown", []string{"init", "--root", filepath.Join(home, "missing")}); got != 1 {
		t.Errorf("init bad root exit = %d, want 1", got)
	}
}
