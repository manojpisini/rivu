package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// flat collapses panel line wraps so wrapped teaching text still matches.
func flat(v string) string {
	return strings.Join(strings.Fields(v), " ")
}

// TestEmptyStatesTeachNextAction (P3.23): every empty render says what
// the user should do next, and only mentions keys that are bound now.
func TestEmptyStatesTeachNextAction(t *testing.T) {
	m, _ := scanFixture(t)

	// workspace with no projects at all
	m.Projects = nil
	m.applyFilter()
	v := flat(m.View())
	for _, want := range []string{
		"No projects yet — run `rivu source <path>` or press r to rescan.",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("workspace empty state must teach source/rescan: %q", v)
		}
	}

	// search that matches nothing
	m, _ = scanFixture(t)
	m.Query = "zzzz"
	m.applyFilter()
	v = flat(m.View())
	if !strings.Contains(v, "No projects match your search — press esc to clear it.") {
		t.Errorf("search empty state must teach esc: %q", v)
	}

	// stage with no projects (fixture projects live in source)
	m, _ = scanFixture(t)
	m.setStage(2) // active
	v = flat(m.View())
	if !strings.Contains(v, "Nothing in this stage — press 1-5 to switch stage, or r to rescan.") {
		t.Errorf("stage empty state must teach stage keys: %q", v)
	}
}

func TestDetailsPanelEmptyTeaches(t *testing.T) {
	m := testModel(t)
	m.Projects = nil
	m.applyFilter()
	if got := m.detailsPanel(60, 20); !strings.Contains(got, "Select a project (up/down to move, enter to open).") {
		t.Errorf("details empty state = %q", got)
	}
}

func TestDoctorEmptyStateTeaches(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	r.screen = ScreenHealth
	r.doctorRes = nil
	v := flat(r.View())
	if !strings.Contains(v, "No projects to check yet — run `rivu source <path>` or press r to rescan.") {
		t.Errorf("doctor empty state = %q", v)
	}
}
