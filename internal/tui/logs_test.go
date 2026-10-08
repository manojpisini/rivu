package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

const sampleLogLine = `{"time":"2026-10-08T09:14:05.000Z","level":"INFO","msg":"scan started"}`

// logsFixture opens the Logs screen from the dashboard with `L` and
// delivers one loaded snapshot so the view has content (P5.10).
func logsFixture(t *testing.T) Root {
	t.Helper()
	m, f := scanFixture(t)
	f.LogLines = []string{
		sampleLogLine,
		`{"time":"2026-10-08T09:15:06.000Z","level":"ERROR","msg":"scan failed"}`,
		"not json at all",
	}
	f.RecentAct = []registry.Activity{
		{Slug: "alpha", Name: "Alpha", Event: "opened", OccurredAt: mustLogTime(t, "2026-10-08 09:14")},
		{Slug: "beta", Name: "Beta", Event: "sourced", OccurredAt: mustLogTime(t, "2026-10-07 08:55")},
	}
	m, cmd := updateC(t, m, runeKey("L"))
	if cmd == nil {
		t.Fatal("L must open the Logs screen")
	}
	r := NewRoot(f, testSettingsConfig())
	r.dashboard = m
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	r, _ = upd(t, r, cmd())
	if r.screen != ScreenLogs {
		t.Fatalf("screen = %v, want logs", r.screen)
	}
	r, _ = upd(t, r, logsLoadedMsg{
		lines: prettyLogLines(f.LogLines),
		act:   f.RecentAct,
	})
	return r
}

// TestLogsScreenOpensAndRenders: L opens the screen, the JSON line is
// pretty-printed, a non-JSON line passes through and follow starts on.
func TestLogsScreenOpensAndRenders(t *testing.T) {
	r := logsFixture(t)
	v := r.View()
	for _, want := range []string{
		"LOGS", "rivu.log",
		"09:14:05 INFO  scan started",
		"09:15:06 ERROR scan failed",
		"not json at all",
		"f follow: on", "esc back",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("logs view missing %q in %q", want, v)
		}
	}
	if !r.logsFollow {
		t.Error("follow must start on")
	}
}

// TestLogsScrollPausesFollowAndTabSwitches: scrolling pauses follow
// (starting from the pinned newest lines), tab swaps to activity_log,
// f resumes follow.
func TestLogsScrollPausesFollowAndTabSwitches(t *testing.T) {
	r := logsFixture(t)
	// Fill past the 24-line window so scrolling has somewhere to go.
	var big []string
	for i := range 40 {
		big = append(big, prettyLogLine(sampleLogLineFor(i)))
	}
	r, _ = upd(t, r, logsLoadedMsg{lines: big, act: r.logsAct})
	if want := r.logsRowSet(len(big)); r.logsScroll != want {
		t.Fatalf("follow pin = %d, want %d", r.logsScroll, want)
	}

	r, _ = upd(t, r, runeKey("j"))
	if r.logsFollow {
		t.Error("j must pause follow")
	}
	if want := r.logsRowSet(len(big)); r.logsScroll != want {
		t.Errorf("scroll while pinned = %d, want %d (nothing newer to move to)", r.logsScroll, want)
	}
	if strings.Contains(r.View(), "f follow: on") {
		t.Error("footer must show follow off after scrolling")
	}

	r, _ = upd(t, r, runeKey("k"))
	if want := r.logsRowSet(len(big)) - 1; r.logsScroll != want {
		t.Errorf("scroll after k = %d, want %d", r.logsScroll, want)
	}
	for i := 0; i < 100; i++ {
		r, _ = upd(t, r, runeKey("k"))
	}
	if r.logsScroll != 0 {
		t.Errorf("scroll must clamp at 0, got %d", r.logsScroll)
	}

	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyTab})
	if r.logsSrc != 1 {
		t.Fatalf("src = %d, want 1 (activity_log)", r.logsSrc)
	}
	v := r.View()
	if !strings.Contains(v, "activity_log") || !strings.Contains(v, "Alpha (alpha)") {
		t.Errorf("activity source missing rows in %q", v)
	}

	r, _ = upd(t, r, runeKey("f"))
	if !r.logsFollow {
		t.Error("f must resume follow")
	}
	if !strings.Contains(r.View(), "f follow: on") {
		t.Error("footer must show follow on after f")
	}
}

// TestLogsEndHomeAndRefresh: end jumps to the newest lines, home back
// to the first, r re-tails through the service.
func TestLogsEndHomeAndRefresh(t *testing.T) {
	r := logsFixture(t)
	for i := 0; i < 50; i++ {
		r, _ = upd(t, r, runeKey("j"))
	}
	r, _ = upd(t, r, runeKey("end"))
	if want := r.logsRowSet(len(r.logsRows())); r.logsScroll != want {
		t.Errorf("end scroll = %d, want %d", r.logsScroll, want)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyHome})
	if r.logsScroll != 0 {
		t.Errorf("home scroll = %d, want 0", r.logsScroll)
	}
	r, _ = upd(t, r, runeKey("r"))
	if len(r.toasts) != 0 {
		t.Errorf("refresh must not toast on success, got %v", r.toasts)
	}
}

// TestLogsEmptyStates: both sources explain what to do next when they
// have nothing to show.
func TestLogsEmptyStates(t *testing.T) {
	m, f := scanFixture(t)
	f.LogLines, f.RecentAct = nil, nil
	m, cmd := updateC(t, m, runeKey("L"))
	r := NewRoot(f, testSettingsConfig())
	r.dashboard = m
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	r, _ = upd(t, r, cmd())
	r, _ = upd(t, r, logsLoadedMsg{})

	if v := r.View(); !strings.Contains(v, "no log entries yet") {
		t.Errorf("log empty state missing in %q", v)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyTab})
	if v := r.View(); !strings.Contains(v, "no activity yet") {
		t.Errorf("activity empty state missing in %q", v)
	}
}

// TestLogsLeavingScreenStopsFollow: a tick after esc is dropped, so
// the follow chain dies with the screen.
func TestLogsLeavingScreenStopsFollow(t *testing.T) {
	r := logsFixture(t)
	r, _ = upd(t, r, keyEsc())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard", r.screen)
	}
	n := len(r.logsLines)
	r, cmd := upd(t, r, logsTickMsg{})
	if cmd != nil {
		t.Error("tick after leaving must not reload")
	}
	if len(r.logsLines) != n {
		t.Error("tick after leaving must not touch content")
	}
}

// TestLogsKeyBinding: L is bound and stays out of the 8-item
// ShortHelp (FullHelp only, like S for Settings).
func TestLogsKeyBinding(t *testing.T) {
	if keys.Logs.Help().Key != "L" || keys.Logs.Help().Desc != "logs" {
		t.Fatalf("binding = %+v, want L/logs", keys.Logs.Help())
	}
	if n := len(keys.ShortHelp()); n != 8 {
		t.Fatalf("ShortHelp = %d bindings, want 8", n)
	}
	found := false
	for _, b := range keys.FullHelp()[1] {
		if b.Help().Key == "L" {
			found = true
		}
	}
	if !found {
		t.Error("Logs must appear in the FullHelp actions column")
	}
}

// mustLogTime pins an activity timestamp so the golden and assertions
// render the same on every OS.
func mustLogTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

// sampleLogLineFor builds a distinct-but-parsable JSON line per index.
func sampleLogLineFor(i int) string {
	return `{"time":"2026-10-08T09:14:05.000Z","level":"INFO","msg":"line ` +
		strconv.Itoa(i) + `"}`
}
