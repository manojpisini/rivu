package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/manojpisini/rivu/internal/registry"
)

func TestMapKeyIsAAndBuildsForSelection(t *testing.T) {
	if !key.Matches(runeKey("a"), keys.Map) {
		t.Fatal("a must be the build-map key (spec 3.9)")
	}
	if key.Matches(runeKey("m"), keys.Map) {
		t.Fatal("m must no longer be bound to build map")
	}
	m, f := scanFixture(t)
	m, cmd := updateC(t, m, runeKey("a"))
	if cmd == nil {
		t.Fatal("map build must run asynchronously")
	}
	if !strings.Contains(m.Status, "Building agent map") {
		t.Fatalf("Status = %q, want progress notice", m.Status)
	}
	if msg := cmd(); msg.(mapDoneMsg).q != "a" {
		t.Fatalf("q = %q, want the selected slug", msg.(mapDoneMsg).q)
	}
	if !contains(f.Calls(), "Map a") {
		t.Errorf("Calls = %v, want Map a", f.Calls())
	}
}

func TestMapWithoutSelectionTeaches(t *testing.T) {
	m, _ := scanFixture(t)
	m.Projects = nil
	m.applyFilter()
	m, cmd := updateC(t, m, runeKey("a"))
	if cmd != nil {
		t.Fatal("no selection means no work")
	}
	if !strings.Contains(m.Status, "Select a project") {
		t.Errorf("Status = %q, want a teaching message", m.Status)
	}
}

func TestMapDoneToastsAndRefreshes(t *testing.T) {
	m, _ := scanFixture(t)
	m, cmd := updateC(t, m, mapDoneMsg{q: "a"})
	if cmd == nil {
		t.Fatal("success must toast and refresh")
	}

	// error path: sticky toast naming the project
	m, cmd = updateC(t, m, mapDoneMsg{q: "a", err: registry.ErrNotFound})
	tm, ok := cmd().(toastMsg)
	if !ok || tm.t.Level != "bad" || !strings.Contains(tm.t.Text, "agent map failed for a") {
		t.Fatalf("toast = %#v, want sticky failure", tm)
	}
}
