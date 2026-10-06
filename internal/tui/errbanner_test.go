package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestErrorStickyExpandAndClear(t *testing.T) {
	r, _ := rootOf(t)
	long := "scan failed: permission denied reading " + strings.Repeat("x", 120)
	r, cmd := upd(t, r, ShowToast(Toast{Level: "bad", Text: long})())
	if cmd != nil {
		t.Fatal("errors must not schedule auto-expiry")
	}
	if len(r.errs) != 1 || len(r.toasts) != 0 {
		t.Fatalf("errs=%v toasts=%v, want the error sticky", r.errs, r.toasts)
	}
	if !strings.Contains(r.View(), "[e expand]") {
		t.Fatalf("banner must hint the expand key, got %q", r.View())
	}

	r, _ = upd(t, r, runeKey("e"))
	if !r.errExpand {
		t.Fatal("e must open the overlay")
	}
	v := r.View()
	if !strings.Contains(v, strings.Repeat("x", 120)) {
		t.Fatal("overlay must show the full untruncated error")
	}
	if !strings.Contains(v, "ERRORS (1)") || !strings.Contains(v, "clear all") {
		t.Fatalf("overlay needs a count and key hints, got %q", v)
	}

	// esc collapses but keeps the error sticky
	r, _ = upd(t, r, keyEsc())
	if r.errExpand || len(r.errs) != 1 {
		t.Fatalf("esc must collapse and keep the error, expand=%v errs=%v", r.errExpand, r.errs)
	}
	if !strings.Contains(r.View(), "[e expand]") {
		t.Fatal("banner must return after collapsing")
	}

	// x from the overlay clears everything
	r, _ = upd(t, r, runeKey("e"))
	r, _ = upd(t, r, runeKey("x"))
	if r.errExpand || len(r.errs) != 0 {
		t.Fatalf("x must clear, expand=%v errs=%v", r.errExpand, r.errs)
	}
}

func TestExpandKeyDoesNotStealSearchInput(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, ShowToast(Toast{Level: "bad", Text: "boom"})())
	nm, _ := r.dashboard.Update(runeKey("/"))
	r.dashboard = nm.(Model)

	r, _ = upd(t, r, runeKey("e"))
	if r.errExpand {
		t.Fatal("e must type into the focused search, not open the overlay")
	}
	if got := r.dashboard.search.Value(); got != "e" {
		t.Fatalf("search value = %q, want %q", got, "e")
	}
}

func TestErrorsSurviveScreenChange(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, ShowToast(Toast{Level: "bad", Text: "persistent failure"})())
	r, _ = upd(t, r, SelectScreenMsg{Screen: ScreenDetail})
	r, _ = upd(t, r, SelectScreenMsg{Screen: ScreenDashboard})
	if len(r.errs) != 1 || !strings.Contains(r.View(), "persistent failure") {
		t.Fatalf("error must survive a redraw and screen hop, errs=%v", r.errs)
	}
}

func TestOverlaySwallowsKeysAndQuitStillWorks(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, ShowToast(Toast{Level: "bad", Text: "boom"})())
	r, _ = upd(t, r, runeKey("e"))
	if r.screen != ScreenDashboard {
		t.Fatal("setup")
	}
	r, cmd := upd(t, r, runeKey("d"))
	if r.dashboard.DetailFull {
		t.Fatal("overlay must swallow keys instead of driving the screen")
	}
	if cmd != nil {
		t.Fatal("swallowed keys must not return commands")
	}
	r, cmd = upd(t, r, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c must still quit from the overlay")
	}
}
