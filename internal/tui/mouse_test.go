package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manojpisini/rivu/internal/registry"
)

// mouseRoot builds a sized dashboard with two visible projects.
func mouseRoot(t *testing.T) Root {
	t.Helper()
	r, svc := rootOf(t)
	svc.Projects = []registry.Project{{ID: "1", Name: "alpha"}, {ID: "2", Name: "beta"}}
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	r, _ = upd(t, r, r.loadProjects()())
	r.dashboard.Cursor = 0
	return r
}

// clickAt builds a left-press on row i of the dashboard at 100x30
// (two panes: sidebar ends at 24, panel starts at 25).
func clickAt(t *testing.T, r Root, i int) tea.MouseMsg {
	t.Helper()
	y := lipgloss.Height(bgStyle.Width(r.width).Render(r.dashboard.header())) + 4 + i
	return tea.MouseMsg{X: 30, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func TestMouseWheelScrollsListWithSidebarFocused(t *testing.T) {
	r := mouseRoot(t)
	if !r.dashboard.FocusSidebar {
		t.Fatal("fixture must start with the sidebar focused (default)")
	}
	r, _ = upd(t, r, tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if r.dashboard.Cursor != 1 {
		t.Fatalf("wheel down cursor = %d, want 1", r.dashboard.Cursor)
	}
	r, _ = upd(t, r, tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if r.dashboard.Cursor != 0 {
		t.Fatalf("wheel up cursor = %d, want 0", r.dashboard.Cursor)
	}
}

func TestMouseClickSelectsRow(t *testing.T) {
	r := mouseRoot(t)
	r, cmd := upd(t, r, clickAt(t, r, 1))
	if r.dashboard.Cursor != 1 {
		t.Fatalf("cursor = %d, want row 1 selected", r.dashboard.Cursor)
	}
	if cmd != nil {
		t.Fatalf("single click must not open, got cmd %v", cmd)
	}
	// a cell above the list selects nothing
	above := clickAt(t, r, 1)
	above.Y = 0
	r.dashboard.Cursor = 0
	r, _ = upd(t, r, above)
	if r.dashboard.Cursor != 0 {
		t.Fatalf("header click moved cursor to %d", r.dashboard.Cursor)
	}
}

func TestMouseDoubleClickOpensRow(t *testing.T) {
	r := mouseRoot(t)
	c := clickAt(t, r, 1)
	r, cmd := upd(t, r, c)
	if cmd != nil {
		t.Fatalf("first click must not open, got %v", cmd)
	}
	r, cmd = upd(t, r, c)
	if cmd == nil {
		t.Fatal("second click on the same row must open (enter)")
	}
	// a stale first click is just another select
	r.lastClickRow, r.lastClickAt = 1, time.Now().Add(-time.Second)
	r, cmd = upd(t, r, c)
	if cmd != nil {
		t.Fatalf("stale click must not open, got %v", cmd)
	}
}

func TestMouseClickIgnoredUnderOverlay(t *testing.T) {
	r := mouseRoot(t)
	r.dashboard.helpOpen = true
	r, cmd := upd(t, r, clickAt(t, r, 1))
	if r.dashboard.Cursor != 0 || cmd != nil {
		t.Fatalf("overlay must swallow clicks: cursor=%d cmd=%v", r.dashboard.Cursor, cmd)
	}
}
