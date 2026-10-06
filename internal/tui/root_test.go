package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/service/fake"
	"github.com/manojpisini/rivu/internal/style"
)

func rootOf(t *testing.T) (Root, *fake.Service) {
	t.Helper()
	svc := &fake.Service{}
	return NewRoot(svc, config.Default()), svc
}

func upd(t *testing.T, r Root, msg tea.Msg) (Root, tea.Cmd) {
	t.Helper()
	m, cmd := r.Update(msg)
	return m.(Root), cmd
}

func keyR(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }
func keyEsc() tea.KeyMsg     { return tea.KeyMsg{Type: tea.KeyEsc} }
func keyEnter() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyEnter} }

// errListSvc fails List so the projectsMsg error path is reachable.
type errListSvc struct{ *fake.Service }

func (errListSvc) List(service.Filter) ([]registry.Project, error) {
	return nil, errors.New("boom")
}

func TestRouterSwitchesScreen(t *testing.T) {
	r, _ := rootOf(t)
	if r.screen != ScreenDashboard {
		t.Fatalf("default screen = %d, want dashboard", r.screen)
	}
	r, _ = upd(t, r, SelectScreen(ScreenStats)())
	if r.screen != ScreenStats {
		t.Fatalf("screen = %d, want stats", r.screen)
	}
	view := r.View()
	if !strings.Contains(view, "STATS") {
		t.Fatalf("view for unimplemented screen should label it, got %q", view)
	}
	r, _ = upd(t, r, SelectScreen(ScreenDashboard)())
	if r.screen != ScreenDashboard {
		t.Fatal("did not route back to dashboard")
	}
}

func TestProjectsMsgPopulatesDashboard(t *testing.T) {
	r, svc := rootOf(t)
	svc.Projects = []registry.Project{{ID: "1", Name: "alpha"}}
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	msg := r.loadProjects()()
	r, cmd := upd(t, r, msg)
	if cmd != nil {
		t.Fatal("projects load should not chain commands")
	}
	if len(r.dashboard.Projects) != 1 || r.dashboard.Projects[0].Name != "alpha" {
		t.Fatalf("dashboard projects = %+v", r.dashboard.Projects)
	}
	if !strings.Contains(r.View(), "alpha") {
		t.Fatalf("view should show the project, got %q", r.View())
	}
}

func TestProjectsMsgErrorToasts(t *testing.T) {
	r := NewRoot(errListSvc{&fake.Service{}}, config.Default())
	r, _ = upd(t, r, r.loadProjects()())
	if len(r.toasts) != 1 || r.toasts[0].Level != "bad" {
		t.Fatalf("toasts = %+v, want one bad toast", r.toasts)
	}
	if !strings.Contains(r.View(), "could not list projects") {
		t.Fatalf("view should explain the failure, got %q", r.View())
	}
}

func TestCurrentMsgStoresCurrent(t *testing.T) {
	r, svc := rootOf(t)
	svc.CurrentP = registry.Project{ID: "9", Name: "current-one"}
	svc.HasCurrent = true
	r, _ = upd(t, r, r.loadCurrent()())
	if !r.hasCurrent || r.current.Name != "current-one" {
		t.Fatalf("current = %+v has=%v", r.current, r.hasCurrent)
	}
}

func TestToastsShowAndExpire(t *testing.T) {
	r, _ := rootOf(t)
	r, cmd := upd(t, r, ShowToast(Toast{Level: "good", Text: "sourced demo"})())
	if cmd == nil {
		t.Fatal("toast should schedule its expiry tick")
	}
	if !strings.Contains(r.View(), "sourced demo") {
		t.Fatalf("view should show the toast, got %q", r.View())
	}
	r, _ = upd(t, r, popToastMsg{})
	if len(r.toasts) != 0 {
		t.Fatalf("toasts = %+v, want empty after expiry", r.toasts)
	}
}

func TestConfirmModalYesAndDefaultNo(t *testing.T) {
	r, _ := rootOf(t)
	yes := 0
	r, _ = upd(t, r, Confirm("Move demo?", func() tea.Cmd {
		yes++
		return nil
	})())
	if len(r.confirms) != 1 {
		t.Fatalf("confirms = %d, want 1", len(r.confirms))
	}
	if !strings.Contains(r.View(), "Move demo?") {
		t.Fatalf("modal should render its title, got %q", r.View())
	}
	r, _ = upd(t, r, keyEsc())
	if yes != 0 || len(r.confirms) != 0 {
		t.Fatalf("esc must decline and close (default No): yes=%d stack=%d", yes, len(r.confirms))
	}
	r, _ = upd(t, r, Confirm("Move demo?", func() tea.Cmd { yes++; return nil })())
	r, _ = upd(t, r, keyEnter())
	if yes != 0 || len(r.confirms) != 0 {
		t.Fatalf("enter must decline and close (default No): yes=%d stack=%d", yes, len(r.confirms))
	}
	r, _ = upd(t, r, Confirm("Move demo?", func() tea.Cmd { yes++; return nil })())
	r, _ = upd(t, r, keyR('y'))
	if yes != 1 || len(r.confirms) != 0 {
		t.Fatalf("y must accept and close: yes=%d stack=%d", yes, len(r.confirms))
	}
}

func TestConfirmSwallowsKeys(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, Confirm("Sure?", nil)())
	for _, k := range []tea.KeyMsg{keyR('q'), keyEsc()} {
		m, cmd := r.Update(k)
		if cmd != nil {
			if _, isQuit := cmd().(tea.QuitMsg); isQuit {
				t.Fatalf("key %v must not quit while a modal is open", k)
			}
		}
		r = m.(Root)
	}
	if len(r.confirms) != 0 {
		t.Fatalf("modal should have closed on esc, stack=%d", len(r.confirms))
	}
}

func TestNewRootThemeFromConfigAndFallback(t *testing.T) {
	svc := &fake.Service{}
	cfg := config.Default()
	cfg.Appearance.Theme = "mono"
	r := NewRoot(svc, cfg)
	if r.theme != style.Mono {
		t.Fatalf("theme = %+v, want mono", r.theme)
	}
	cfg.Appearance.Theme = "nope"
	r = NewRoot(svc, cfg)
	if r.theme != style.GraphiteViolet {
		t.Fatal("unknown theme must fall back to graphite-violet")
	}
	if len(r.toasts) != 1 || r.toasts[0].Level != "warn" {
		t.Fatalf("toasts = %+v, want a warning about the theme", r.toasts)
	}
}

func TestWindowSizeStoresAndForwards(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 120, Height: 40})
	if r.width != 120 || r.height != 40 {
		t.Fatalf("size = %dx%d", r.width, r.height)
	}
	if r.dashboard.help.Width != 120 {
		t.Fatalf("dashboard help width = %d, want forwarded 120", r.dashboard.help.Width)
	}
}
