package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service/fake"
)

// TestColourlessViewCarriesStatusText is the accessibility guarantee
// (spec 3.10): badges are colour AND text, so under NO_COLOR every
// status the theme would colour is still readable in the stripped
// frame. Runs for each theme because a theme must not be the thing
// that carries meaning.
func TestColourlessViewCarriesStatusText(t *testing.T) {
	for _, name := range []string{"graphite-violet", "mono", "light"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "1")
			svc := &fake.Service{}
			svc.Projects = []registry.Project{
				{ID: "1", Name: "alpha", FlowStage: "active", HealthScore: 100},
				{ID: "2", Name: "beta", FlowStage: "source", HealthScore: 42},
			}
			cfg := config.Default()
			cfg.Appearance.Theme = name
			r := NewRoot(svc, cfg)
			r, _ = upd(t, r, tea.WindowSizeMsg{Width: 120, Height: 40})
			r, _ = upd(t, r, r.loadProjects()())

			v := ansi.Strip(r.View())
			for _, want := range []string{"alpha", "beta", "HEALTH", "100", "42", "ACTIVE", "SOURCE", "ALL"} {
				if !strings.Contains(v, want) {
					t.Errorf("stripped view missing %q", want)
				}
			}
			if strings.Contains(v, "\x1b[") {
				t.Error("escape sequences leaked into the frame under NO_COLOR")
			}
		})
	}
}
