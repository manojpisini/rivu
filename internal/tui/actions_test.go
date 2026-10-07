package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestActionsMenuOpensRunsAndCloses (P4.11): x opens the menu with all
// actions for the selection, enter runs the highlighted one through the
// normal key path, esc cancels, foreign keys stay swallowed.
func TestActionsMenuOpensRunsAndCloses(t *testing.T) {
	m, _ := pickFixture(t)

	m, cmd := updateC(t, m, keyR('x'))
	if cmd != nil || !m.actionMenu {
		t.Fatalf("x must open the menu: cmd=%v menu=%v", cmd, m.actionMenu)
	}
	v := m.View()
	for _, want := range []string{"ACTIONS — Alpha", "enter  open in preferred editor", "f  flow to another stage", "esc cancel"} {
		if !strings.Contains(v, want) {
			t.Errorf("menu missing %q in %q", want, v)
		}
	}

	// foreign keys are swallowed while the menu is open
	m, cmd = updateC(t, m, keyR('r'))
	if !m.actionMenu || cmd != nil {
		t.Fatal("menu must swallow refresh")
	}

	// down to detail and run it through the same key path as `d`
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, cmd = updateC(t, m, keyEnter())
	if m.actionMenu {
		t.Fatal("running an action must close the menu")
	}
	if !m.DetailFull {
		t.Fatal("enter on detail must open the detail view")
	}
	if cmd != nil {
		t.Fatalf("detail must not need a command, got %v", cmd())
	}
	m, _ = updateC(t, m, keyEsc())
	if m.DetailFull {
		t.Fatal("esc must leave the detail view")
	}

	// esc cancels the menu
	m, _ = updateC(t, m, keyR('x'))
	m, _ = updateC(t, m, keyEsc())
	if m.actionMenu {
		t.Fatal("esc must close the menu")
	}

	// a command-returning action hands back its command without running it
	m, _ = updateC(t, m, keyR('x'))
	for i := 0; i < 6; i++ { // to copy path (y)
		m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m, cmd = updateC(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("copy path must return its command")
	}
	if m.actionMenu {
		t.Fatal("the menu must close after dispatch")
	}
}

// TestActionsMenuGuards: no selection explains instead of opening.
func TestActionsMenuGuards(t *testing.T) {
	m, _ := pickFixture(t)
	m.Projects = nil
	m.applyFilter()
	m, cmd := updateC(t, m, keyR('x'))
	if cmd != nil || m.actionMenu {
		t.Fatalf("no selection must not open the menu: cmd=%v menu=%v", cmd, m.actionMenu)
	}
	if !strings.Contains(m.Status, "Select a project to see its actions") {
		t.Errorf("Status = %q, want the selection hint", m.Status)
	}
}
