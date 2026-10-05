package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// captureStdout returns everything fn writes to the process stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// captureStderr returns everything fn writes to the process stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestPathCommand(t *testing.T) {
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
	if got := Run("test", "dev", "unknown", []string{"source", "pathable", "--git=false"}); got != 0 {
		t.Fatalf("source exit = %d", got)
	}
	want := filepath.Join(ws, "00_Source", "pathable")
	// Registry canonicalises paths (Windows 8.3, symlinks).
	if x, err := filepath.EvalSymlinks(want); err == nil {
		want = x
	}

	// Named project: path only, one line.
	var got string
	code := 0
	out := captureStdout(t, func() {
		code = Run("test", "dev", "unknown", []string{"path", "pathable"})
	})
	if code != 0 {
		t.Errorf("path exit = %d, want 0", code)
	}
	if got = out; got != want+"\n" {
		t.Errorf("path output = %q, want %q", got, want+"\n")
	}
	// No argument: Current (set by source) — same path.
	out = captureStdout(t, func() {
		code = Run("test", "dev", "unknown", []string{"path"})
	})
	if code != 0 || out != want+"\n" {
		t.Errorf("path (current) = %q exit %d, want %q", out, code, want+"\n")
	}
	// Unknown project: exit 3, nothing on stdout.
	out = captureStdout(t, func() {
		code = Run("test", "dev", "unknown", []string{"path", "ghost"})
	})
	if code != 3 {
		t.Errorf("path ghost exit = %d, want 3", code)
	}
	if out != "" {
		t.Errorf("path ghost wrote %q to stdout", out)
	}
}
