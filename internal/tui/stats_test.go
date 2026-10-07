package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/manojpisini/rivu/internal/service"
)

// TestStatsScreenOpensAndRenders: s loads totals, the breakdowns and
// the 30-day activity sparkline, and esc returns (P4.19).
func TestStatsScreenOpensAndRenders(t *testing.T) {
	m, f := scanFixture(t)
	f.StatsRes = service.Stats{
		Range:        "all",
		Total:        6,
		AvgHealth:    74,
		MedianHealth: 70,
		Stale:        3,
		ByFlow:       []service.Count{{Name: "source", Count: 2}, {Name: "active", Count: 4}},
		ByLanguage:   []service.Count{{Name: "Go", Count: 4}, {Name: "TS", Count: 2}},
		ByHealth:     []service.Count{{Name: "90-100", Count: 1}, {Name: "70-89", Count: 5}},
	}
	f.ActivityRes = []int{0, 1, 0, 3}
	m, cmd := updateC(t, m, runeKey("s"))
	if cmd == nil {
		t.Fatal("s must load the stats screen")
	}
	if !strings.Contains(m.Status, "Loading stats") {
		t.Fatalf("Status = %q, want progress notice", m.Status)
	}
	sm, ok := cmd().(statsMsg)
	if !ok || sm.err != nil {
		t.Fatalf("cmd = %#v, want statsMsg", cmd())
	}

	r, _ := rootOf(t)
	r.dashboard = m
	r, _ = upd(t, r, sm)
	if r.screen != ScreenStats {
		t.Fatalf("screen = %v, want stats", r.screen)
	}
	v := r.View()
	for _, want := range []string{
		"STATS", "Range: All time",
		"Projects total", "6", "Avg health", "74/100", "Median health", "70/100",
		"Stale (45d+)", "3", "Missing map",
		"By language", "By flow", "By health", "90-100",
		"Activity (last 30 days)", "esc back",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("stats view missing %q in %q", want, v)
		}
	}
	// the sparkline scales to the counts: max 3 renders a full block
	if !strings.Contains(v, "█") {
		t.Errorf("stats view missing the activity sparkline in %q", v)
	}

	r, _ = upd(t, r, keyEsc())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after esc", r.screen)
	}
}

// TestStatsScreenErrorsAndEmpty: a failed load lands in the sticky
// banner; an empty snapshot explains itself instead of drawing an
// all-zero sparkline (P4.19).
func TestStatsScreenErrorsAndEmpty(t *testing.T) {
	m, f := scanFixture(t)
	f.StatsErr = errors.New("db locked")
	m, cmd := updateC(t, m, runeKey("s"))
	r, _ := rootOf(t)
	r.dashboard = m
	r, cmd = upd(t, r, cmd())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after a failed load", r.screen)
	}
	if cmd != nil {
		t.Errorf("failed load must not queue work, got %v", cmd)
	}
	if len(r.errs) != 1 || !strings.Contains(r.errs[0], "could not load stats: db locked") {
		t.Fatalf("errs = %v, want the sticky failure", r.errs)
	}

	// no events: the activity row says so instead of a flat line
	f.StatsErr = nil
	f.ActivityRes = nil
	m, cmd = updateC(t, m, runeKey("s"))
	r, _ = upd(t, r, cmd())
	v := r.View()
	if !strings.Contains(v, "no events yet") {
		t.Errorf("empty stats = %q, want the no-events hint", v)
	}
}
