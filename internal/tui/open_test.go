package tui

import (
	"strings"
	"testing"

	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service/fake"
)

func TestGUILaunchClassification(t *testing.T) {
	gui := []string{"code", "zed", "subl", "idea"}
	cases := []struct {
		argv []string
		gui  bool
	}{
		{[]string{"code", "C:\\ws\\demo"}, true},
		{[]string{"/usr/bin/code", "/ws/demo"}, true},
		{[]string{"C:\\tools\\subl.exe", "p"}, true},
		{[]string{"zed", "p"}, true},
		{[]string{"code-insiders", "p"}, false},
		{[]string{"vim", "p"}, false},
		{[]string{"/usr/bin/nvim", "p"}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := guiLaunch(c.argv, gui); got != c.gui {
			t.Errorf("guiLaunch(%v) = %v, want %v", c.argv, got, c.gui)
		}
	}
}

func TestOpenTargetClassifiesAndReportsErrors(t *testing.T) {
	f := &fake.Service{} // fake resolves editor "code" for the default
	argv, tty, err := openTarget(f, "demo", "", []string{"code"})
	if err != nil {
		t.Fatalf("openTarget: %v", err)
	}
	if tty {
		t.Errorf("code is a GUI editor, tty = true (would wrongly seize the TTY)")
	}
	if len(argv) != 2 || argv[0] != "code" {
		t.Errorf("argv = %v, want [code path]", argv)
	}

	_, tty, err = openTarget(f, "demo", "", nil)
	if err != nil || !tty {
		t.Errorf("without a GUI list the editor is terminal: tty=%v err=%v", tty, err)
	}

	// the wizard's Editor row feeds the launch (P4.23)
	argv, _, err = openTarget(f, "demo", "nvim", nil)
	if err != nil || len(argv) == 0 || argv[0] != "nvim" {
		t.Errorf("openTarget with editor nvim = %v, err %v; want argv[0] nvim", argv, err)
	}

	f.OpenErr = registry.ErrNotFound
	if _, _, err := openTarget(f, "ghost", "", nil); err == nil {
		t.Error("missing project must surface the service error")
	}
}

func TestOpenKeysBuildLaunchCommands(t *testing.T) {
	// default cfg lists "code" as GUI -> Start() path (cmd non-nil, no TTY)
	m, _ := scanFixture(t)
	m.cfg = config.Default()
	m, cmd := updateC(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("GUI open must return a background start command")
	}
	if !contains(fCalls(m), "OpenCommand a") {
		t.Errorf("Calls = %v, want OpenCommand for the selected project", fCalls(m))
	}

	// empty GUI list -> terminal editor -> tea.ExecProcess command
	m2, _ := scanFixture(t)
	m2.cfg = config.Default()
	m2.cfg.Editors.GUI = nil
	m2, cmd2 := updateC(t, m2, keyEnter())
	if cmd2 == nil {
		t.Fatal("terminal open must return an ExecProcess command")
	}
}

func TestOpenErrorIsStickyToast(t *testing.T) {
	m, f := scanFixture(t)
	m.cfg = config.Default()
	f.OpenErr = registry.ErrNotFound
	m, cmd := updateC(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("failed open must return a toast command")
	}
	msg := cmd()
	tm, ok := msg.(toastMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want toastMsg", msg)
	}
	if tm.t.Level != "bad" || !strings.Contains(tm.t.Text, "could not open a") {
		t.Errorf("toast = %+v, want sticky error naming the project", tm.t)
	}
}

func TestEditorDoneRefreshesProjects(t *testing.T) {
	m, f := scanFixture(t)
	m, cmd := updateC(t, m, editorDoneMsg{slug: "a"})
	if cmd == nil {
		t.Fatal("returning from an editor must refresh the list")
	}
	pm, ok := cmd().(projectsMsg)
	if !ok {
		t.Fatalf("refresh returned %T, want projectsMsg", pm)
	}
	if pm.err != nil || len(pm.ps) != len(f.Projects) {
		t.Fatalf("refresh = %d projects, err %v; want %d", len(pm.ps), pm.err, len(f.Projects))
	}

	m, cmd = updateC(t, m, editorDoneMsg{slug: "a", err: registry.ErrNotFound})
	msg := cmd()
	tm, ok := msg.(toastMsg)
	if !ok || tm.t.Level != "bad" || !strings.Contains(tm.t.Text, "editor for a failed") {
		t.Fatalf("msg = %#v, want sticky editor error", msg)
	}
}

// fCalls reads the fake behind the model via a fresh scanCmd-style probe:
// the fixture keeps the fake in svc.
func fCalls(m Model) []string {
	if f, ok := m.svc.(*fake.Service); ok {
		return f.Calls()
	}
	return nil
}
