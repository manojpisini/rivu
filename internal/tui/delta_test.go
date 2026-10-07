package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/service"
)

// TestDeltaActionDedicatedCopy (P4.10): A opens the Plan modal with
// delta wording (not the generic Flow wording) and applies with the
// dedicated toast.
func TestDeltaActionDedicatedCopy(t *testing.T) {
	m, f := pickFixture(t)
	f.FlowRes = service.FlowResult{
		Plan: service.FlowPlan{
			Query: "a", FromStage: "source", ToStage: "delta",
			Move: []string{"a -> 90_Delta/a"},
		},
	}
	m, cmd := updateC(t, m, keyR('A'))
	if cmd == nil {
		t.Fatal("A must request the delta dry run")
	}
	pm, ok := cmd().(flowPlanMsg)
	if !ok || pm.err != nil || !pm.delta || pm.stage != "delta" {
		t.Fatalf("plan = %#v, want delta flowPlanMsg", cmd())
	}
	if !contains(f.Calls(), "Flow a -> delta") {
		t.Fatalf("Calls = %v, want the delta dry run", f.Calls())
	}

	r, _ := rootOf(t)
	r.dashboard = m
	r, cmd2 := upd(t, r, pm)
	if cmd2 == nil {
		t.Fatal("delta plan must open the modal")
	}
	r, _ = upd(t, r, cmd2())
	if len(r.confirms) != 1 {
		t.Fatalf("confirms = %d, want the delta modal", len(r.confirms))
	}
	v := r.View()
	if !strings.Contains(v, "Delta a?") || !strings.Contains(v, "project files stay untouched") {
		t.Errorf("modal missing dedicated delta copy: %q", v)
	}
	if strings.Contains(v, "Flow a: source -> delta?") {
		t.Errorf("modal must not use the generic flow title: %q", v)
	}

	r, cmd3 := upd(t, r, keyR('y'))
	if len(r.confirms) != 0 {
		t.Fatal("y must close the modal")
	}
	am, ok := cmd3().(flowApplyMsg)
	if !ok || am.err != nil || !am.delta {
		t.Fatalf("apply = %#v, want delta flowApplyMsg", cmd3())
	}
	_, cmd4 := updateC(t, m, am)
	if cmd4 == nil {
		t.Fatal("delta apply must toast and refresh")
	}
	batch, ok := cmd4().(tea.BatchMsg)
	if !ok {
		t.Fatalf("apply returned %T, want a batch", cmd4())
	}
	var toast tea.Msg
	for _, c := range batch {
		if tm, ok := c().(toastMsg); ok {
			toast = tm
		}
	}
	tm, ok := toast.(toastMsg)
	if !ok || tm.t.Level != "good" || !strings.Contains(tm.t.Text, "Deltaed") || !strings.Contains(tm.t.Text, "project files untouched") {
		t.Fatalf("toast = %#v, want the dedicated delta copy", toast)
	}
	if n := countCalls(f.Calls(), "Flow a -> delta"); n != 2 {
		t.Errorf("Calls = %v, want dry run + apply", f.Calls())
	}
}

// TestDeltaGuards (P4.10): already at Delta, no selection and no
// service all explain instead of calling the service.
func TestDeltaGuards(t *testing.T) {
	m := testModel(t)
	m, cmd := updateC(t, m, keyR('A'))
	if cmd != nil || !strings.Contains(m.Status, "no service") {
		t.Fatalf("svc guard: cmd=%v status=%q", cmd, m.Status)
	}

	m, f := scanFixture(t)
	m.FocusSidebar = false
	m.Projects[0].FlowStage = "delta"
	m.applyFilter()
	m, cmd = updateC(t, m, keyR('A')) // cursor sits on the delta project
	if cmd != nil || !strings.Contains(m.Status, "already at Delta") {
		t.Fatalf("already-delta guard: cmd=%v status=%q", cmd, m.Status)
	}

	m.Projects = nil
	m.applyFilter()
	m, cmd = updateC(t, m, keyR('A'))
	if cmd != nil || !strings.Contains(m.Status, "Select a project to delta") {
		t.Fatalf("selection guard: cmd=%v status=%q", cmd, m.Status)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("Calls = %v, want none", f.Calls())
	}
}
