package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

func TestWindowTitle(t *testing.T) {
	for _, tc := range []struct {
		name string
		has  bool
		want string
	}{
		{"", false, "Rivu"},
		{"", true, "Rivu"},
		{"Alpha", true, "Rivu — Alpha"},
	} {
		if got := windowTitle(tc.name, tc.has); got != tc.want {
			t.Errorf("windowTitle(%q, %v) = %q, want %q", tc.name, tc.has, got, tc.want)
		}
	}
}

// TestRootSetsTitleOnSizeAndCurrent (P3.28): the title command rides
// the size message and every Current change.
func TestRootSetsTitleOnSizeAndCurrent(t *testing.T) {
	r, _ := rootOf(t)
	r, cmd := upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd == nil {
		t.Fatal("WindowSizeMsg must (re)issue the title command")
	}
	if r.width != 100 {
		t.Fatalf("width = %d, forward must still run", r.width)
	}
	r, cmd = upd(t, r, currentMsg{p: registry.Project{Name: "Alpha"}, ok: true})
	if cmd == nil {
		t.Fatal("setting Current must reissue the title command")
	}
	if !r.hasCurrent || r.dashboard.Current.Name != "Alpha" {
		t.Fatal("currentMsg must still update Current")
	}
	if got := windowTitle(r.current.Name, r.hasCurrent); got != "Rivu — Alpha" {
		t.Errorf("title = %q", got)
	}
}
