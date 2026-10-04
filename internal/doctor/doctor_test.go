package doctor

import (
	"github.com/manojpisini/rivu/internal/registry"
	"os"
	"path/filepath"
	"testing"
)

func TestScore(t *testing.T) {
	d := t.TempDir()
	_ = os.WriteFile(filepath.Join(d, "README.md"), []byte("x"), 0644)
	r := Run(registry.Project{Path: d})
	if r.Score < 20 {
		t.Fatalf("score=%d", r.Score)
	}
}

func TestCheckWeightsSumTo100(t *testing.T) {
	r := Run(registry.Project{Path: t.TempDir()})
	sum := 0
	for _, c := range r.Checks {
		sum += c.Weight
	}
	if sum != 100 {
		t.Errorf("check weights sum to %d, want 100", sum)
	}
}
