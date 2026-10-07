package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

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
		"By language", "Recent activity", "opened", "Alpha", "█",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("master view missing %q in %q", want, v)
		}
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
	m, cmd = updateC(t, m, runeKey("g"))
	r, _ = upd(t, r, cmd())
	v := r.View()
	for _, want := range []string{"no projects yet", "no activity recorded yet", "Health: 0/100"} {
		if !strings.Contains(v, want) {
			t.Errorf("empty master view missing %q in %q", want, v)
		}
	}
}
