package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestEndToEndSearch (P3.35): drive the real Root through a tea
// program — type `/go<enter>`, assert the filtered model, then `q`.
func TestEndToEndSearch(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	tm := teatest.NewTestModel(t, goldenRoot(), teatest.WithInitialTermSize(100, 30))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "RIVU")
	}, teatest.WithDuration(3*time.Second))

	tm.Type("/go")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	tm.Send(keyR('q'))

	fm := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second))
	root, ok := fm.(Root)
	if !ok {
		t.Fatalf("final model = %T, want Root", fm)
	}
	m := root.dashboard
	if m.Query != "go" {
		t.Errorf("Query = %q, want %q", m.Query, "go")
	}
	if m.search.Focused() {
		t.Error("enter must leave the search input")
	}
	want := map[string]bool{"alpha": true, "gamma": true, "beta": false}
	if len(m.Visible) != 2 {
		t.Fatalf("visible = %d, want 2 (alpha, gamma)", len(m.Visible))
	}
	for _, p := range m.Visible {
		if !want[p.Slug] {
			t.Errorf("%s must not be visible for query %q", p.Slug, m.Query)
		}
	}
	if m.Cursor < 0 || m.Cursor >= len(m.Visible) {
		t.Errorf("cursor %d out of range after filtering", m.Cursor)
	}
}
