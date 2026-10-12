package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/service"
)

// TestMapReportOpensPreviewsAndScrolls: a from the list opens the report
// with the on-disk bodies and diff, tab focuses the content panel,
// j/k scroll it, i inside improves and reloads, esc returns to the
// dashboard (P4.15, P10.06, P12.01).
func TestMapReportOpensPreviewsAndScrolls(t *testing.T) {
	m, f := scanFixture(t)
	f.MapPreviewRes = service.MapPreview{
		Project:     m.Projects[0],
		MapBody:     "line one\nline two\nline three\n" + strings.Repeat("filler line\n", 40),
		MapStale:    true,
		Diff:        []string{"- old header", "+ new header"},
		BankPresent: []string{"agent/PROJECT_MAP.md"},
	}
	m, cmd := updateC(t, m, runeKey("a"))
	pm, ok := cmd().(mapPreviewMsg)
	if !ok {
		t.Fatalf("cmd = %T, want mapPreviewMsg", cmd())
	}

	r, rf := rootOf(t)
	r.dashboard = m
	r, _ = upd(t, r, pm)
	if r.screen != ScreenMap {
		t.Fatalf("screen = %v, want map report", r.screen)
	}
	v := r.View()
	for _, want := range []string{"BANK & MAP - ", "map: stale", "PROJECT_MAP.md", "line one", "CONTENT"} {
		if !strings.Contains(v, want) {
			t.Errorf("report view missing %q in %q", want, v)
		}
	}
	// the legacy flat body still carries the diff and AGENTS sections
	all := strings.Join(r.mapLines(), "\n")
	for _, want := range []string{"DIFF vs disk (i improves):", "- old header", "+ new header", "AGENTS.md"} {
		if !strings.Contains(all, want) {
			t.Errorf("report body missing %q in %q", want, all)
		}
	}

	// tab focuses the content panel; j/k then scroll it
	tab := tea.KeyMsg{Type: tea.KeyTab}
	r, _ = upd(t, r, tab)
	if r.mapFocus != 1 {
		t.Fatalf("focus = %d, want 1 after tab", r.mapFocus)
	}
	before := r.View()
	r, _ = upd(t, r, keyR('j'))
	if r.mapScroll != 1 {
		t.Fatalf("scroll = %d, want 1 after j", r.mapScroll)
	}
	if r.View() == before {
		t.Fatal("scrolling must move the window on long content")
	}
	r, _ = upd(t, r, keyR('k'))
	if r.mapScroll != 0 {
		t.Fatalf("scroll = %d, want 0 after k", r.mapScroll)
	}
	// tab back to the file list: j moves the cursor, not the scroll
	r, _ = upd(t, r, tab)
	if r.mapFocus != 0 {
		t.Fatalf("focus = %d, want 0 after second tab", r.mapFocus)
	}
	r, _ = upd(t, r, keyR('j'))
	if r.mapCursor != 1 || r.mapScroll != 0 {
		t.Fatalf("cursor = %d scroll = %d, want cursor 1 scroll 0", r.mapCursor, r.mapScroll)
	}

	// i inside the report improves (rebuilds) AND reloads the preview
	r, cmd = upd(t, r, keyR('i'))
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("rebuild returned %T, want a batch", cmd())
	}
	var done, prev tea.Msg
	for _, c := range batch {
		switch x := c().(type) {
		case mapDoneMsg:
			done = x
		case mapPreviewMsg:
			prev = x
		}
	}
	dm, ok := done.(mapDoneMsg)
	if !ok || dm.err != nil || dm.q != "a" {
		t.Fatalf("done = %#v, want successful mapDoneMsg for a", done)
	}
	if _, ok := prev.(mapPreviewMsg); !ok {
		t.Fatalf("prev = %#v, want the preview reload", prev)
	}
	if !contains(rf.Calls(), "Map a") {
		t.Errorf("root svc calls = %v, want the rebuild", rf.Calls())
	}
	r, cmd = upd(t, r, dm)
	if cmd == nil {
		t.Fatal("successful rebuild must toast and refresh")
	}
	var toast tea.Msg
	for _, c := range flattenBatch(t, cmd) {
		if tm, ok := c().(toastMsg); ok {
			toast = tm
		}
	}
	tm, ok := toast.(toastMsg)
	if !ok || tm.t.Level != "good" || !strings.Contains(tm.t.Text, "bank + map improved for a") {
		t.Fatalf("toast = %#v, want the build success", toast)
	}
	r, _ = upd(t, r, prev)
	if r.screen != ScreenMap {
		t.Errorf("screen = %v, want to stay on the report", r.screen)
	}

	// esc returns to the dashboard
	r, _ = upd(t, r, keyEsc())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after esc", r.screen)
	}
}

