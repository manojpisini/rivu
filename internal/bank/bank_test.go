package bank

import (
	"github.com/manojpisini/rivu/internal/registry"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildProtectsExistingFiles(t *testing.T) {
	p := registry.Project{ID: "1", Name: "Demo", Slug: "demo", Path: t.TempDir(), FlowStage: "source", Channel: "00_Source"}
	if err := Build(p, "rivu"); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(p.Path, ".metadata", "overview.md")
	if err := os.WriteFile(f, []byte("custom"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Build(p, "rivu"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	if string(b) != "custom" {
		t.Fatal("protected file overwritten")
	}
}
