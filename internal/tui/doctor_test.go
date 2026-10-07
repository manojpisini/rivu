package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/doctor"
	"github.com/manojpisini/rivu/internal/registry"
)

func TestDoctorChecksSelectedProject(t *testing.T) {
	m, f := scanFixture(t)
	m, cmd := updateC(t, m, runeKey("h"))
	if cmd == nil {
		t.Fatal("doctor must run asynchronously")
	}
	if !strings.Contains(m.Status, "health checks") {
		t.Fatalf("Status = %q, want progress notice", m.Status)
	}
	msg := cmd()
	dm, ok := msg.(doctorDoneMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want doctorDoneMsg", msg)
	}
	if dm.q != "a" {
		t.Errorf("q = %q, want the selected project slug", dm.q)
	}
	if !contains(f.Calls(), "Doctor a") {
		t.Errorf("Calls = %v, want Doctor a", f.Calls())
	}
}

func TestDoctorWithNoSelectionChecksAll(t *testing.T) {
	m, f := scanFixture(t)
	m.Projects = nil
	m.applyFilter()
	m, cmd := updateC(t, m, runeKey("h"))
	if cmd == nil {
		t.Fatal("doctor with no selection must still run")
	}
	if msg := cmd(); msg.(doctorDoneMsg).q != "" {
		t.Fatalf("q = %q, want empty (all projects)", msg.(doctorDoneMsg).q)
	}
	if !contains(f.Calls(), "Doctor ") {
		t.Errorf("Calls = %v, want an all-projects Doctor call", f.Calls())
	}
}

func TestDoctorDoneOpensHealthScreen(t *testing.T) {
	r, _ := rootOf(t)
	r.dashboard.Status = "Running health checks…"
	reports := []doctor.Report{
		{
			Project: registry.Project{Name: "demo", Slug: "demo"},
			Score:   72,
			Checks: []doctor.Check{
				{Name: "README", OK: true, Detail: "project documentation"},
				{Name: "CI", OK: false, Detail: "continuous integration"},
			},
		},
	}
	r, cmd := upd(t, r, doctorDoneMsg{reports: reports, q: "demo"})
	if cmd != nil {
		t.Fatal("successful doctor run needs no follow-up command")
	}
	if r.screen != ScreenHealth {
		t.Fatalf("screen = %v, want ScreenHealth", r.screen)
	}
	if r.dashboard.Status != "" {
		t.Errorf("progress status must clear, got %q", r.dashboard.Status)
	}
	v := r.View()
	for _, want := range []string{"HEALTH — demo", "demo", "72", "CI", "continuous integration"} {
		if !strings.Contains(v, want) {
			t.Errorf("health view missing %q in %q", want, v)
		}
	}
	// every check is a finding (P4.13): pass lines stay, failures
	// carry a fix, and the score is drawn as a bar
	if !strings.Contains(v, "project documentation") {
		t.Errorf("passing check must be listed as a finding: %q", v)
	}
	if !strings.Contains(v, "fix: add a workflow under .github/workflows") {
		t.Errorf("failing CI check must offer its remedy: %q", v)
	}
	if !strings.Contains(v, "█") || !strings.Contains(v, "72/100") {
		t.Errorf("score bar missing from %q", v)
	}

	// esc returns to the dashboard; ctrl+c quits
	r, _ = upd(t, r, keyEsc())
	if r.screen != ScreenDashboard {
		t.Errorf("screen = %v, want back to the dashboard", r.screen)
	}
	r, _ = upd(t, r, SelectScreenMsg{Screen: ScreenHealth})
	r, cmd = upd(t, r, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c must quit from a sub-screen")
	}
}

func TestDoctorErrorIsStickyAndKeepsDashboard(t *testing.T) {
	r, _ := rootOf(t)
	r, cmd := upd(t, r, doctorDoneMsg{err: registry.ErrNotFound})
	if cmd != nil {
		if tm, ok := cmd().(toastMsg); !ok || tm.t.Level != "bad" {
			t.Fatalf("cmd = %#v, want sticky error toast", cmd())
		}
	}
	if r.screen != ScreenDashboard {
		t.Errorf("screen = %v, want the dashboard kept on error", r.screen)
	}
	if len(r.errs) != 1 {
		t.Errorf("errs = %v, want the error sticky", r.errs)
	}
}

func TestSubScreenSwallowsForeignKeys(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, SelectScreenMsg{Screen: ScreenHealth})
	r, cmd := upd(t, r, runeKey("r")) // refresh belongs to the dashboard
	if r.screen != ScreenHealth {
		t.Fatal("foreign keys must not navigate away")
	}
	if cmd != nil {
		t.Fatal("foreign keys must not launch commands")
	}
}