// TestMapReportErrorAndMissingFiles: a failed preview lands in the sticky
// error banner without leaving the dashboard, and missing bank files
// explain how to fill them.
func TestMapReportErrorAndMissingFiles(t *testing.T) {
	m, f := scanFixture(t)
	f.MapPreviewErr = errors.New("db locked")
	m, cmd := updateC(t, m, runeKey("a"))
	r, _ := rootOf(t)
	r.dashboard = m
	r, cmd = upd(t, r, cmd())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after a failed preview", r.screen)
	}
	if cmd != nil {
		t.Errorf("failed preview must not queue work, got %v", cmd)
	}
	if len(r.errs) != 1 || !strings.Contains(r.errs[0].Text, "could not open the map report: db locked") {
		t.Fatalf("errs = %v, want the sticky failure", r.errs)
	}

	// empty bodies: the report tells the user a rebuild fills them in
	f.MapPreviewErr = nil
	f.MapPreviewRes = service.MapPreview{
		Project:     m.Projects[0],
		BankMissing: []string{"agent/PROJECT_MAP.md"},
	}
	m, cmd = updateC(t, m, runeKey("a"))
	r, _ = upd(t, r, cmd().(mapPreviewMsg))
	if r.screen != ScreenMap {
		t.Fatalf("screen = %v, want the report", r.screen)
	}
	v := r.View()
	if !strings.Contains(v, "press i to rebuild") {
		t.Errorf("report = %q, want the rebuild hint", v)
	}
	if !strings.Contains(v, "CONTENT 1/") {
		t.Errorf("report = %q, want a content line counter", v)
	}
}

// TestProjectFileTreeNestsFolders: the Bank & Map left panel renders a
// nested tree of the project directory (P12.02).
func TestProjectFileTreeNestsFolders(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel string) {
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel string) {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("src")
	mk("docs")
	write("README.md")
	write(filepath.Join("src", "main.go"))
	lines := projectFileTree(dir, 3)
	all := strings.Join(lines, "\n")
	for _, want := range []string{"src/", "docs/", "README.md", "main.go"} {
		if !strings.Contains(all, want) {
			t.Errorf("tree missing %q in %q", want, all)
		}
	}
	// junk dirs are skipped
	mk("node_modules")
	write(filepath.Join("node_modules", "junk.js"))
	lines = projectFileTree(dir, 3)
	if strings.Contains(strings.Join(lines, "\n"), "junk.js") {
		t.Error("tree must skip node_modules")
	}
	if projectFileTree("", 3) != nil {
		t.Error("empty root must return nil")
	}
}

// flattenBatch unwraps nested BatchMsgs so tests can run every child.
func flattenBatch(t *testing.T, cmd tea.Cmd) []tea.Cmd {
	t.Helper()
	var out []tea.Cmd
	var walk func(msg tea.Msg)
	walk = func(msg tea.Msg) {
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				walk(c())
			}
			return
		}
		out = append(out, func() tea.Msg { return msg })
	}
	walk(cmd())
	return out
}
