package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

// TestHeaderShowsRootCountCurrentAndScanAge covers P3.06: the header
// must show workspace root, project count, Current and last scan age.
func TestHeaderShowsRootCountCurrentAndScanAge(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 160, Height: 45})
	r, _ = upd(t, r, projectsMsg{ps: []registry.Project{
		{ID: "1", Name: "alpha", LastScannedAt: time.Now().Add(-4 * time.Minute)},
		{ID: "2", Name: "beta"},
	}})
	r, _ = upd(t, r, currentMsg{p: registry.Project{ID: "1", Name: "alpha"}, ok: true})
	view := r.View()
	for _, want := range []string{"PROJECT EXPLORER", "ALL 2", "CURRENT alpha", "LAST SCAN 4m ago"} {
		if !strings.Contains(view, want) {
			t.Errorf("header missing %q", want)
		}
	}
	if !strings.Contains(view, r.dashboard.WorkspaceRoot) {
		t.Error("header must show the workspace root")
	}
}

// TestHeaderWithoutCurrentOrScan: placeholders stay honest.
func TestHeaderWithoutCurrentOrScan(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 160, Height: 45})
	r, _ = upd(t, r, projectsMsg{ps: []registry.Project{{ID: "1", Name: "alpha"}}})
	view := r.View()
	if !strings.Contains(view, "CURRENT —") {
		t.Errorf("want em-dash Current placeholder, got header context")
	}
	if !strings.Contains(view, "LAST SCAN never") {
		t.Errorf("want \"LAST SCAN never\" when nothing was scanned")
	}
}

// TestAgoBuckets pins the relative-time wording.
func TestAgoBuckets(t *testing.T) {
	now := time.Now()
	cases := map[time.Time]string{
		{}:                         "never",
		now.Add(-30 * time.Second): "just now",
		now.Add(-4 * time.Minute):  "4m ago",
		now.Add(-3 * time.Hour):    "3h ago",
		now.Add(-48 * time.Hour):   "2d ago",
	}
	for ts, want := range cases {
		if got := ago(ts); got != want {
			t.Errorf("ago(%v) = %q, want %q", ts, got, want)
		}
	}
}
