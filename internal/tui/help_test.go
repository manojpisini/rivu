package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpOverlayToggles(t *testing.T) {
	m := testModel(t)
	if !key.Matches(runeKey("?"), keys.Help) {
		t.Fatal("? must be bound to help")
	}
	m, cmd := updateC(t, m, runeKey("?"))
	if !m.helpOpen || cmd != nil {
		t.Fatalf("? must open the overlay without a command, open=%v cmd=%v", m.helpOpen, cmd)
	}
	v := m.View()
	for _, want := range []string{"KEYS", "switch panel", "help", "? or esc to close"} {
		if !strings.Contains(v, want) {
			t.Errorf("help missing %q in %q", want, v)
		}
	}
	// ? closes again
	m, _ = updateC(t, m, runeKey("?"))
	if m.helpOpen {
		t.Fatal("? must close the overlay")
	}
	// esc closes too
	m, _ = updateC(t, m, runeKey("?"))
	m, _ = updateC(t, m, keyEsc())
	if m.helpOpen {
		t.Fatal("esc must close the overlay")
	}
	// closing restores the dashboard
	if !strings.Contains(m.View(), "PROJECTS") {
		t.Errorf("dashboard must return after closing help: %q", m.View())
	}
}

func TestHelpSwallowsKeysAndQuits(t *testing.T) {
	m := testModel(t)
	m, _ = updateC(t, m, runeKey("?"))
	for _, k := range []tea.KeyMsg{runeKey("r"), runeKey("f"), runeKey("d"), runeKey("1")} {
		m, cmd := updateC(t, m, k)
		if !m.helpOpen || cmd != nil {
			t.Fatalf("key %v must be swallowed by the overlay, open=%v cmd=%v", k, m.helpOpen, cmd)
		}
	}
	_, cmd := updateC(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c must still quit from help")
	}
}

func TestHelpOverlaysDetailAndPicker(t *testing.T) {
	m := testModel(t)
	m, _ = updateC(t, m, runeKey("d")) // full-screen detail
	if !m.DetailFull {
		t.Fatal("setup: d opens detail")
	}
	m, _ = updateC(t, m, runeKey("?"))
	if !m.helpOpen {
		t.Fatal("? must open over the detail screen")
	}
	if !strings.Contains(m.View(), "KEYS") {
		t.Fatal("help must take render priority")
	}
	m, _ = updateC(t, m, keyEsc())
	if !m.DetailFull || m.helpOpen {
		t.Fatalf("closing help must return to detail, full=%v help=%v", m.DetailFull, m.helpOpen)
	}
}
