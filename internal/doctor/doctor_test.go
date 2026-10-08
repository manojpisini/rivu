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

// TestDependencyCheck (D-01): every known manifest must ship a
// lockfile; projects with no tracked manifest pass vacuously.
func TestDependencyCheck(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  bool
	}{
		{"no manifest", nil, true},
		{"go locked", []string{"go.mod", "go.sum"}, true},
		{"go unlocked", []string{"go.mod"}, false},
		{"npm via yarn", []string{"package.json", "yarn.lock"}, true},
		{"npm unlocked", []string{"package.json"}, false},
		{"cargo locked", []string{"Cargo.toml", "Cargo.lock"}, true},
		{"cargo unlocked", []string{"Cargo.toml"}, false},
		{"python unlocked", []string{"pyproject.toml"}, false},
		{"python via uv", []string{"pyproject.toml", "uv.lock"}, true},
		{"maven has no lock convention", []string{"pom.xml"}, true},
		{"one locked one not", []string{"go.mod", "go.sum", "package.json"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(d, f), []byte("x"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			c := depCheck(d)
			if c.OK != tc.want {
				t.Errorf("OK=%v (%s), want %v", c.OK, c.Detail, tc.want)
			}
			if c.Name != "Dependencies" || c.Weight != 5 {
				t.Errorf("check = %+v, want Dependencies weight 5", c)
			}
		})
	}

	// The score consequence: an unlocked manifest costs the +5.
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	base := Run(registry.Project{Path: d}, "").Score
	if err := os.WriteFile(filepath.Join(d, "go.sum"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if got := Run(registry.Project{Path: d}, "").Score; got != base+5 {
		t.Errorf("lockfile adds %d to score, want +5 (base %d, got %d)", got-base, base, got)
	}
}
