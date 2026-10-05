package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
)

func TestStatsMath(t *testing.T) {
	a := openTestApp(t)
	ws := a.Config.Workspace.Root
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// p1: healthy, fresh, complete (README + map) — active, Go.
	p1 := filepath.Join(ws, "p1")
	mkdir(t, p1)
	mkdir(t, filepath.Join(p1, ".metadata"))
	writeFile(t, filepath.Join(p1, "README.md"), "# p1")
	// p2: stale (60d) and its folder is gone — source, Go.
	p2 := filepath.Join(ws, "p2")
	// p3: fresh but no README and no map — active, C++.
	p3 := filepath.Join(ws, "p3")
	mkdir(t, p3)

	projects := []registry.Project{
		{ID: "1", Name: "One", Slug: "one", Path: p1, FlowStage: "active", Language: "go", HasMap: true, HealthScore: 80, CreatedAt: now, LastOpenedAt: now},
		{ID: "2", Name: "Two", Slug: "two", Path: p2, FlowStage: "source", Language: "go", HealthScore: 40, CreatedAt: now.AddDate(0, 0, -60)},
		{ID: "3", Name: "Three", Slug: "three", Path: p3, FlowStage: "active", Language: "C++", HealthScore: 70, CreatedAt: now},
	}
	for _, p := range projects {
		if err := a.Registry.Upsert(p); err != nil {
			t.Fatalf("Upsert %s: %v", p.Slug, err)
		}
	}

	st, err := a.Stats(0)
	if err != nil {
		t.Fatal(err)
	}
	if st.Range != "all" || st.Total != 3 || st.AvgHealth != 63 || st.MedianHealth != 70 {
		t.Errorf("totals = %+v, want all/3/63/70", st)
	}
	if st.Stale != 1 || st.MissingFolder != 1 || st.MissingMap != 1 || st.MissingReadme != 1 {
		t.Errorf("counts = stale %d folder %d map %d readme %d, want 1/1/1/1",
			st.Stale, st.MissingFolder, st.MissingMap, st.MissingReadme)
	}
	assertCount(t, st.ByFlow, map[string]int{"active": 2, "source": 1, "maintenance": 0, "research": 0, "delta": 0})
	assertCount(t, st.ByLanguage, map[string]int{"go": 2, "C++": 1})
	assertCount(t, st.ByHealth, map[string]int{"70-89": 2, "0-49": 1, "90-100": 0, "50-69": 0})
	// Language order: descending count, then name.
	if st.ByLanguage[0].Name != "go" {
		t.Errorf("ByLanguage order = %+v, want go first", st.ByLanguage)
	}

	// Range window: only the fresh projects (p1, p3).
	st, err = a.Stats(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.Range != "1d" || st.Total != 2 || st.Stale != 0 {
		t.Errorf("range 1d = %s/%d/%d, want 1d/2/0", st.Range, st.Total, st.Stale)
	}
}

func TestHealthBandBoundaries(t *testing.T) {
	for h, want := range map[int]string{
		100: "90-100", 90: "90-100", 89: "70-89", 70: "70-89",
		69: "50-69", 50: "50-69", 49: "0-49", 0: "0-49", -3: "0-49",
	} {
		if got := healthBand(h); got != want {
			t.Errorf("healthBand(%d) = %q, want %q", h, got, want)
		}
	}
}

func TestMedian(t *testing.T) {
	for _, tc := range []struct {
		in   []int
		want int
	}{{nil, 0}, {[]int{50}, 50}, {[]int{40, 80}, 60}, {[]int{40, 70, 80}, 70}, {[]int{90, 10, 50, 70}, 60}} {
		if got := median(tc.in); got != tc.want {
			t.Errorf("median(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func assertCount(t *testing.T, got []Count, want map[string]int) {
	t.Helper()
	seen := map[string]int{}
	for _, c := range got {
		seen[c.Name] = c.Count
		if want[c.Name] != c.Count {
			t.Errorf("%s = %d, want %d", c.Name, c.Count, want[c.Name])
		}
	}
	if len(seen) != len(want) {
		t.Errorf("got %d buckets %v, want %d", len(seen), seen, len(want))
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
