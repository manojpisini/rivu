package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestGlobalKeysInertWhileInputFocused (P3.14): while the search
// textinput has focus every global key types instead of acting —
// except ctrl+c, which always quits (spec 3.9), and esc/enter, the
// input's own cancel/confirm.
func TestGlobalKeysInertWhileInputFocused(t *testing.T) {
	m := searchFixture()
	m = update(t, m, runeKey("/"))
	if !m.search.Focused() {
		t.Fatal("/ must focus the textinput")
	}
	for _, k := range []tea.KeyMsg{
		runeKey("q"), runeKey("r"), runeKey("d"), runeKey("h"),
		runeKey("m"), runeKey("g"), runeKey("G"), runeKey("1"), runeKey("/"),
		runeKey("a"), runeKey("f"), runeKey("n"), runeKey("?"), runeKey("s"),
	} {
		nm, cmd := m.Update(k)
		if cmd != nil {
			if _, isQuit := cmd().(tea.QuitMsg); isQuit {
				t.Fatalf("key %v must not quit while the input is focused", k)
			}
		}
		m = nm.(Model)
	}
	if !m.search.Focused() {
		t.Fatal("global keys must not blur the input")
	}
	want := "qrdhmgG1/afn?s"
	if m.Query != want {
		t.Fatalf("query = %q, want every key typed verbatim (%q)", m.Query, want)
	}
	if m.Status != "" {
		t.Fatalf("global side effects fired (status %q)", m.Status)
	}

	// ctrl+c still quits immediately while focused.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c must return a command while focused")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("ctrl+c returned %T, want tea.QuitMsg", cmd())
	}
}

// TestQuitWorksAgainAfterBlur: leaving the input restores globals.
func TestQuitWorksAgainAfterBlur(t *testing.T) {
	m := searchFixture()
	m = update(t, m, runeKey("/"))
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.search.Focused() {
		t.Fatal("esc must blur the input")
	}
	_, cmd := m.Update(runeKey("q"))
	if cmd == nil {
		t.Fatal("q must quit again once the input is blurred")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("q returned %T, want tea.QuitMsg", cmd())
	}
}
