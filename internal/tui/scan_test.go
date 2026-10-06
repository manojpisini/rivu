package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/service/fake"
)

// scanFixture returns the dashboard model with its service wired to a
// fake whose List mirrors the model's own projects.
func scanFixture(t *testing.T) (Model, *fake.Service) {
	t.Helper()
	m := testModel(t)
	f := &fake.Service{Projects: m.Projects}
	m.svc = f
	return m, f
}

// updateC is update() but keeps the returned command.
func updateC(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	nm, cmd := m.Update(msg)
	next, ok := nm.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", nm)
	}
	return next, cmd
}

func TestRefreshStartsAsyncScan(t *testing.T) {
	m, f := scanFixture(t)
	m, cmd := updateC(t, m, runeKey("r"))
	if !m.scanning {
		t.Fatal("refresh must enter the scanning state")
	}
	if cmd == nil {
		t.Fatal("refresh must return spinner + scan commands")
	}
	if calls := f.Calls(); len(calls) != 0 {
		t.Errorf("scan must run async, not synchronously: %v", calls)
	}
	if foot := m.footer(); !strings.Contains(foot, "scanning") || !strings.Contains(foot, "dirs") {
		t.Errorf("footer = %q, want spinner with dir count", foot)
	}
	// second refresh while running must say so, not start another scan
	m, _ = updateC(t, m, runeKey("r"))
	if !strings.Contains(m.Status, "already running") {
		t.Errorf("Status = %q, want already-running notice", m.Status)
	}
}

func TestRefreshWithoutServiceExplains(t *testing.T) {
	m := testModel(t)
	m, cmd := updateC(t, m, runeKey("r"))
	if m.scanning || cmd != nil {
		t.Fatal("nil service must not start a scan")
	}
	if !strings.Contains(m.Status, "no service") {
		t.Errorf("Status = %q, want why the scan is unavailable", m.Status)
	}
}

func TestScanDoneReloadsList(t *testing.T) {
	m, f := scanFixture(t)
	f.ScanRes = service.ScanResult{Warnings: []string{"w1"}}
	m, _ = updateC(t, m, runeKey("r"))

	msg := scanCmd(f, context.Background(), nil)()
	d, ok := msg.(scanDoneMsg)
	if !ok {
		t.Fatalf("scanCmd returned %T, want scanDoneMsg", msg)
	}
	if d.err != nil || d.cancelled {
		t.Fatalf("unexpected outcome: %+v", d)
	}
	m, _ = updateC(t, m, d)
	if m.scanning {
		t.Error("scan must stop when done arrives")
	}
	if len(m.Projects) != 2 {
		t.Errorf("Projects = %d, want the 2 from List", len(m.Projects))
	}
	if !strings.Contains(m.Status, "Scan done: 2 projects") ||
		!strings.Contains(m.Status, "1 warnings") {
		t.Errorf("Status = %q, want counts and warnings", m.Status)
	}
	if !contains(f.Calls(), "ScanContext") {
		t.Errorf("Calls = %v, want ScanContext", f.Calls())
	}
}

func TestScanEscCancelsViaContext(t *testing.T) {
	m, f := scanFixture(t)
	m, _ = updateC(t, m, runeKey("r"))
	m, _ = updateC(t, m, keyEsc())
	if !m.scanning {
		t.Fatal("cancel is async: keep scanning until the done message arrives")
	}
	if !strings.Contains(m.Status, "Cancelling") {
		t.Errorf("Status = %q, want cancelling notice", m.Status)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	msg := scanCmd(f, ctx, nil)()
	d := msg.(scanDoneMsg)
	if !d.cancelled {
		t.Fatalf("cancelled context must yield cancelled outcome, got %+v", d)
	}
	m, _ = updateC(t, m, d)
	if m.scanning {
		t.Error("done message must clear the scanning state")
	}
	if !strings.Contains(m.Status, "cancelled") {
		t.Errorf("Status = %q, want cancelled notice", m.Status)
	}
}

func TestScanProgressReachesSpinnerFooter(t *testing.T) {
	m, _ := scanFixture(t)
	m, _ = updateC(t, m, runeKey("r"))
	if m.scanCount == nil {
		t.Fatal("refresh must arm the progress counter")
	}
	m.scanCount.Store(7)
	m, cmd := updateC(t, m, m.spin.Tick())
	if m.scanDirs != 7 {
		t.Errorf("scanDirs = %d, want 7 from the counter", m.scanDirs)
	}
	if cmd == nil {
		t.Fatal("spinner tick must re-arm itself while scanning")
	}
	if !strings.Contains(m.footer(), "7 dirs") {
		t.Errorf("footer = %q, want the dir count", m.footer())
	}
	// after the scan stops, spinner ticks are ignored (no leaked timers)
	m.scanning = false
	m, cmd = updateC(t, m, m.spin.Tick())
	if cmd != nil {
		t.Error("no spinner re-arm once scanning is over")
	}
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
