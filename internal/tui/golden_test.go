package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
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
