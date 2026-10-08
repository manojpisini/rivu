package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

// typeIn sends each rune of s to the palette query line.
func typeIn(t *testing.T, r Root, s string) Root {
	t.Helper()
	for _, c := range s {
		r, _ = upd(t, r, keyR(c))
	}
	return r
}

func TestPaletteOpensFiltersAndRuns(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 120, Height: 40})
	r, cmd := upd(t, r, keyR(':'))
	if cmd != nil {
		t.Fatal("opening the palette should not schedule a command")
	}
	if !r.palOn {
		t.Fatal(": must open the palette")
	}
	if !strings.Contains(r.View(), "COMMAND") {
		t.Fatalf("view must show the palette title, got %q", r.View())
	}
	r = typeIn(t, r, "set")
	rows := r.palRows()
	if len(rows) == 0 || paletteLabels()[rows[0]] != "settings" {
		t.Fatalf("query set ranks %v, want settings first", rows)
	}
	r, cmd = upd(t, r, keyEnter())
	if r.palOn {
		t.Fatal("enter must close the palette")
	}
	if cmd == nil {
		t.Fatal("enter must return the action's command")
	}
	r, _ = upd(t, r, cmd())
	if r.screen != ScreenSettings {
		t.Fatalf("screen = %d, want settings", r.screen)
	}
}

func TestPaletteEscClosesWithoutRunning(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, keyR(':'))
	if !r.palOn {
		t.Fatal(": must open the palette")
	}
	r, cmd := upd(t, r, keyEsc())
	if r.palOn || cmd != nil || r.screen != ScreenDashboard {
		t.Fatalf("esc must close quietly: on=%v cmd=%v screen=%v", r.palOn, cmd, r.screen)
	}
}

func TestPaletteSwallowsTypingKeys(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, keyR(':'))
	r, _ = upd(t, r, keyR('q'))
	r, _ = upd(t, r, keyR('j'))
	if !r.palOn {
		t.Fatal("q/j must type into the query, not quit or move the list")
	}
	if r.palTI.Value() != "qj" {
		t.Fatalf("query = %q, want qj", r.palTI.Value())
	}
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard", r.screen)
	}
}

func TestPaletteDoctorAllRunsHealthChecks(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, keyR(':'))
	r = typeIn(t, r, "doctor all")
	rows := r.palRows()
	if len(rows) == 0 || paletteLabels()[rows[0]] != "doctor all" {
		t.Fatalf("doctor all query ranks %v", rows)
	}
	r, cmd := upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("doctor all must return the doctor command")
	}
	if !strings.Contains(r.dashboard.Status, "Running health checks") {
		t.Fatalf("status = %q, want the health run announced", r.dashboard.Status)
	}
	if r.palOn {
		t.Fatal("palette must close before the command runs")
	}
}

func TestPaletteFlowToActivePlansMove(t *testing.T) {
	r, svc := rootOf(t)
	svc.Projects = []registry.Project{
		{ID: "1", Name: "alpha", Slug: "alpha", Path: "/ws/alpha", FlowStage: "source", Channel: "00_Source", OnDisk: true},
	}
	r, _ = upd(t, r, r.loadProjects()())
	r, _ = upd(t, r, keyR(':'))
	r = typeIn(t, r, "flow to active")
	r, cmd := upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("flow to active must return the plan command")
	}
	if !strings.Contains(r.dashboard.Status, "Preparing move to active") {
		t.Fatalf("status = %q, want the move announced", r.dashboard.Status)
	}
}

func TestPaletteGuardWhileSearchFocused(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, keyR('/')) // focus the search input
	if !r.dashboard.search.Focused() {
		t.Fatal("search must be focused")
	}
	r, _ = upd(t, r, keyR(':'))
	if r.palOn {
		t.Fatal(": must type into a focused search, not open the palette")
	}
}

func TestPaletteOverlaysDismissedOnRun(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, keyR('?')) // help overlay open on the dashboard
	if !r.dashboard.helpOpen {
		t.Fatal("? must open help")
	}
	r, _ = upd(t, r, keyR(':'))
	if !r.palOn {
		t.Fatal(": must open over the help overlay")
	}
	r = typeIn(t, r, "settings")
	r, cmd := upd(t, r, keyEnter())
	if r.dashboard.helpOpen {
		t.Fatal("running a command must dismiss the help overlay")
	}
	if cmd == nil {
		t.Fatal("settings action must return its command")
	}
}

func TestPaletteNavigationAndQuit(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, keyR(':'))
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyDown})
	if r.palCursor != 1 {
		t.Fatalf("cursor = %d after down, want 1", r.palCursor)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyUp})
	if r.palCursor != 0 {
		t.Fatalf("cursor = %d after up, want 0", r.palCursor)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyUp})
	if r.palCursor != 0 {
		t.Fatalf("cursor = %d at ceiling, want 0", r.palCursor)
	}
	_, cmd := upd(t, r, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c must quit while the palette is open")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl+c must produce a QuitMsg")
	}
}

func TestFuzzyRanks(t *testing.T) {
	labels := paletteLabels()
	tests := []struct {
		query string
		first string // "" means expect no matches
	}{
		{"", labels[0]},
		{"sta", "stats"},
		{"doctor all", "doctor all"},
		{"mstr", "master dashboard"},
		{"logs", "logs"},
		{"zzz", ""},
	}
	for _, tc := range tests {
		rows := fuzzyRanks(tc.query, labels)
		if tc.first == "" {
			if len(rows) != 0 {
				t.Errorf("fuzzyRanks(%q) = %v, want no matches", tc.query, rows)
			}
			continue
		}
		if len(rows) == 0 || labels[rows[0]] != tc.first {
			got := []string{}
			for _, i := range rows {
				got = append(got, labels[i])
			}
			t.Errorf("fuzzyRanks(%q) first = %v, want %q first", tc.query, got, tc.first)
		}
	}
}

func TestPaletteKeyBinding(t *testing.T) {
	if keys.Palette.Help().Key != ":" || keys.Palette.Help().Desc != "command palette" {
		t.Fatalf("binding = %+v, want :/command palette", keys.Palette.Help())
	}
	if !paletteBinds(keyR(':')) {
		t.Error("paletteBinds must match :")
	}
	if paletteBinds(keyR('x')) {
		t.Error("paletteBinds must not match x")
	}
	found := false
	for _, b := range keys.FullHelp()[2] {
		if b.Help().Key == ":" {
			found = true
		}
	}
	if !found {
		t.Error("Palette must appear in the FullHelp global column")
	}
	if n := len(keys.ShortHelp()); n != 8 {
		t.Errorf("ShortHelp = %d entries, want 8", n)
	}
}
