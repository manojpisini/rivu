package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDetectsGoProject(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "01_Active", "demo")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module demo"), 0644); err != nil {
		t.Fatal(err)
	}
	got, _, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FlowStage != "active" || got[0].Language != "Go" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestScanMissingRootErrors(t *testing.T) {
	if _, _, err := New(nil, 6).Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing root must be an error, not zero projects")
	}
}

func TestScanFileRootErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New(nil, 6).Scan(p); err == nil {
		t.Error("file root must be an error")
	}
}

func TestScanCollectsWalkWarnings(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("directory permissions are not enforced on Windows")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0755) })
	good := filepath.Join(root, "demo")
	if err := os.Mkdir(good, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "go.mod"), []byte("module demo"), 0644); err != nil {
		t.Fatal(err)
	}
	got, warns, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatalf("permission warning must not abort the scan: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("good project still discovered, got %d", len(got))
	}
	if len(warns) == 0 {
		t.Error("expected a warning for the unreadable directory")
	}
}
