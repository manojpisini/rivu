package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
)

// TestMasterDashboardOpensAndRenders: g and m both load the snapshot;
// the screen shows portfolio, language bars and recent activity, and
// esc returns to the list (P4.16).
func TestMasterDashboardOpensAndRenders(t *testing.T) {
	m, f := scanFixture(t)
	f.DashRes = service.Dashboard{
		Root:     "C:/ws",
		Roots:    2,
		LastScan: time.Now().Add(-4 * time.Minute),
		Stats: service.Stats{
			Total:     6,
			AvgHealth: 74,
			ByFlow:    []service.Count{{Name: "source", Count: 2}, {Name: "active", Count: 4}},
			ByLanguage: []service.Count{
				{Name: "Go", Count: 4},
				{Name: "TS", Count: 2},
			},
		},
		Recent: []registry.Activity{
			{Slug: "alpha", Name: "Alpha", Event: "opened", OccurredAt: time.Now()},
		},
		// service.Dashboard already ranks these (spec 3.3); the screen
		// must preserve that order (P4.17)
		Attention: []service.Attention{
			{Key: "missing_map", Count: 2},
			{Key: "missing_git", Count: 1},
			{Key: "stale", Count: 7},
		},
	}
	m, cmd := updateC(t, m, runeKey("g"))
	if cmd == nil {
		t.Fatal("g must load the master dashboard")
	}
	if !strings.Contains(m.Status, "Loading master dashboard") {
		t.Fatalf("Status = %q, want progress notice", m.Status)
	}
	dm, ok := cmd().(dashMsg)
	if !ok || dm.err != nil {
		t.Fatalf("cmd = %#v, want dashMsg", cmd())
	}

	r, _ := rootOf(t)
	r.dashboard = m
	r, _ = upd(t, r, dm)
	if r.screen != ScreenMasterDashboard {
		t.Fatalf("screen = %v, want master dashboard", r.screen)
	}
	v := r.View()
	for _, want := range []string{
		"MASTER DASHBOARD", "Root: C:/ws", "Roots: 2", "Health: 74/100",
		"Portfolio", "Total projects", "source (untriaged)", "active",
		"Needs attention", "! 2 projects missing Map", "! 1 project missing git",
		"! 7 stale projects (45d+)",
		"By language", "Recent activity", "opened", "Alpha", "█",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("master view missing %q in %q", want, v)
		}
	}
	// the queue keeps the service's rank order (P4.17)
	if i, j := strings.Index(v, "missing Map"), strings.Index(v, "stale projects"); i > j {
		t.Errorf("attention order lost: missing Map at %d after stale at %d", i, j)
	}

	// esc returns; m then opens it too (the alias from the task)
	r, _ = upd(t, r, keyEsc())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after esc", r.screen)
	}
	m2, cmd2 := updateC(t, r.dashboard, runeKey("m"))
	if cmd2 == nil || !strings.Contains(m2.Status, "Loading master dashboard") {
		t.Fatalf("m = cmd %v status %q, want the master load", cmd2, m2.Status)
	}
}

// TestMasterDashboardErrorAndEmpty: a failed snapshot lands in the
// sticky error banner; an empty one explains itself instead of showing
// blank sections (P4.16).
func TestMasterDashboardErrorAndEmpty(t *testing.T) {
	m, f := scanFixture(t)
	f.StatsErr = errors.New("db locked")
	m, cmd := updateC(t, m, runeKey("m"))
	r, _ := rootOf(t)
	r.dashboard = m
	r, cmd = upd(t, r, cmd())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after a failed load", r.screen)
	}
	if cmd != nil {
		t.Errorf("failed load must not queue work, got %v", cmd)
	}
	if len(r.errs) != 1 || !strings.Contains(r.errs[0], "could not load the master dashboard: db locked") {
		t.Fatalf("errs = %v, want the sticky failure", r.errs)
	}

	// empty snapshot: muted placeholders, no crash
	f.StatsErr = nil
	f.DashRes = service.Dashboard{Root: "C:/ws"}
	_, cmd = updateC(t, m, runeKey("g"))
	r, _ = upd(t, r, cmd())
	v := r.View()
	for _, want := range []string{"no projects yet", "nothing needs attention", "no activity recorded yet", "Health: 0/100"} {
		if !strings.Contains(v, want) {
			t.Errorf("empty master view missing %q in %q", want, v)
		}
	}
}

