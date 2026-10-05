package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceFlagHandling(t *testing.T) {
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

	// Unknown template is a usage error.
	if got := run("source", "tmpl", "--template", "bogus"); got != 2 {
		t.Errorf("--template bogus exit = %d, want 2", got)
	}
	// --no-git skips init.
	if got := run("source", "nogit", "--no-git"); got != 0 {
		t.Fatalf("--no-git exit = %d", got)
	}
	if _, err := os.Stat(filepath.Join(ws, "00_Source", "nogit", ".git")); err == nil {
		t.Error("--no-git still created .git")
	}
	// Domain folder + confluence membership, visible via list --confluence.
	if got := run("source", "tool", "--domain", "tools", "--confluence", "ship", "--no-git"); got != 0 {
		t.Fatalf("source --domain exit = %d", got)
	}
	if _, err := os.Stat(filepath.Join(ws, "00_Source", "tools", "tool")); err != nil {
		t.Errorf("domain folder missing: %v", err)
	}
	out := captureStdout(t, func() {
		if code := run("list", "--confluence", "ship", "--json"); code != 0 {
			t.Errorf("list --confluence exit = %d", code)
		}
	})
	var payload struct {
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("list --json %q: %v", out, err)
	}
	if len(payload.Projects) != 1 || payload.Projects[0].Slug != "tool" {
		t.Errorf("confluence members = %+v, want [tool]", payload.Projects)
	}
	// --bridge records ownership without running git.
	if !strings.Contains(captureStdout(t, func() {
		_ = run("source", "brdg", "--bridge", "--git")
	}), "Sourced brdg") {
		t.Error("source --bridge did not succeed")
	}
	if _, err := os.Stat(filepath.Join(ws, "00_Source", "brdg", ".git")); err == nil {
		t.Error("--bridge ran git init")
	}
}
