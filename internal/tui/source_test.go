package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keyTab() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyTab} }
func keyShiftTab() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyShiftTab}
}
func keyCtrlS() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlS} }

// openSource runs the `n` dispatch command and feeds the resulting
// screen switch back into the model.
func openSource(t *testing.T, r Root) Root {
	t.Helper()
	r, cmd := upd(t, r, runeKey("n"))
	if cmd != nil {
		r, _ = upd(t, r, cmd())
	}
	return r
}

// TestSourceWizardScaffold (P4.01): n opens the wizard, the indicator
// shows all five steps, tab/shift+tab walk the fields (clamped), enter
// advances steps, ctrl+s teaches what is next, esc and q cancel.
func TestSourceWizardScaffold(t *testing.T) {
	r, _ := rootOf(t)
	r = openSource(t, r)
	if r.screen != ScreenSource {
		t.Fatalf("screen = %s, want source", screenNames[r.screen])
	}
	v := r.View()
	for _, want := range []string{"Source — Identity", "1 Identity", "2 Classification", "3 Stack", "4 Automation", "5 Dry run", "Name:"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}

	// tab walks fields, clamped at the end of the step
	for range 5 {
		r, _ = upd(t, r, keyTab())
	}
	if r.src.field != 2 { // Identity has 3 fields
		t.Errorf("field = %d, want clamped at 2", r.src.field)
	}
	// shift+tab walks back, clamped at 0
	for range 5 {
		r, _ = upd(t, r, keyShiftTab())
	}
	if r.src.field != 0 {
		t.Errorf("field = %d, want clamped at 0", r.src.field)
	}

	// enter advances steps and resets the field; clamps at the last step
	for range 6 {
		r, _ = upd(t, r, keyEnter())
	}
	if r.src.step != len(sourceSteps)-1 || r.src.field != 0 {
		t.Fatalf("step = %d field = %d, want last step", r.src.step, r.src.field)
	}
	v = r.View()
	if !strings.Contains(v, "Source — Dry run") || !strings.Contains(v, "✓ 1 Identity") || !strings.Contains(v, "▸ 5 Dry run") {
		t.Errorf("step 5 view missing updated indicator: %q", v)
	}
	r, _ = upd(t, r, keyCtrlS())
	if !strings.Contains(r.dashboard.Status, "Nothing created") {
		t.Errorf("ctrl+s at dry run = %q", r.dashboard.Status)
	}

	// esc cancels everything; the wizard resets
	r, _ = upd(t, r, keyEsc())
	if r.screen != ScreenDashboard || r.src != (sourceWizard{}) {
		t.Errorf("esc: screen=%s src=%+v", screenNames[r.screen], r.src)
	}

	// ctrl+s on a mid step teaches the way forward
	r = openSource(t, r)
	r, _ = upd(t, r, keyCtrlS())
	if !strings.Contains(r.dashboard.Status, "Step 1 of 5") {
		t.Errorf("ctrl+s status = %q", r.dashboard.Status)
	}

	// q also cancels
	r, _ = upd(t, r, keyEnter())
	r, _ = upd(t, r, keyR('q'))
	if r.screen != ScreenDashboard || r.src.step != 0 {
		t.Errorf("q: screen=%s src=%+v", screenNames[r.screen], r.src)
	}

	// ctrl+c always quits
	r = openSource(t, r)
	var cmd tea.Cmd
	r, cmd = upd(t, r, tea.KeyMsg{Type: tea.KeyCtrlC})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c returned %T, want QuitMsg", cmd())
	}
}

// TestSourceWizardFreshOnOpen: leaving and reopening starts over.
func TestSourceWizardFreshOnOpen(t *testing.T) {
	r, _ := rootOf(t)
	r = openSource(t, r)
	r, _ = upd(t, r, keyEnter())
	r, _ = upd(t, r, keyTab())
	r, _ = upd(t, r, keyEsc())
	r = openSource(t, r)
	if r.src != (sourceWizard{}) {
		t.Errorf("reopened wizard = %+v, want fresh", r.src)
	}
}