// TestAttentionEnterJumpsToFix: enter on a bucket opens that project's
// Detail with the fix line lit, clearing any active filter on the way,
// and esc leaves no stale highlight behind (P4.18).
func TestAttentionEnterJumpsToFix(t *testing.T) {
	m, f := scanFixture(t)
	// Beta is hidden by an active search; the jump must clear it
	m.Query = "alpha"
	m.applyFilter()
	if len(m.Visible) != 1 {
		t.Fatalf("setup: visible = %d, want only Alpha", len(m.Visible))
	}
	f.DashRes = service.Dashboard{
		Attention: []service.Attention{{Key: "missing_map", Count: 1, Sample: "b"}},
	}
	m, cmd := updateC(t, m, runeKey("g"))
	r, _ := rootOf(t)
	r.dashboard = m
	r, _ = upd(t, r, cmd())
	if r.screen != ScreenMasterDashboard {
		t.Fatalf("screen = %v, want master dashboard", r.screen)
	}

	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyEnter})
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after enter", r.screen)
	}
	if !r.dashboard.DetailFull {
		t.Fatal("enter must open the full-screen detail")
	}
	if r.dashboard.FixKey != "missing_map" {
		t.Fatalf("FixKey = %q, want missing_map", r.dashboard.FixKey)
	}
	if r.dashboard.Query != "" {
		t.Fatalf("Query = %q, want cleared so the sample is visible", r.dashboard.Query)
	}
	p, ok := r.dashboard.selectedProject()
	if !ok || p.Slug != "b" {
		t.Fatalf("selected = %+v, want the b sample", p)
	}
	r.dashboard.Width, r.dashboard.Height = 90, 30
	view := r.dashboard.View()
	if !strings.Contains(view, "FIX") || !strings.Contains(view, "then a again to rebuild Bank + Map") {
		t.Errorf("detail view missing the pre-highlighted fix in %q", view)
	}

	// esc leaves no stale highlight for the next manual d
	nm, _ := r.dashboard.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if nm.(Model).FixKey != "" {
		t.Error("closing detail must clear the fix highlight")
	}
}

// TestAttentionCursorClamps: the queue cursor never runs off the ends
// and an empty queue ignores enter (P4.18).
func TestAttentionCursorClamps(t *testing.T) {
	m, f := scanFixture(t)
	f.DashRes = service.Dashboard{
		Attention: []service.Attention{
			{Key: "missing_git", Count: 1, Sample: "a"},
			{Key: "stale", Count: 2, Sample: "b"},
		},
	}
	m, cmd := updateC(t, m, runeKey("g"))
	r, _ := rootOf(t)
	r.dashboard = m
	r, _ = upd(t, r, cmd())

	r, _ = upd(t, r, keyR('k'))
	if r.masterCursor != 0 {
		t.Errorf("cursor = %d, want clamped at 0", r.masterCursor)
	}
	r, _ = upd(t, r, keyR('j'))
	r, _ = upd(t, r, keyR('j'))
	if r.masterCursor != 1 {
		t.Errorf("cursor = %d, want clamped at 1", r.masterCursor)
	}
	// enter on the stale bucket jumps to beta with the stale fix
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyEnter})
	if r.dashboard.FixKey != "stale" {
		t.Fatalf("FixKey = %q, want stale", r.dashboard.FixKey)
	}

	// empty queue on the master screen: enter does nothing
	r.masterRes = &service.Dashboard{}
	r.masterCursor = 0
	r.screen = ScreenMasterDashboard
	r.dashboard.DetailFull = false
	r.dashboard.FixKey = ""
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyEnter})
	if r.screen != ScreenMasterDashboard || r.dashboard.DetailFull {
		t.Errorf("empty queue enter: screen=%v detail=%v, want no jump",
			r.screen, r.dashboard.DetailFull)
	}
}
