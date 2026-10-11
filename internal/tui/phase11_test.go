package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service/fake"
)

func phase11Model(t *testing.T) Model {
	t.Helper()
	m := New([]registry.Project{
		{ID: "1", Name: "alpha", Slug: "alpha", Path: "w/alpha", FlowStage: "active", Language: "Go", HealthScore: 80, OnDisk: true},
		{ID: "2", Name: "beta", Slug: "beta", Path: "w/beta", FlowStage: "source", Language: "Go", HealthScore: 40, OnDisk: true},
		{ID: "3", Name: "gamma", Slug: "gamma", Path: "w/15_Archive/Old/gamma", FlowStage: "source", Language: "Rust", HealthScore: 10, OnDisk: true},
		{ID: "4", Name: "delta", Slug: "delta", Path: "w/delta", FlowStage: "source", Language: "Python", HealthScore: 60, OnDisk: false},
	}, "w")
	m.Width, m.Height = 140, 40
	return m
}

// TestGroupLangSortsAndFilters: b toggles grouping; rows sort by
// language when on and revert to stage order when off.
func TestGroupLangSortsAndFilters(t *testing.T) {
	m := phase11Model(t)
	applyKey(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if !m.GroupLang {
		t.Fatal("b must toggle GroupLang")
	}
	if m.Visible[0].Language != "Go" || m.Visible[1].Language != "Go" {
		t.Fatalf("grouped order = %+v, want Go first", names(m.Visible))
	}
	if !strings.Contains(m.View(), "── Go (2) ──") {
		t.Fatal("grouped view must render a language section header")
	}
	applyKey(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if m.GroupLang {
		t.Fatal("second b must toggle grouping off")
	}
}

// TestHideStaleFiltersOldAndMissing: U hides missing folders and
// archive-ish paths; toggling back restores them.
func TestHideStaleFiltersOldAndMissing(t *testing.T) {
	m := phase11Model(t)
	applyKey(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	if !m.HideStale {
		t.Fatal("U must toggle HideStale")
	}
	for _, p := range m.Visible {
		if p.ID == "3" || p.ID == "4" {
			t.Fatalf("stale project %s must be hidden, visible=%+v", p.Name, names(m.Visible))
		}
	}
	if len(m.Visible) != 2 {
		t.Fatalf("visible = %d, want 2", len(m.Visible))
	}
	applyKey(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	if len(m.Visible) != 4 {
		t.Fatalf("visible after untoggle = %d, want 4", len(m.Visible))
	}
}

// TestTreeViewOpensAndCloses: w opens the overlay, esc closes it.
func TestTreeViewOpensAndCloses(t *testing.T) {
	m := phase11Model(t)
	applyKey(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if !m.treeView {
		t.Fatal("w must open the tree view")
	}
	view := m.View()
	if !strings.Contains(view, "WORKSPACE TREE") || !strings.Contains(view, "alpha") {
		t.Fatalf("tree view missing content: %q", view)
	}
	applyKey(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.treeView {
		t.Fatal("esc must close the tree view")
	}
}

// TestHeatmapRenders: the side pane shows the ACTIVITY block when
// enough daily counts exist.
func TestHeatmapRenders(t *testing.T) {
	svc := &fake.Service{}
	heat := make([]int, 90)
	heat[89] = 5 // today
	m := phase11Model(t)
	m.svc = svc
	m.heat = heat
	view := m.View()
	if !strings.Contains(view, "ACTIVITY") {
		t.Fatal("side pane must render the activity heatmap")
	}
	if got := heatmapView(nil, 60); got != "" {
		t.Fatalf("heatmapView(nil) = %q, want empty", got)
	}
}

// TestLooksOldPath: archive-ish segments flag, normal paths do not.
func TestLooksOldPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/w/15_Archive/Old/x", true},
		{"/w/backups/x", true},
		{"/w/Active/x", false},
		{"/w/playground", false},
	} {
		if got := looksOldPath(tc.path); got != tc.want {
			t.Errorf("looksOldPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// applyKey applies a key message to the model in place.
func applyKey(t *testing.T, m *Model, msg tea.KeyMsg) {
	t.Helper()
	nm, _ := m.Update(msg)
	*m = nm.(Model)
}

// names is a small test helper.
func names(ps []registry.Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}
