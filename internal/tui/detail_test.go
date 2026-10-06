package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

func detailModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := New([]registry.Project{{
		ID: "1", Name: "Alpha", Slug: "alpha", FlowStage: "source",
		Channel: "00_Source", Stack: []string{"Go", "Bubble Tea"},
		HealthScore: 42, HasGit: true, HasBank: true, HasMap: false,
		Path: `C:\ws\alpha`,
	}}, `C:\ws`)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return nm.(Model)
}

// TestDetailFullScreen: `d` opens the full-screen detail (spec 3.4)
// with badges and remedies; esc/q/d go back without quitting.
func TestDetailFullScreen(t *testing.T) {
	m := detailModel(t, 90, 30)                    // below the 120-col side layout
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab}) // leave the sidebar
	m = update(t, m, runeKey("d"))
	if !m.DetailFull {
		t.Fatal("d must open the full-screen detail")
	}
	view := m.View()
	for _, want := range []string{"Alpha", "[Bank:OK]", "[Map:Missing]", "[Health:42]", "rivu agent sync"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view missing %q", want)
		}
	}
	if !strings.Contains(view, "rivu doctor") {
		t.Error("low health check must offer the doctor remedy")
	}

	// esc goes back and does not quit.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.DetailFull {
		t.Fatal("esc must close the detail")
	}
}

// TestDetailKeyDoesNotQuit: q closes detail first, ctrl+c still quits.
func TestDetailKeyDoesNotQuit(t *testing.T) {
	m := detailModel(t, 90, 30)
	m = update(t, m, runeKey("d"))
	if !m.DetailFull {
		t.Fatal("d must open detail")
	}
	nm, cmd := m.Update(runeKey("q"))
	m = nm.(Model)
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("q must go back from detail, not quit")
		}
	}
	if m.DetailFull {
		t.Fatal("q must close the detail")
	}
}

// TestDetailWithoutSelection: teaches the next action instead.
func TestDetailWithoutSelection(t *testing.T) {
	m := New(nil, `C:\ws`)
	m.Width, m.Height = 90, 30
	m = update(t, m, runeKey("d"))
	if m.DetailFull {
		t.Fatal("no selection means no detail")
	}
	if !strings.Contains(m.Status, "Select a project") {
		t.Errorf("status = %q, want guidance", m.Status)
	}
}

// TestSideDetailBadges: at >=120 cols the side pane carries badges and
// remedies too.
func TestSideDetailBadges(t *testing.T) {
	m := detailModel(t, 160, 45)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab}) // focus the list
	view := m.View()
	for _, want := range []string{"PROJECT DETAILS", "[Git]", "[Bank:OK]", "[Map:Missing]", "rivu agent sync"} {
		if !strings.Contains(view, want) {
			t.Errorf("side detail missing %q", want)
		}
	}
}

// TestDetailDoctorKeyMoved: spec 3.9 — d is detail, h is doctor.
func TestDetailDoctorKeyMoved(t *testing.T) {
	if !key.Matches(runeKey("d"), keys.Detail) {
		t.Error("d must open detail")
	}
	if key.Matches(runeKey("d"), keys.Doctor) {
		t.Error("d must no longer trigger doctor")
	}
	if !key.Matches(runeKey("h"), keys.Doctor) {
		t.Error("h must trigger doctor (spec 3.9)")
	}
}
