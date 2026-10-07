package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

func motionFixture(t *testing.T) Model {
	t.Helper()
	ps := make([]registry.Project, 60)
	for i := range ps {
		ps[i] = registry.Project{ID: fmt.Sprintf("id-%02d", i), Slug: fmt.Sprintf("p%02d", i), Name: fmt.Sprintf("P%02d", i), FlowStage: "source"}
	}
	m := New(ps, `C:\ws`)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return nm.(Model)
}

// TestPageAndHalfMotions (P3.30, revised by P4.16): pgdn/pgup and
// ctrl+d/ctrl+u move by the same row count the table actually renders;
// home/end jump to the edges; G stays bottom while g opens the Master
// Dashboard (spec 3.9), so it no longer scrolls.
func TestPageAndHalfMotions(t *testing.T) {
	m := motionFixture(t)
	rows := m.listRows()
	if rows < 2 {
		t.Fatalf("listRows = %d, want a real page", rows)
	}

	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.Cursor != rows {
		t.Errorf("pgdn cursor = %d, want %d", m.Cursor, rows)
	}
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.Cursor != 0 {
		t.Errorf("pgup cursor = %d, want 0", m.Cursor)
	}

	m.Cursor = 0
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.Cursor != rows/2 {
		t.Errorf("ctrl+d cursor = %d, want %d", m.Cursor, rows/2)
	}
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.Cursor != 0 {
		t.Errorf("ctrl+u cursor = %d, want 0", m.Cursor)
	}

	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if m.Cursor != len(m.Visible)-1 {
		t.Errorf("end cursor = %d, want last (%d)", m.Cursor, len(m.Visible)-1)
	}
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyHome})
	if m.Cursor != 0 {
		t.Errorf("home cursor = %d, want 0", m.Cursor)
	}
	m, _ = updateC(t, m, runeKey("G"))
	if m.Cursor != len(m.Visible)-1 {
		t.Errorf("G cursor = %d, want last", m.Cursor)
	}
	// g opens the Master Dashboard (spec 3.9), so it no longer scrolls;
	// top stays reachable on Home above
	m, _ = updateC(t, m, runeKey("g"))
	if m.Cursor != len(m.Visible)-1 {
		t.Errorf("g cursor = %d, want unchanged (master, not top)", m.Cursor)
	}
}

// TestPageMotionsClampAndSurviveEmptyList: overshoot lands on the last
// row; an empty list never goes negative.
func TestPageMotionsClampAndSurviveEmptyList(t *testing.T) {
	m := motionFixture(t)
	m.Cursor = len(m.Visible) - 1
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.Cursor != len(m.Visible)-1 {
		t.Errorf("pgdn past the end = %d, want clamped to %d", m.Cursor, len(m.Visible)-1)
	}

	m.Projects = nil
	m.applyFilter()
	m, cmd := updateC(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if cmd != nil || m.Cursor != 0 {
		t.Errorf("empty list: cursor %d cmd %v, want 0/nil", m.Cursor, cmd)
	}
}

// TestPageMotionsIgnoreSidebarFocus: while the sidebar has focus,
// pgdn still drives the project list (six stages are no page).
func TestPageMotionsIgnoreSidebarFocus(t *testing.T) {
	m := motionFixture(t)
	m.FocusSidebar = true
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.Cursor != m.listRows() {
		t.Errorf("cursor = %d with sidebar focused, want %d", m.Cursor, m.listRows())
	}
}
