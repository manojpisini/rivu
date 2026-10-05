package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIJourney drives the whole command surface in-process with
// captured stdout/stderr (P2.29), asserting the scripting contract:
// data on stdout, messages on stderr, honest exit codes along the way.
func TestCLIJourney(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) (code int, out, errOut string) {
		t.Helper()
		out = captureStdout(t, func() {
			errOut = captureStderr(t, func() {
				code = Run("test", "dev", "unknown", args)
			})
		})
		return
	}
	check := func(name string, code, wantCode int, out, want string) {
		t.Helper()
		if code != wantCode {
			t.Fatalf("%s: exit = %d, want %d (stdout %q)", name, code, wantCode, out)
		}
		if want != "" && !strings.Contains(out, want) {
			t.Fatalf("%s: stdout %q, want contains %q", name, out, want)
		}
	}

	// Fresh home: init creates config + database.
	code, out, errOut := run("init", "--root", ws)
	check("init", code, 0, out, "Initialized Rivu")
	if errOut != "" {
		t.Errorf("init stderr = %q, want empty", errOut)
	}

	code, out, _ = run("scan")
	check("scan", code, 0, out, "Mapped 0 project(s)")

	code, out, _ = run("source", "journey", "--git=false")
	check("source", code, 0, out, "Sourced journey")

	code, out, _ = run("list")
	check("list", code, 0, out, "journey")

	code, out, _ = run("list", "--json")
	check("list --json", code, 0, out, "")
	var listOut struct {
		Schema   int `json:"schema"`
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(out), &listOut); err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	if listOut.Schema != 1 || len(listOut.Projects) != 1 || listOut.Projects[0].Slug != "journey" {
		t.Errorf("list --json = %+v, want schema 1 with journey", listOut)
	}

	code, out, _ = run("path", "journey")
	check("path", code, 0, out, "")
	want := filepath.Join(ws, "00_Source", "journey")
	if x, err := filepath.EvalSymlinks(want); err == nil {
		want = x
	}
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}

	// Plan → confirm → Apply: gate, preview, then the real move.
	code, out, errOut = run("flow", "journey", "--to", "active")
	if code != 4 || out != "" || !strings.Contains(errOut, "--yes") {
		t.Errorf("flow without --yes: exit %d (want 4), stdout %q, stderr %q", code, out, errOut)
	}
	code, out, _ = run("flow", "journey", "--to", "active", "--dry-run")
	check("flow --dry-run", code, 0, out, "DRY RUN")
	code, out, _ = run("flow", "journey", "--to", "active", "--yes")
	check("flow --yes", code, 0, out, "Flowed journey")

	code, out, _ = run("doctor", "journey")
	check("doctor", code, 0, out, "journey")
	code, out, _ = run("doctor", "--json")
	check("doctor --json", code, 0, out, "")
	var docOut struct {
		Schema  int `json:"schema"`
		Reports []struct {
			Slug string `json:"slug"`
		} `json:"reports"`
	}
	if err := json.Unmarshal([]byte(out), &docOut); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	if docOut.Schema != 1 || len(docOut.Reports) != 1 {
		t.Errorf("doctor --json = %+v, want schema 1 with one report", docOut)
	}

	code, out, _ = run("agent", "sync", "journey")
	check("agent sync", code, 0, out, "")
	code, out, _ = run("agent", "sync", "--check")
	check("agent sync --check", code, 0, out, "")

	code, out, _ = run("index")
	check("index", code, 0, out, "Indexed 1 project(s)")
	code, out, _ = run("index", "--json")
	check("index --json", code, 0, out, `"reconciled":[]`)

	code, out, _ = run("stats")
	check("stats", code, 0, out, "Projects: 1")
	code, out, _ = run("stats", "--json")
	check("stats --json", code, 0, out, `"total":1`)
	code, out, _ = run("dashboard")
	check("dashboard", code, 0, out, "")
	if out == "" {
		t.Error("dashboard printed nothing")
	}

	code, out, _ = run("config", "get", "workspace.root")
	check("config get", code, 0, out, ws)
	code, out, _ = run("config", "validate")
	check("config validate", code, 0, out, "config OK")
	code, out, _ = run("config", "show")
	check("config show", code, 0, out, "[workspace]")

	code, out, _ = run("delta", "journey", "--yes")
	check("delta --yes", code, 0, out, "Deltaed journey")

	code, out, _ = run("version", "--json")
	check("version --json", code, 0, out, `"schema":1`)
	code, out, _ = run("completion", "bash")
	check("completion bash", code, 0, out, "rivu")

	// Error paths: message on stderr, nothing on stdout.
	code, out, errOut = run("nosuchcommand")
	if code != 2 || out != "" || !strings.Contains(errOut, "unknown command") {
		t.Errorf("unknown command: exit %d (want 2), stdout %q, stderr %q", code, out, errOut)
	}
	code, out, errOut = run("path", "ghost")
	if code != 3 || out != "" || !strings.Contains(errOut, "not found") {
		t.Errorf("path ghost: exit %d (want 3), stdout %q, stderr %q", code, out, errOut)
	}
}
