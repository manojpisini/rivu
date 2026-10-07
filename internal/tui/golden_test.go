package tui

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service/fake"
)

// updateGolden rewrites the golden files; run:
//
//	go test ./internal/tui -run TestGoldenViews -update
var updateGolden = flag.Bool("update", false, "rewrite golden files")

// goldenRoot pins every variable the render could otherwise pick up
// from the environment: fixed paths, zero scan times ("never"), no
// toasts, and the UTF-8 glyph set.
func goldenRoot() Root {
	cfg := config.Default()
	cfg.Workspace.Root = "/w"
	r := NewRoot(&fake.Service{}, cfg)
	m := r.dashboard
	m.Projects = []registry.Project{
		{ID: "1", Name: "Alpha", Slug: "alpha", Path: "/w/alpha", Language: "go", FlowStage: "source", Stack: []string{"go", "sqlite"}, HealthScore: 88, OnDisk: true},
		{ID: "2", Name: "Beta", Slug: "beta", Path: "/w/beta", Language: "rust", FlowStage: "active", Stack: []string{"tokio"}, HealthScore: 61},
		{ID: "3", Name: "Gamma", Slug: "gamma", Path: "/w/gamma", Language: "go", FlowStage: "maintenance", HealthScore: 44},
	}
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
			got := r.View()

			golden := filepath.Join("testdata", "view_"+tc.name+".golden")
			if *updateGolden {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Errorf("view at %s diverges from golden; re-run with -update if the change is intentional\n--- got ---\n%s", tc.name, got)
			}
		})
	}
}
