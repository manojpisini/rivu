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
	got, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FlowStage != "active" || got[0].Language != "Go" {
		t.Fatalf("unexpected: %#v", got)
	}
}
