package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

// TestActionGuardsExplainThemselves (P3.33): every action key without
// a service or without a selection returns teaching status, no command.
func TestActionGuardsExplainThemselves(t *testing.T) {
	// no service
	m := testModel(t)
	for _, tc := range []struct {
		key  string
		want string
	}{
		{"h", "Health checks unavailable"},
		{"a", "Agent map unavailable"},
		{"f", "Flow unavailable"},
		{"g", "Master dashboard unavailable"},
		{"r", "no service"},
	} {
		m, cmd := updateC(t, m, runeKey(tc.key))
		if cmd != nil || !strings.Contains(m.Status, tc.want) {
			t.Errorf("%s without svc: cmd=%v status=%q, want %q", tc.key, cmd, m.Status, tc.want)
		}
	}

	// no selection
	m, _ = scanFixture(t)
	m.Projects = nil
	m.applyFilter()
	for _, tc := range []struct {
		key  string
		want string
	}{
		{"h", "Running health checks"}, // doctor with nothing selected checks ALL — no guard
		{"a", "Select a project to open its map report"},
		{"o", "Select a project to reveal its folder"},
		{"f", "Select a project to flow"},
	} {
		m, cmd := updateC(t, m, runeKey(tc.key))
		if tc.key == "h" {
			if cmd == nil {
				t.Error("doctor with an empty list must check all projects")
			}
			continue
		}
		if cmd != nil || !strings.Contains(m.Status, tc.want) {
			t.Errorf("%s without selection: cmd=%v status=%q, want %q", tc.key, cmd, m.Status, tc.want)
		}
	}
	// enter is the open key: without a selection it explains itself
	m, cmd := updateC(t, m, keyEnter())
	if cmd != nil || !strings.Contains(m.Status, "Select a project to open") {
		t.Errorf("enter without selection: cmd=%v status=%q", cmd, m.Status)
	}
}

// TestFlowPickerArrowKeysClamp: up at the top, down at the ceiling —
// guards hold, in-between moves work.
func TestFlowPickerArrowKeysClamp(t *testing.T) {
	m, _ := scanFixture(t)
	m, _ = updateC(t, m, runeKey("f"))
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.flowPickCursor != 0 {
		t.Fatalf("up at ceiling 0 = %d, want 0", m.flowPickCursor)
	}
	ceiling := len(flowOrder) - 2
	for range ceiling + 3 {
		m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.flowPickCursor != ceiling {
		t.Fatalf("cursor = %d after many downs, want ceiling %d", m.flowPickCursor, ceiling)
	}
	m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.flowPickCursor != ceiling-1 {
		t.Fatalf("cursor = %d after up, want %d", m.flowPickCursor, ceiling-1)
	}
}

// TestFlowApplyEmptyNoteFallsBack: an apply result without a Note
// still toasts what happened (P3.33 covers the fallback branch).
func TestFlowApplyEmptyNoteFallsBack(t *testing.T) {
	m, _ := scanFixture(t)
	m, cmd := updateC(t, m, flowApplyMsg{slug: "a", stage: "active"})
	if cmd == nil {
		t.Fatal("successful apply must toast and refresh")
	}
	// the command is a Batch (toast + list refresh) — find the toast
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd returned %#v, want a batch", cmd())
	}
	found := false
	for _, c := range batch {
		if tm, ok := c().(toastMsg); ok && strings.Contains(tm.t.Text, "a moved to active") {
			found = true
		}
	}
	if !found {
		t.Fatal("generated note toast not found in the batch")
	}
}

// TestRootInterceptsAsyncMsgsOffScreen (P3.33): messages that arrive
// while another screen is active must still reach the dashboard —
// forward() would drop them.
func TestRootInterceptsAsyncMsgsOffScreen(t *testing.T) {
	r, _ := rootOf(t)
	r.screen = ScreenHealth // forward() swallows everything here
	r.dashboard.scanning = true

	// scan done reloads despite the other screen
	r, _ = upd(t, r, scanDoneMsg{ps: []registry.Project{{ID: "x", Slug: "x", Name: "X"}}})
	if r.dashboard.scanning {
		t.Error("scanDoneMsg must clear the scanning state off-screen")
	}
	if len(r.dashboard.Projects) != 1 {
		t.Fatalf("projects = %d, want scan results folded in", len(r.dashboard.Projects))
	}

	// editor returning refreshes
	r, cmd := upd(t, r, editorDoneMsg{slug: "x"})
	if cmd == nil {
		t.Error("editorDoneMsg must request a list refresh off-screen")
	}

	// map failure toasts
	r, cmd = upd(t, r, mapDoneMsg{q: "x", err: errors.New("boom")})
	if cmd == nil {
		t.Fatal("mapDoneMsg must surface a command off-screen")
	}
	tm, ok := cmd().(toastMsg)
	if !ok || tm.t.Level != "bad" {
		t.Fatalf("map toast = %#v", tm)
	}

	// flow plan opens the modal off-screen
	r, cmd = upd(t, r, flowPlanMsg{slug: "x", stage: "active"})
	if cmd == nil {
		t.Fatal("flowPlanMsg must return the confirm command")
	}
	r, _ = upd(t, r, cmd())
	if len(r.confirms) != 1 {
		t.Fatalf("confirms = %d, want the Plan modal off-screen", len(r.confirms))
	}

	// a plain key on the other screen is swallowed by forward()
	r2, cmd2 := upd(t, r, keyR('j'))
	if cmd2 != nil || r2.dashboard.Cursor != r.dashboard.Cursor {
		t.Error("keys must not leak into the dashboard off-screen")
	}
}

// TestConfirmModalSwallowsOtherKeys (P3.33): while a modal is open
// only its own keys act; everything else is ignored, modal stays.
func TestConfirmModalSwallowsOtherKeys(t *testing.T) {
	r, _ := rootOf(t)
	r, cmd := upd(t, r, confirmMsg{c: confirm{Title: "Test?", OnYes: func() tea.Cmd { return nil }}})
	if cmd != nil || len(r.confirms) != 1 {
		t.Fatalf("setup: cmd=%v confirms=%d", cmd, len(r.confirms))
	}
	for _, k := range []tea.KeyMsg{keyR('j'), keyR('r'), keyR('d'), keyR('g')} {
		var c tea.Cmd
		r, c = upd(t, r, k)
		if c != nil || len(r.confirms) != 1 {
			t.Fatalf("key %v must be swallowed: cmd=%v confirms=%d", k, c, len(r.confirms))
		}
	}
	// esc dismisses
	r, _ = upd(t, r, keyEsc())
	if len(r.confirms) != 0 {
		t.Fatalf("esc must close the modal, confirms=%d", len(r.confirms))
	}
}

// TestStickyErrorsCapAtTen (P3.33): the banner keeps the newest ten.
func TestStickyErrorsCapAtTen(t *testing.T) {
	r, _ := rootOf(t)
	for i := range 12 {
		r, _ = upd(t, r, copyDoneMsg{err: fmt.Errorf("err%d", i)})
	}
	if len(r.errs) != 10 {
		t.Fatalf("errs = %d, want capped at 10", len(r.errs))
	}
	if r.errs[0] != "copy path failed: err2" || r.errs[9] != "copy path failed: err11" {
		t.Errorf("cap must keep the newest entries: %v", r.errs)
	}
}
