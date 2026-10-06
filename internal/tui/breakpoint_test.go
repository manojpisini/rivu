package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

// TestBreakpoints checks the layout rules from AGENTS 8: <60x15 too
// small, 60-99 single pane, 100-139 two panes, >=140 three panes.
func TestBreakpoints(t *testing.T) {
	seeds := []registry.Project{
		{ID: "1", Name: "alpha", Language: "Go", HealthScore: 90},
		{ID: "2", Name: "beta", Language: "TS", HealthScore: 80},
	}
	cases := []struct {
		w, h     int
		tooSmall bool
		sidebar  bool
		list     bool
		detail   bool
	}{
		{59, 30, true, false, false, false}, // narrow
		{80, 14, true, false, false, false}, // short
		{60, 15, false, false, true, false}, // single, boundary
		{99, 30, false, false, true, false}, // single, top
		{100, 30, false, false, true, true}, // two panes, boundary
		{139, 40, false, false, true, true}, // two panes, top
		{140, 45, false, true, true, true},  // three panes, boundary
		{200, 50, false, true, true, true},  // three panes
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%dx%d", c.w, c.h), func(t *testing.T) {
			r, _ := rootOf(t)
			r, _ = upd(t, r, tea.WindowSizeMsg{Width: c.w, Height: c.h})
			r, _ = upd(t, r, projectsMsg{ps: seeds})
			view := r.View()
			if c.tooSmall {
				if !strings.Contains(view, "Terminal too small") {
					t.Fatalf("want too-small screen, got %q", view)
				}
				return
			}
			if strings.Contains(view, "Terminal too small") {
				t.Fatalf("unexpected too-small screen at %dx%d", c.w, c.h)
			}
			if got := strings.Contains(view, "QUICK ACTIONS"); got != c.sidebar {
				t.Errorf("sidebar present = %v, want %v", got, c.sidebar)
			}
			if got := strings.Contains(view, "PROJECT DETAILS"); got != c.detail {
				t.Errorf("detail panel present = %v, want %v", got, c.detail)
			}
			if !strings.Contains(view, "alpha") {
				t.Errorf("project list must show projects, got %q", view)
			}
		})
	}
}

// TestNarrowWidthUnfocusesSidebar: the sidebar pane is hidden below
// 100 columns, so focus must not be stuck there.
func TestNarrowWidthUnfocusesSidebar(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 140, Height: 40})
	r.dashboard.FocusSidebar = true
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 80, Height: 40})
	if r.dashboard.FocusSidebar {
		t.Fatal("focus must leave the hidden sidebar below 100 columns")
	}
}
