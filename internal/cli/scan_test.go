package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	ws := filepath.Join(home, "ws")
	alt := filepath.Join(home, "alt")
	for _, d := range []string{
		filepath.Join(ws, "00_Source", "at-ws"),
		filepath.Join(alt, "00_Source", "at-alt"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		// A strong marker makes the directory a project (scanner rule).
		if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module example.com/x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := fmt.Sprintf("[workspace]\nroot = %q\n", ws)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) int {
		t.Helper()
		return Run("test", "dev", "unknown", args)
	}

	// Text mode reports count and root.
	out := captureStdout(t, func() {
		if code := run("scan"); code != 0 {
			t.Errorf("scan exit = %d", code)
		}
	})
	if !strings.Contains(out, "Mapped 1 project(s) from "+ws) {
		t.Errorf("scan text = %q, want mapped count and root", out)
	}

	// --root scans only the given folder; config root stays put.
	out = captureStdout(t, func() {
		if code := run("scan", "--root", alt); code != 0 {
			t.Errorf("scan --root exit = %d", code)
		}
	})
	if !strings.Contains(out, "Mapped 1 project(s) from "+alt) {
		t.Errorf("scan --root = %q, want alt root", out)
	}

	// --json: schema 1, projects and mismatch lists.
	out = captureStdout(t, func() {
		if code := run("scan", "--json", "--root", ws); code != 0 {
			t.Errorf("scan --json exit = %d", code)
		}
	})
	var payload struct {
		Schema   int    `json:"schema"`
		Root     string `json:"root"`
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
		Warnings      []string `json:"warnings"`
		Missing       []string `json:"missing"`
		Unregistered  []string `json:"unregistered"`
		StageMismatch []string `json:"stage_mismatch"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("scan --json %q: %v", out, err)
	}
	if payload.Schema != 1 || payload.Root != ws || len(payload.Projects) != 1 || payload.Projects[0].Slug != "at-ws" {
		t.Errorf("scan --json = %+v, want schema 1 root ws one project", payload)
	}
	if payload.Warnings == nil || payload.Missing == nil || payload.Unregistered == nil || payload.StageMismatch == nil {
		t.Errorf("scan --json lists = %+v, want non-nil arrays", payload)
	}

	// Extra args are usage errors.
	if code := run("scan", "extra"); code != 2 {
		t.Errorf("scan with args exit = %d, want 2", code)
	}
}
