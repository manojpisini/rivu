package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func testModel(t *testing.T) Model {
	t.Helper()
	m := New([]registry.Project{
		{Slug: "a", Name: "Alpha", FlowStage: "source"},
		{Slug: "b", Name: "Beta", FlowStage: "source"},
	}, `C:\ws`)
	nm, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd != nil {
		t.Fatal("WindowSizeMsg must not return a command")
	}
	m = nm.(Model)
	return m
}

// update applies a key and returns the new model, failing on panics in
// the type assertion so tests read linearly.
func update(t *testing.T, m Model, msg tea.KeyMsg) Model {
	t.Helper()
	nm, _ := m.Update(msg)
	next, ok := nm.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", nm)
	}
	return next
}

func TestShortHelpBindsKeyAndDescription(t *testing.T) {
	for _, b := range keys.ShortHelp() {
		if b.Help().Key == "" || b.Help().Desc == "" {
			t.Errorf("short help binding missing help text: key=%q desc=%q", b.Help().Key, b.Help().Desc)
		}
	}
	for _, group := range keys.FullHelp() {
		for _, b := range group {
			if b.Help().Key == "" || b.Help().Desc == "" {
				t.Errorf("full help binding missing help text: key=%q desc=%q", b.Help().Key, b.Help().Desc)
			}
		}
	}
}

func TestQuitReturnsQuitCommand(t *testing.T) {
	m := testModel(t)
	_, cmd := m.Update(runeKey("q"))
	if cmd == nil {
		t.Fatal("q must return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q returned %T, want tea.QuitMsg", cmd())
	}
	// ctrl+c shares the binding.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c must return a command")
	}
}

func TestNavigationKeysDriveHandlers(t *testing.T) {
	m := testModel(t)
	if !m.FocusSidebar {
		t.Fatal("new model must focus the sidebar")
	}
	m = update(t, m, runeKey("tab"))
	if m.FocusSidebar {
		t.Error("tab must switch panel")
	}
	m = update(t, m, runeKey("/"))
	if !m.Searching {
		t.Error("/ must enter search")
	}
	m.Searching = false
	m = update(t, m, runeKey("j"))
	if m.Cursor != 1 {
		t.Errorf("j cursor = %d, want 1", m.Cursor)
	}
	m = update(t, m, runeKey("k"))
	if m.Cursor != 0 {
		t.Errorf("k cursor = %d, want 0", m.Cursor)
	}
	m = update(t, m, runeKey("G"))
	if m.Cursor != 1 {
		t.Errorf("G cursor = %d, want last row (1)", m.Cursor)
	}
	m = update(t, m, runeKey("g"))
	if m.Cursor != 0 {
		t.Errorf("g cursor = %d, want 0", m.Cursor)
	}
	m.Query = "beta"
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.Query != "" {
		t.Errorf("esc query = %q, want cleared", m.Query)
	}
}

func TestFooterHelpAndStatus(t *testing.T) {
	m := testModel(t)
	view := m.View()
	for _, want := range []string{"switch panel", "quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("footer help missing %q:\n%s", want, view)
		}
	}
	m.Status = "Open with: rivu open alpha"
	if got := m.footer(); !strings.Contains(got, m.Status) {
		t.Errorf("status footer = %q, want status text", got)
	}
}

// TestFooterContextHints: the hint set follows the active context but
// always renders bindings from the single keyMap table.
func TestFooterContextHints(t *testing.T) {
	m := testModel(t)
	if hints := m.footerHints(); len(hints) == 0 {
		t.Fatal("normal context must offer hints")
	}
	normal := m.footer()
	for _, want := range []string{"switch panel", "search", "quit"} {
		if !strings.Contains(normal, want) {
			t.Errorf("normal footer missing %q: %s", want, normal)
		}
	}

	m = update(t, m, runeKey("/"))
	if !m.Searching {
		t.Fatal("/ must enter search")
	}
	search := m.footer()
	for _, want := range []string{"done", "cancel", "quit"} {
		if !strings.Contains(search, want) {
			t.Errorf("search footer missing %q: %s", want, search)
		}
	}
	for _, hint := range []string{"switch panel", "clear filter"} {
		if strings.Contains(search, hint) {
			t.Errorf("search footer must not show %q: %s", hint, search)
		}
	}
	for _, b := range m.footerHints() {
		if b.Help().Key == "" || b.Help().Desc == "" {
			t.Errorf("context binding missing help text: %+v", b.Help())
		}
	}
}

// TestStageSwitching: 1-5 jump to a Flow stage, left/right walk the
// sidebar stages with wrap, and left still switches panels elsewhere.
func TestStageSwitching(t *testing.T) {
	m := testModel(t)
	if m.FocusSidebar {
		m = update(t, m, runeKey("tab")) // start with the list focused
	}
	m = update(t, m, runeKey("2")) // active
	if m.FlowCursor != 2 {
		t.Fatalf("digit 2 -> FlowCursor %d, want 2 (active)", m.FlowCursor)
	}
	if flowOrder[m.FlowCursor] != "active" {
		t.Fatalf("stage = %q, want active", flowOrder[m.FlowCursor])
	}
	m = update(t, m, runeKey("3")) // maintenance
	if flowOrder[m.FlowCursor] != "maintenance" {
		t.Fatalf("stage = %q, want maintenance", flowOrder[m.FlowCursor])
	}
	m = update(t, m, runeKey("5")) // delta
	if m.FlowCursor != 5 {
		t.Fatalf("digit 5 -> FlowCursor %d, want 5 (delta)", m.FlowCursor)
	}
	for _, p := range m.Visible {
		if p.FlowStage != "delta" {
			t.Fatalf("stage filter leaked %q (flow %q)", p.Name, p.FlowStage)
		}
	}

	m = update(t, m, runeKey("tab")) // focus sidebar
	if !m.FocusSidebar {
		t.Fatal("tab must focus the sidebar")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.FlowCursor != 4 {
		t.Errorf("left in sidebar -> %d, want 4", m.FlowCursor)
	}
	m.FlowCursor = 0
	m = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.FlowCursor != len(flowOrder)-1 {
		t.Errorf("left must wrap to %d, got %d", len(flowOrder)-1, m.FlowCursor)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.FlowCursor != 0 {
		t.Errorf("right must wrap back to 0, got %d", m.FlowCursor)
	}

	m = update(t, m, runeKey("tab")) // back to the list
	m = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if !m.FocusSidebar {
		t.Error("left in the list must still switch panels")
	}
}

// TestSidebarShowsCounts: the Flow sidebar lists every stage with its
// project count.
func TestSidebarShowsCounts(t *testing.T) {
	m := New([]registry.Project{
		{ID: "1", Name: "alpha", FlowStage: "source"},
		{ID: "2", Name: "beta", FlowStage: "source"},
		{ID: "3", Name: "gamma", FlowStage: "delta"},
	}, `C:\ws`)
	side := m.sidebar(24, 30)
	for _, pattern := range []string{`FLOW`, `ALL\s+3`, `SOURCE\s+2`, `DELTA\s+1`} {
		if !regexp.MustCompile(pattern).MatchString(side) {
			t.Errorf("sidebar missing %s:\n%s", pattern, side)
		}
	}
}
