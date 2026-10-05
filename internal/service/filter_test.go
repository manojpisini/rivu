package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
)

// seedProjects creates n projects through Source and then tweaks their
// rows directly for filter inputs the Service does not expose (language,
// health, timestamps).
func seedProjects(t *testing.T, a *App, names ...string) map[string]registry.Project {
	t.Helper()
	out := map[string]registry.Project{}
	for _, n := range names {
		if _, err := a.Source(n, "source", false, false, false); err != nil {
			t.Fatalf("Source %s: %v", n, err)
		}
		p, err := a.Registry.Find(n)
		if err != nil {
			t.Fatalf("Find %s: %v", n, err)
		}
		out[n] = p
	}
	return out
}

func tweak(t *testing.T, a *App, id string, set string, v any) {
	t.Helper()
	if _, err := a.Registry.DB.Exec("UPDATE projects SET "+set+" = ? WHERE id = ?", v, id); err != nil {
		t.Fatalf("tweak %s: %v", set, err)
	}
}

func slugs(ps []registry.Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Slug
	}
	return out
}

func TestListFiltersAndSort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	seeded := seedProjects(t, a, "zeta", "alpha", "mid")
	tweak(t, a, seeded["zeta"].ID, "flow_stage", "active")
	tweak(t, a, seeded["zeta"].ID, "language", "Go")
	tweak(t, a, seeded["zeta"].ID, "health_score", 90)
	tweak(t, a, seeded["alpha"].ID, "language", "Python")
	tweak(t, a, seeded["alpha"].ID, "health_score", 30)
	tweak(t, a, seeded["mid"].ID, "flow_stage", "delta")
	tweak(t, a, seeded["mid"].ID, "language", "go")
	tweak(t, a, seeded["mid"].ID, "health_score", 50)
	// mid opened long before the stale threshold; others just now.
	long := time.Now().AddDate(0, 0, -a.Config.Flow.StaleThresholdDays-5).Format(time.RFC3339)
	tweak(t, a, seeded["mid"].ID, "last_opened_at", long)

	for _, tc := range []struct {
		name string
		f    Filter
		want []string
	}{
		{"zero filter keeps lifecycle order", Filter{}, []string{"alpha", "zeta", "mid"}},
		{"flow", Filter{Flow: "active"}, []string{"zeta"}},
		{"lang case-insensitive", Filter{Lang: "go"}, []string{"zeta", "mid"}},
		{"unhealthy below 60", Filter{Unhealthy: true}, []string{"alpha", "mid"}},
		{"stale", Filter{Stale: true}, []string{"mid"}},
		{"combined and", Filter{Lang: "go", Unhealthy: true}, []string{"mid"}},
		{"sort name", Filter{Sort: "name"}, []string{"alpha", "mid", "zeta"}},
		{"sort health worst first", Filter{Sort: "health"}, []string{"alpha", "mid", "zeta"}},
		{"sort name within flow", Filter{Flow: "delta", Sort: "name"}, []string{"mid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps, err := a.List(tc.f)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			got := slugs(ps)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("List(%+v) = %v, want %v", tc.f, got, tc.want)
			}
		})
	}
}

func TestListConfluenceFilter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	seeded := seedProjects(t, a, "one", "two")
	if _, err := a.Registry.DB.Exec(`INSERT INTO confluences(id,name) VALUES('c1','ship')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Registry.DB.Exec(`INSERT INTO project_confluences(project_id,confluence_id) VALUES(?, 'c1')`, seeded["two"].ID); err != nil {
		t.Fatal(err)
	}

	ps, err := a.List(Filter{Confluence: "ship"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := slugs(ps); len(got) != 1 || got[0] != "two" {
		t.Errorf("confluence filter = %v, want [two]", got)
	}
	// Unknown confluence matches nothing rather than failing.
	ps, err = a.List(Filter{Confluence: "nope"})
	if err != nil || len(ps) != 0 {
		t.Errorf("unknown confluence = %v, %v; want empty", ps, err)
	}
}
