package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
)

func TestFlowOpensStagePicker(t *testing.T) {
	m, _ := scanFixture(t)
	m, cmd := updateC(t, m, runeKey("f"))
	if !m.flowPick || cmd != nil {
		t.Fatalf("f must open the picker without a command, pick=%v cmd=%v", m.flowPick, cmd)
	}
	v := m.View()
	for _, want := range []string{"FLOW — move Alpha", "> source", "enter plan"} {
		if !strings.Contains(v, want) {
			t.Errorf("picker missing %q in %q", want, v)
		}
	}
	if !strings.Contains(v, "(current)") {
		t.Errorf("current stage must be marked: %q", v)
	}

	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.flowPickCursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.flowPickCursor)
	}
	// foreign keys are swallowed while picking
	m, cmd = updateC(t, m, runeKey("r"))
	if !m.flowPick || cmd != nil {
		t.Fatal("picker must swallow refresh")
	}
	m, _ = updateC(t, m, keyEsc())
	if m.flowPick {
		t.Fatal("esc must cancel the picker")
	}
}

func TestFlowPickSameStageExplains(t *testing.T) {
	m, _ := scanFixture(t) // Alpha lives in source; cursor 0 is source
	m, _ = updateC(t, m, runeKey("f"))
	m, cmd := updateC(t, m, keyEnter())
	if cmd != nil {
		t.Fatal("same-stage pick must not call the service")
	}
	if !strings.Contains(m.Status, "already in source") {
		t.Errorf("Status = %q, want already-in notice", m.Status)
	}
	if m.flowPick {
		t.Error("picker must close")
	}
}

func TestFlowPlanThenApply(t *testing.T) {
	// the plan opens the confirm modal at the Root; keep the fixture's
	// fake behind the dashboard so dry-run and apply hit the same recorder
	m, f := scanFixture(t)
	f.FlowRes = service.FlowResult{
		Note: "Alpha moved to active",
		Plan: service.FlowPlan{
			Query: "a", FromStage: "source", ToStage: "active",
			Move:     []string{"a -> 01_Active/a"},
			Registry: []string{"update a flow_stage"},
			Bank:     []string{"update .metadata/project.toml"},
		},
	}
	m, _ = updateC(t, m, runeKey("f"))
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyDown}) // active
	m, cmd := updateC(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("cross-stage pick must request a dry-run plan")
	}
	pm, ok := cmd().(flowPlanMsg)
	if !ok || pm.err != nil {
		t.Fatalf("dry run = %#v, want flowPlanMsg", cmd())
	}
	if !contains(f.Calls(), "Flow a -> active") {
		t.Fatalf("Calls = %v, want the dry-run flow", f.Calls())
	}

	r, _ := rootOf(t)
	r.dashboard = m
	r, cmd2 := upd(t, r, pm)
	if cmd2 == nil {
		t.Fatal("plan must open the modal")
	}
	r, _ = upd(t, r, cmd2())
	if len(r.confirms) != 1 {
		t.Fatalf("confirms = %d, want the Plan modal", len(r.confirms))
	}
	v := r.View()
	for _, want := range []string{"Flow a: source -> active?", "Will move a -> 01_Active/a", "Registry:", "Bank:"} {
		if !strings.Contains(v, want) {
			t.Errorf("modal missing %q in %q", want, v)
		}
	}

	// y accepts: the OnYes closure runs the real apply
	r, cmd3 := upd(t, r, keyR('y'))
	if len(r.confirms) != 0 {
		t.Fatal("y must close the modal")
	}
	am, ok := cmd3().(flowApplyMsg)
	if !ok || am.err != nil {
		t.Fatalf("apply = %#v, want flowApplyMsg", am)
	}
	applied := 0
	for _, c := range f.Calls() {
		if c == "Flow a -> active" {
			applied++
		}
	}
	if applied != 2 {
		t.Errorf("Calls = %v, want dry-run + apply (2 flows)", f.Calls())
	}

	// success refreshes the list; errors are sticky
	m, cmd4 := updateC(t, m, am)
	if cmd4 == nil {
		t.Fatal("applied flow must toast and refresh")
	}
	_, cmd5 := updateC(t, m, flowApplyMsg{slug: "a", stage: "active", err: registry.ErrNotFound})
	tm, ok := cmd5().(toastMsg)
	if !ok || tm.t.Level != "bad" || !strings.Contains(tm.t.Text, "flow failed for a") {
		t.Fatalf("toast = %#v, want sticky failure", tm)
	}
}

func TestFlowErrorsAndGuards(t *testing.T) {
	// no service
	m := testModel(t)
	m, cmd := updateC(t, m, runeKey("f"))
	if cmd != nil || !strings.Contains(m.Status, "no service") {
		t.Fatalf("svc guard: cmd=%v status=%q", cmd, m.Status)
	}
	// no selection
	m, _ = scanFixture(t)
	m.Projects = nil
	m.applyFilter()
	m, cmd = updateC(t, m, runeKey("f"))
	if cmd != nil || !strings.Contains(m.Status, "Select a project") {
		t.Fatalf("selection guard: cmd=%v status=%q", cmd, m.Status)
	}
	// dry-run failure surfaces as a sticky toast
	m, f := scanFixture(t)
	f.FlowErr = registry.ErrNotFound
	m, _ = updateC(t, m, runeKey("f"))
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, cmd = updateC(t, m, keyEnter())
	pm := cmd().(flowPlanMsg)
	if pm.err == nil {
		t.Fatal("fake FlowErr must reach the message")
	}
	m, cmd = updateC(t, m, pm)
	tm, ok := cmd().(toastMsg)
	if !ok || tm.t.Level != "bad" || !strings.Contains(tm.t.Text, "cannot move a") {
		t.Fatalf("toast = %#v, want sticky dry-run failure", tm)
	}
}
