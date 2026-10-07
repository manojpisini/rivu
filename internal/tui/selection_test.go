package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

func selectionModel(t *testing.T) Model {
	t.Helper()
	m := New([]registry.Project{
		{ID: "id-a", Slug: "a", Name: "Alpha", FlowStage: "source"},
		{ID: "id-b", Slug: "b", Name: "Beta", FlowStage: "source"},
	}, `C:\ws`)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return nm.(Model)
}

// TestRefreshKeepsSelectionByID (P3.27): a refresh that resorts the
// rows must keep the same project selected by ID, not by row index.
func TestRefreshKeepsSelectionByID(t *testing.T) {
	m := selectionModel(t)
	m.Cursor = 1 // Beta
	// Beta is renamed so it now sorts first — index 1 would be Alpha.
	m.Projects = []registry.Project{
		{ID: "id-b", Slug: "b", Name: "Aaa", FlowStage: "source"},
		{ID: "id-a", Slug: "a", Name: "Alpha", FlowStage: "source"},
	}
	m.applyFilter()
	if got := m.Visible[m.Cursor].ID; got != "id-b" {
		t.Fatalf("selected %q after refresh, want id-b (cursor %d, visible %v)", got, m.Cursor, m.Visible)
	}
}

// TestFilterKeepsSelectedProject: search dropping the other row leaves
// the survivor selected; clearing the search brings the old partner
// back without jumping to it.
func TestFilterKeepsSelectedProject(t *testing.T) {
	m := selectionModel(t)
	m.Cursor = 1 // Beta
	m.Query = "beta"
	m.applyFilter()
	if len(m.Visible) != 1 || m.Visible[m.Cursor].ID != "id-b" {
		t.Fatalf("filtered = %v cursor %d, want Beta selected", m.Visible, m.Cursor)
	}
	m.Query = ""
	m.applyFilter()
	if got := m.Visible[m.Cursor].ID; got != "id-b" {
		t.Fatalf("selected %q after clearing search, want id-b (cursor %d)", got, m.Cursor)
	}
}

// TestStageSwitchStartsAtTop: browsing a stage is a new list — the old
// selection must not be dragged along.
func TestStageSwitchStartsAtTop(t *testing.T) {
	m := selectionModel(t)
	m.Cursor = 1
	m.setStage(0) // ALL — same rows, fresh cursor
	if m.Cursor != 0 {
		t.Fatalf("cursor = %d after stage switch, want 0", m.Cursor)
	}
	// empty stage still lands at 0 without a stray -1
	m.setStage(2) // active — nothing there
	if m.Cursor != 0 || len(m.Visible) != 0 {
		t.Fatalf("cursor %d visible %d, want 0/0", m.Cursor, len(m.Visible))
	}
}

// TestRootRefreshRoutesThroughPreserve: the async projectsMsg path
// applies the same ID preservation.
func TestRootRefreshRoutesThroughPreserve(t *testing.T) {
	r, _ := rootOf(t)
	m := selectionModel(t)
	m.Cursor = 1 // Beta
	r.dashboard = m
	r, _ = upd(t, r, projectsMsg{ps: []registry.Project{
		{ID: "id-b", Slug: "b", Name: "Aaa", FlowStage: "source"},
		{ID: "id-a", Slug: "a", Name: "Alpha", FlowStage: "source"},
	}})
	if got := r.dashboard.Visible[r.dashboard.Cursor].ID; got != "id-b" {
		t.Fatalf("selected %q after projectsMsg, want id-b", got)
	}
}
