package tui

import (
	"errors"
	"os"
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
	_, cmd = updateC(t, m, runeKey("s"))
	r, _ = upd(t, r, cmd())
	v := r.View()
	if !strings.Contains(v, "no events yet") {
		t.Errorf("empty stats = %q, want the no-events hint", v)
	}
}

// statsFixture opens the Stats screen with a populated snapshot so the
// P4.20 tests start from one known state.
func statsFixture(t *testing.T) Root {
	t.Helper()
	m, f := scanFixture(t)
	f.StatsRes = service.Stats{
		Range:      "all",
		Total:      6,
		AvgHealth:  74,
		ByFlow:     []service.Count{{Name: "active", Count: 6}},
		ByLanguage: []service.Count{{Name: "Go", Count: 6}},
		ByHealth:   []service.Count{{Name: "70-89", Count: 6}},
	}
	f.ActivityRes = []int{1, 2}
	m, cmd := updateC(t, m, runeKey("s"))
	r, _ := rootOf(t)
	r.dashboard = m
	r, _ = upd(t, r, cmd())
	if r.screen != ScreenStats {
		t.Fatalf("screen = %v, want stats", r.screen)
	}
	return r
}

// TestStatsToggleCompact: t hides the language and flow breakdowns
// while health and the totals stay (P4.20).
func TestStatsToggleCompact(t *testing.T) {
	r := statsFixture(t)
	r, _ = upd(t, r, runeKey("t"))
	v := r.View()
	if !r.statsCompact {
		t.Fatal("t must set the compact view")
	}
	if strings.Contains(v, "By language") || strings.Contains(v, "By flow") {
		t.Errorf("compact stats must hide language/flow, got %q", v)
	}
	if !strings.Contains(v, "By health") || !strings.Contains(v, "Projects total") {
		t.Errorf("compact stats must keep health and totals, got %q", v)
	}
	r, _ = upd(t, r, runeKey("t"))
	if r.statsCompact {
		t.Error("t must toggle back")
	}
	if v = r.View(); !strings.Contains(v, "By language") || !strings.Contains(v, "By flow") {
		t.Errorf("full stats must show language/flow, got %q", v)
	}
}

// TestStatsExportPickerWritesFile: e opens the format picker, enter
// writes ./stats.csv or ./stats.json with the shared service encoders,
// esc cancels without a command (P4.20).
func TestStatsExportPickerWritesFile(t *testing.T) {
	r := statsFixture(t)
	t.Chdir(t.TempDir())

	// esc closes the picker and queues nothing
	r, _ = upd(t, r, runeKey("e"))
	if !r.statsExport {
		t.Fatal("e must open the export picker")
	}
	if v := r.View(); !strings.Contains(v, "Export:") || !strings.Contains(v, "[csv]") {
		t.Fatalf("picker = %q, want the csv/json chooser", v)
	}
	// while the picker owns keys, t must not toggle the view
	r, _ = upd(t, r, runeKey("t"))
	if r.statsCompact {
		t.Error("the picker must swallow t")
	}
	r, _ = upd(t, r, keyEsc())
	if r.statsExport {
		t.Fatal("esc must close the picker")
	}

	// csv: enter with the picker closed is inert, then open and export
	r, cmd := upd(t, r, keyEnter())
	if cmd != nil {
		t.Fatalf("enter with no picker must do nothing, got %v", cmd)
	}
	r, _ = upd(t, r, runeKey("e"))
	r, cmd = upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("enter in the picker must run the export")
	}
	msg := cmd()
	done, ok := msg.(exportDoneMsg)
	if !ok || done.err != nil {
		t.Fatalf("cmd = %#v, want a clean exportDoneMsg", msg)
	}
	if done.name != "stats.csv" {
		t.Errorf("name = %q, want stats.csv", done.name)
	}
	data, err := os.ReadFile(done.name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "metric,value") {
		t.Errorf("stats.csv = %q, want the CLI's csv table", data)
	}
	r, _ = upd(t, r, done)
	if len(r.toasts) != 1 || !strings.Contains(r.toasts[0].Text, "exported stats.csv") {
		t.Errorf("toasts = %#v, want the export confirmation", r.toasts)
	}

	// json: right moves to the json slot
	r, _ = upd(t, r, runeKey("e"))
	r, _ = upd(t, r, keyRight())
	if r.statsExportC != 1 {
		t.Fatal("right must select json")
	}
	r, cmd = upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("enter must run the json export")
	}
	done, ok = cmd().(exportDoneMsg)
	if !ok || done.err != nil {
		t.Fatalf("cmd = %#v, want a clean exportDoneMsg", cmd())
	}
	if done.name != "stats.json" {
		t.Fatalf("name = %q, want stats.json", done.name)
	}
	data, err = os.ReadFile(done.name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schema":1`) {
		t.Errorf("stats.json = %q, want the schema-1 document", data)
	}
}

// TestStatsExportFailureIsSticky: a failed write lands in the sticky
// banner, not a toast that expires (P4.20).
func TestStatsExportFailureIsSticky(t *testing.T) {
	r := statsFixture(t)
	dir := t.TempDir()
	t.Chdir(dir)
	// a directory in the way makes os.Create fail
	if err := os.Mkdir("stats.csv", 0o755); err != nil {
		t.Fatal(err)
	}
	r, _ = upd(t, r, runeKey("e"))
	_, cmd := upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("enter must attempt the export")
	}
	r, _ = upd(t, r, cmd())
	if len(r.errs) != 1 || !strings.Contains(r.errs[0], "export failed") {
		t.Fatalf("errs = %v, want the sticky export failure", r.errs)
	}
}
