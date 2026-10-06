package tui

import (
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
