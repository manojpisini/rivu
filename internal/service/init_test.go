package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manojpisini/rivu/internal/config"
)

func TestInitCreatesConfigAndDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	ws := t.TempDir()

	res, err := Init(ws, false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if res.Root != ws {
		t.Errorf("Root = %q, want %q", res.Root, ws)
	}
	if _, err := os.Stat(res.ConfigPath); err != nil {
		t.Errorf("config not written: %v", err)
	}
	if _, err := os.Stat(res.DBPath); err != nil {
		t.Errorf("database not created: %v", err)
	}
	cfg, _, err := config.Load()
	if err != nil {
		t.Fatalf("Load after init: %v", err)
	}
	if cfg.Workspace.Root != ws {
		t.Errorf("config root = %q, want %q", cfg.Workspace.Root, ws)
	}

	// Second init without --force must refuse.
	if _, err := Init(ws, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("re-init without --force = %v, want --force hint", err)
	}
	// --force overwrites and keeps the same root working.
	if _, err := Init(ws, true); err != nil {
		t.Errorf("Init --force: %v", err)
	}
	// Missing root is validated before anything is written.
	bad := filepath.Join(home, "no-such-root")
	cfgPath, _ := config.Path()
	mtimeBefore := statTime(t, cfgPath)
	if _, err := Init(bad, true); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("missing root = %v, want does-not-exist error", err)
	}
	if statTime(t, cfgPath) != mtimeBefore {
		t.Error("failed init rewrote the config")
	}
}

func statTime(t *testing.T, p string) int64 {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime().UnixNano()
}
