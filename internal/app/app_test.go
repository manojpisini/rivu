package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanRejectsMissingWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	a.Config.Workspace.Root = filepath.Join(home, "never-created")
	if _, err := a.Scan(); err == nil {
		t.Fatal("Scan accepted a missing workspace root, want error")
	}
}

func TestScanRejectsInvalidConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	body := "[scanner]\nmax_depth = 0\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(); err == nil {
		t.Fatal("Open accepted max_depth = 0, want error")
	}
}
