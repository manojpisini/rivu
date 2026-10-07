package tui

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

func TestOSC52Encoding(t *testing.T) {
	path := `C:\ws\demo`
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(path)) + "\x07"
	if got := osc52(path); got != want {
		t.Fatalf("osc52 = %q, want %q", got, want)
	}
}

// TestCopyAndRevealKeys (P3.29): y copies the path via OSC 52, o
// reveals the folder; both explain themselves without a selection, and
// `o` no longer opens the editor (that stays on enter).
func TestCopyAndRevealKeys(t *testing.T) {
	if key.Matches(runeKey("y"), keys.Copy) == false {
		t.Fatal("y must be bound to copy")
	}
	if key.Matches(runeKey("o"), keys.Reveal) == false {
		t.Fatal("o must be bound to reveal")
	}
	if key.Matches(runeKey("o"), keys.Open) {
		t.Fatal("o must no longer open the editor (P3.29 gives it to reveal)")
	}
	if key.Matches(keyEnter(), keys.Open) == false {
		t.Fatal("enter must still open the editor (spec 3.9)")
	}

	m, _ := scanFixture(t)
	m, cmd := updateC(t, m, runeKey("y"))
	if cmd == nil {
		t.Fatal("copy with a selection must return the OSC 52 command")
	}
	m, cmd = updateC(t, m, runeKey("o"))
	if cmd == nil {
		t.Fatal("reveal with a selection must return a launch command")
	}

	// no selection: teach, don't run
	m.Projects = nil
	m.applyFilter()
	m, cmd = updateC(t, m, runeKey("y"))
	if cmd != nil || !strings.Contains(m.Status, "Select a project to copy") {
		t.Fatalf("copy guard: cmd=%v status=%q", cmd, m.Status)
	}
	m, cmd = updateC(t, m, runeKey("o"))
	if cmd != nil || !strings.Contains(m.Status, "Select a project to reveal") {
		t.Fatalf("reveal guard: cmd=%v status=%q", cmd, m.Status)
	}
}

// TestCopyRevealOutcomesReachToasts: both done messages land in Root
// state — good toasts transient (with their 3 s tick), failures sticky
// on the error banner.
func TestCopyRevealOutcomesReachToasts(t *testing.T) {
	r, _ := rootOf(t)

	r, cmd := upd(t, r, copyDoneMsg{})
	if cmd == nil {
		t.Fatal("good toast must schedule its expiry tick")
	}
	if len(r.toasts) != 1 || r.toasts[0].Level != "good" || !strings.Contains(r.toasts[0].Text, "OSC52") {
		t.Fatalf("copy ok toast = %+v", r.toasts)
	}
	r, _ = upd(t, r, copyDoneMsg{err: errors.New("broken pipe")})
	if len(r.errs) == 0 || !strings.Contains(r.errs[len(r.errs)-1], "broken pipe") {
		t.Fatalf("copy failure must be sticky: %v", r.errs)
	}

	r, cmd = upd(t, r, revealDoneMsg{path: `C:\ws\demo`})
	if cmd == nil || len(r.toasts) == 0 ||
		!strings.Contains(r.toasts[len(r.toasts)-1].Text, `C:\ws\demo`) {
		t.Fatalf("reveal ok toast = %+v cmd=%v", r.toasts, cmd)
	}
	r, _ = upd(t, r, revealDoneMsg{path: `C:\ws\demo`, err: errors.New("no xdg-open")})
	if len(r.errs) < 2 || !strings.Contains(r.errs[len(r.errs)-1], "no xdg-open") {
		t.Fatalf("reveal failure must be sticky: %v", r.errs)
	}
}

// TestRevealTargetsSelectedProject: the launch command acts on the
// selected project's path (the argv itself is never started in tests).
func TestRevealTargetsSelectedProject(t *testing.T) {
	m, _ := scanFixture(t)
	m, cmd := updateC(t, m, runeKey("o"))
	if cmd == nil {
		t.Fatal("reveal must return a command")
	}
	if p, ok := m.selectedProject(); !ok || p.Slug != "a" {
		t.Fatalf("reveal target = %+v, want the selected project", p)
	}
}
