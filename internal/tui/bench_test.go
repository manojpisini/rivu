package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

// benchRoot builds a dashboard with n projects at the widest layout
// (three panes); no testing.T so benchmarks can reuse it.
func benchRoot(n int) Root {
	r := goldenRoot()
	ps := make([]registry.Project, n)
	stages := []string{"source", "active", "maintenance", "research", "delta"}
	for i := range n {
		ps[i] = registry.Project{
			ID:          fmt.Sprintf("%d", i),
			Name:        fmt.Sprintf("proj-%04d", i),
			Slug:        fmt.Sprintf("proj-%04d", i),
			Path:        fmt.Sprintf("/w/proj-%04d", i),
			Language:    "go",
			FlowStage:   stages[i%len(stages)],
			HealthScore: i % 100,
			OnDisk:      true,
		}
	}
	m := r.dashboard
	m.Projects = ps
	m.applyFilter()
	r.dashboard = m
	nm, _ := r.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	return nm.(Root)
}

// BenchmarkView5000 guards the 16 ms (60 fps) render budget with the
// worst-case three-pane layout at 5,000 projects (P3.36).
func BenchmarkView5000(b *testing.B) {
	r := benchRoot(5000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = r.View()
	}
}
