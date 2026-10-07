package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/service/fake"
)

// pickFixture is scanFixture plus stable IDs, which the picked set
// keys on.
func pickFixture(t *testing.T) (Model, *fake.Service) {
	t.Helper()
	m, f := scanFixture(t)
	m.FocusSidebar = false // j must move the list cursor, not the stage filter
	m.Projects[0].ID = "id-a"
	m.Projects[1].ID = "id-b"
	m.applyFilter()
	f.Projects = m.Projects
	return m, f
}

// TestMultiSelectPick (P4.08): space toggles rows into the picked set,
// shows the count, marks the rows, and types when search is focused.
func TestMultiSelectPick(t *testing.T) {
	m, _ := pickFixture(t)

	m, cmd := updateC(t, m, keySpace())
	if cmd != nil {
		t.Fatal("picking must not need a command")
	}
	if !m.Picked["id-a"] {
		t.Fatalf("picked = %v, want id-a", m.Picked)
	}
	if !strings.Contains(m.Status, "1 picked") {
		t.Errorf("Status = %q, want the picked count", m.Status)
	}

	m, _ = updateC(t, m, runeKey("j"))
	m, _ = updateC(t, m, keySpace())
	if len(m.Picked) != 2 {
		t.Fatalf("picked = %v, want both rows", m.Picked)
	}
	if !strings.Contains(m.Status, "2 picked") {
		t.Errorf("Status = %q, want 2 picked", m.Status)
	}
	v := m.View()
	if !strings.Contains(v, "›✓") || !strings.Contains(v, "✓ ") {
		t.Errorf("view missing pick markers: %q", v)
	}

	// space toggles the cursor row back off
	m, _ = updateC(t, m, keySpace())
	if len(m.Picked) != 1 || !m.Picked["id-a"] {
		t.Fatalf("picked = %v, want only id-a", m.Picked)
	}

	// while search is focused, space types a literal space
	m, _ = updateC(t, m, runeKey("/"))
	m, _ = updateC(t, m, keySpace())
	if !strings.HasSuffix(m.search.Value(), " ") {
		t.Errorf("search value = %q, want a typed trailing space", m.search.Value())
	}
	if len(m.Picked) != 1 {
		t.Errorf("typing in search changed picked to %v", m.Picked)
	}
	m, _ = updateC(t, m, keyEsc())
}

// TestBulkFlowPlanAndApply (P4.08): picked rows win over the cursor,
// the Plan modal lists every move, y applies and the set clears.
func TestBulkFlowPlanAndApply(t *testing.T) {
	m, f := pickFixture(t)
	f.FlowRes = service.FlowResult{
		Plan: service.FlowPlan{
			Query: "a", FromStage: "source", ToStage: "active",
			Move: []string{"a -> 01_Active/a"},
		},
	}

	m, _ = updateC(t, m, keySpace())
	m, _ = updateC(t, m, runeKey("j"))
	m, _ = updateC(t, m, keySpace())

	m, _ = updateC(t, m, runeKey("f"))
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyDown}) // active
	m, cmd := updateC(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("bulk pick must request the dry-run plan")
	}
	bm, ok := cmd().(bulkFlowPlanMsg)
	if !ok || bm.err != nil {
		t.Fatalf("dry run = %#v, want bulkFlowPlanMsg", cmd())
	}
	if len(bm.slugs) != 2 || bm.stage != "active" {
		t.Fatalf("slugs=%v stage=%q, want both picked to active", bm.slugs, bm.stage)
	}
	if !contains(f.Calls(), "FlowBulk -> active") {
		t.Fatalf("Calls = %v, want the bulk dry run", f.Calls())
	}
	if !strings.Contains(m.Status, "2 moves") {
		t.Errorf("Status = %q, want the move count", m.Status)
	}

	// the modal opens at the Root and lists the plan
	r, _ := rootOf(t)
	r.dashboard = m
	r, cmd2 := upd(t, r, bm)
	if cmd2 == nil {
		t.Fatal("bulk plan must open the modal")
	}
	r, _ = upd(t, r, cmd2())
	if len(r.confirms) != 1 {
		t.Fatalf("confirms = %d, want the Plan modal", len(r.confirms))
	}
	v := r.View()
	for _, want := range []string{"Flow 2 projects to active?", "Will move a -> 01_Active/a"} {
		if !strings.Contains(v, want) {
			t.Errorf("modal missing %q in %q", want, v)
		}
	}

	// esc declines: no apply call, picks kept
	r, _ = upd(t, r, keyEsc())
	if len(r.confirms) != 0 {
		t.Fatal("esc must close the modal")
	}
	if n := countCalls(f.Calls(), "FlowBulk -> active"); n != 1 {
		t.Errorf("Calls = %v, want only the dry run after decline", f.Calls())
	}

	// reopen and accept: apply runs, the set clears, a toast reports it
	r.dashboard.flowPick = false
	r, cmd2 = upd(t, r, bm)
	r, _ = upd(t, r, cmd2())
	r, cmd3 := upd(t, r, keyR('y'))
	if len(r.confirms) != 0 {
		t.Fatal("y must close the modal")
	}
	am, ok := cmd3().(bulkFlowApplyMsg)
	if !ok || am.err != nil {
		t.Fatalf("apply = %#v, want bulkFlowApplyMsg", cmd3())
	}
	r, batch := upd(t, r, am)
	if r.dashboard.Picked != nil {
		t.Errorf("picked = %v after apply, want cleared", r.dashboard.Picked)
	}
	bm2, ok := batch().(tea.BatchMsg)
	if !ok {
		t.Fatalf("apply returned %T, want a batch", batch())
	}
	for _, c := range bm2 {
		r, _ = upd(t, r, c())
	}
	if len(r.toasts) == 0 || !strings.Contains(r.toasts[0].Text, "moved 2") {
		t.Errorf("toasts = %+v, want the moved summary", r.toasts)
	}
	if n := countCalls(f.Calls(), "FlowBulk -> active"); n != 2 {
		t.Errorf("Calls = %v, want dry run + apply", f.Calls())
	}
}

// TestBulkFlowAllAlreadyThere: picking rows that already sit in the
// target stage never reaches the service.
func TestBulkFlowAllAlreadyThere(t *testing.T) {
	m, f := pickFixture(t) // both projects are in source
	m, _ = updateC(t, m, keySpace())
	m, _ = updateC(t, m, runeKey("j"))
	m, _ = updateC(t, m, keySpace())

	m, _ = updateC(t, m, runeKey("f")) // picker opens on source
	m, cmd := updateC(t, m, keyEnter())
	if cmd != nil {
		t.Fatal("same-stage bulk pick must not call the service")
	}
	if !strings.Contains(m.Status, "already in source") {
		t.Errorf("Status = %q, want the already-there notice", m.Status)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("Calls = %v, want none", f.Calls())
	}
}

// countCalls counts entries containing sub.
func countCalls(calls []string, sub string) int {
	n := 0
	for _, c := range calls {
		if strings.Contains(c, sub) {
			n++
		}
	}
	return n
}
