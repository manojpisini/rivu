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
	r := Run(registry.Project{Path: d}, "")
	if r.Score < 20 {
		t.Fatalf("score=%d", r.Score)
	}
}

func TestCheckWeightsSumTo100(t *testing.T) {
	r := Run(registry.Project{Path: t.TempDir()}, "")
	sum := 0
	for _, c := range r.Checks {
		sum += c.Weight
	}
	if sum != 100 {
		t.Errorf("check weights sum to %d, want 100", sum)
	}
}

func TestDoubleInitCheck(t *testing.T) {
	d := t.TempDir()
	for _, p := range []string{".git", ".lode"} {
		if err := os.MkdirAll(filepath.Join(d, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeOwner := func(owner string) {
		body := "[rivu]\ngit_init_owner = \"" + owner + "\"\n"
		if err := os.MkdirAll(filepath.Join(d, ".metadata"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, ".metadata", "project.toml"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	find := func(r Report, name string) (Check, bool) {
		for _, c := range r.Checks {
			if c.Name == name {
				return c, true
			}
		}
		return Check{}, false
	}

	writeOwner("rivu")
	c, ok := find(Run(registry.Project{Path: d}, ".lode"), "Git exclusivity")
	if !ok {
		t.Fatal("exclusivity check missing")
	}
	if c.OK {
		t.Error("double-init not flagged with owner=rivu + marker + .git")
	}

	writeOwner("bridge")
	if c, _ = find(Run(registry.Project{Path: d}, ".lode"), "Git exclusivity"); !c.OK {
		t.Error("owner=bridge must not flag double-init")
	}

	if _, ok = find(Run(registry.Project{Path: d}, ""), "Git exclusivity"); ok {
		t.Error("check must be absent when no scaffold marker configured")
	}
}
