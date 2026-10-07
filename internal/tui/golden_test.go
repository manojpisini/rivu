package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/service/fake"
)

// goldenProjects pins every variable the render could otherwise pick
// up from the environment: fixed paths, zero scan times ("never").
func goldenProjects() []registry.Project {
	return []registry.Project{
		{ID: "1", Name: "Alpha", Slug: "alpha", Path: "/w/alpha", Language: "go", FlowStage: "source", Stack: []string{"go", "sqlite"}, HealthScore: 88, OnDisk: true},
		{ID: "2", Name: "Beta", Slug: "beta", Path: "/w/beta", Language: "rust", FlowStage: "active", Stack: []string{"tokio"}, HealthScore: 61},
		{ID: "3", Name: "Gamma", Slug: "gamma", Path: "/w/gamma", Language: "go", FlowStage: "maintenance", HealthScore: 44},
	}
}

// goldenRoot gives the model fixed paths, no toasts, the UTF-8 glyph
// set and a root that exists; the service carries goldenProjects so a
// live program's Init loads the same list.
func goldenRoot() Root {
	cfg := config.Default()
	cfg.Workspace.Root = "/w"
	r := NewRoot(&fake.Service{Projects: goldenProjects()}, cfg)
	m := r.dashboard
	m.Projects = goldenProjects()
	m.RootExists = true
	m.Status = ""
	m.applyFilter()
	r.dashboard = m
	return r
}

func TestGoldenViews(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	prevGlyphs := asciiGlyphs
	asciiGlyphs = false
	t.Cleanup(func() { asciiGlyphs = prevGlyphs })

	for _, tc := range []struct {
		name string
		w, h int
	}{
		{"60x20", 60, 20},
		{"100x30", 100, 30},
		{"160x45", 160, 45},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := goldenRoot()
			r, _ = upd(t, r, tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
			v := r.View()
			if n := lipgloss.Height(v); n > tc.h {
				t.Errorf("view is %d lines at height %d — bubbletea would cut the top lines", n, tc.h)
			}
			golden.RequireEqual(t, []byte(v))
		})
	}
}

// TestGoldenPhase4Screens pins the Phase 4 screens — source wizard,
// master dashboard, stats, map report, the x actions menu and the
// delta confirm modal — at one canonical size (P4.21). Payloads are
// fixed structs with zero times so ago() renders "never" on every OS.
func TestGoldenPhase4Screens(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	prevGlyphs := asciiGlyphs
	asciiGlyphs = false
	t.Cleanup(func() { asciiGlyphs = prevGlyphs })

	const w, h = 100, 30
	sized := func(t *testing.T) Root {
		t.Helper()
		r := goldenRoot()
		r, _ = upd(t, r, tea.WindowSizeMsg{Width: w, Height: h})
		return r
	}

	for _, tc := range []struct {
		name  string
		build func(t *testing.T) Root
	}{
		{"source-wizard", func(t *testing.T) Root {
			r := sized(t)
			r, _ = upd(t, r, SelectScreenMsg{Screen: ScreenSource})
			return r
		}},
		{"master-dashboard", func(t *testing.T) Root {
			r := sized(t)
			r, _ = upd(t, r, dashMsg{d: service.Dashboard{
				Root: "/w", Roots: 1,
				Stats: service.Stats{
					Range: "all", Total: 3, AvgHealth: 64, MedianHealth: 61,
					ByFlow: []service.Count{
						{Name: "source", Count: 1}, {Name: "active", Count: 1}, {Name: "maintenance", Count: 1},
					},
					ByLanguage: []service.Count{{Name: "go", Count: 2}, {Name: "rust", Count: 1}},
				},
				Attention: []service.Attention{
					{Key: "missing_map", Count: 1, Sample: "beta"},
					{Key: "stale", Count: 1, Sample: "gamma"},
				},
				Recent: []registry.Activity{
					{Slug: "alpha", Name: "Alpha", Event: "flowed source -> active"},
				},
			}})
			return r
		}},
		{"stats", func(t *testing.T) Root {
			r := sized(t)
			r, _ = upd(t, r, statsMsg{
				st: service.Stats{
					Range: "all", Total: 3, AvgHealth: 64, MedianHealth: 61, Stale: 1,
					ByFlow:        []service.Count{{Name: "source", Count: 1}, {Name: "active", Count: 2}},
					ByLanguage:    []service.Count{{Name: "go", Count: 2}, {Name: "rust", Count: 1}},
					ByHealth:      []service.Count{{Name: "90-100", Count: 1}, {Name: "50-69", Count: 1}, {Name: "0-49", Count: 1}},
					MissingReadme: 1,
				},
				daily: []int{0, 1, 0, 3, 2},
			})
			return r
		}},
		{"map-report", func(t *testing.T) Root {
			r := sized(t)
			r, _ = upd(t, r, mapPreviewMsg{q: "alpha", prev: service.MapPreview{
				Project:    goldenProjects()[0],
				MapBody:    "# Alpha\nagent map line one\nagent map line two\n",
				MapStale:   true,
				Diff:       []string{"- stale header", "+ fresh header"},
				AgentsBody: "agents stay create-if-missing\n",
			}})
			return r
		}},
		{"actions-menu", func(t *testing.T) Root {
			r := sized(t)
			r, _ = upd(t, r, runeKey("x"))
			return r
		}},
		{"delta-modal", func(t *testing.T) Root {
			r := sized(t)
			r, _ = upd(t, r, ConfirmPlan("Delta alpha?", []string{
				"Move to the Delta stage — project files stay untouched",
				"alpha -> 90_Delta/alpha",
			}, nil)())
			return r
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.build(t)
			v := r.View()
			if n := lipgloss.Height(v); n > h {
				t.Errorf("view is %d lines at height %d — bubbletea would cut the top lines", n, h)
			}
			golden.RequireEqual(t, []byte(v))
		})
	}
}
