package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/style"
)

// Screen identifies the active route (spec 2.10).
type Screen int

// The screen enum, in spec order.
const (
	ScreenDashboard Screen = iota
	ScreenMasterDashboard
	ScreenProjectList
	ScreenDetail
	ScreenSource
	ScreenSettings
	ScreenStats
	ScreenConfluence
	ScreenHealth
	ScreenMap
	ScreenSearch
	ScreenDryRun
	ScreenLogs
	ScreenHelp
)

// screenNames labels every Screen for placeholders and tests.
var screenNames = [...]string{
	"dashboard", "master dashboard", "project list", "detail", "source",
	"settings", "stats", "confluence", "health", "map", "search", "dry run",
	"logs", "help",
}

// SelectScreenMsg routes the root model to another screen.
type SelectScreenMsg struct{ Screen Screen }

// SelectScreen returns a command that switches screens.
func SelectScreen(s Screen) tea.Cmd {
	return func() tea.Msg { return SelectScreenMsg{Screen: s} }
}

// Toast is a transient status line shown below the active screen.
type Toast struct {
	// Level is "good", "warn", "bad" or "" for neutral.
	Level string
	Text  string
}

type (
	projectsMsg struct {
		ps  []registry.Project
		err error
	}
	currentMsg struct {
		p  registry.Project
		ok bool
	}
	confirmMsg  struct{ c confirm }
	toastMsg    struct{ t Toast }
	popToastMsg struct{}
)

// confirm is one entry in the modal stack: a yes/no question whose
// default answer is No (esc, enter, n and q all decline).
type confirm struct {
	Title string
	OnYes func() tea.Cmd
}

// Root is the Bubble Tea root model (spec 2.10): the router plus the
// shared state every screen reads — service, config, theme, size,
// Current, toasts and the modal stack.
type Root struct {
	svc    service.Service
	cfg    config.Config
	theme  style.Theme
	width  int
	height int
	screen Screen

	dashboard    Model
	current      registry.Project
	hasCurrent   bool
	toasts       []Toast
	confirms     []confirm
	styleTitle   lipgloss.Style
	styleConfirm lipgloss.Style
	styleMuted   lipgloss.Style
	styleToast   map[string]lipgloss.Style
}

// NewRoot wires the shared state. The theme comes from config
// appearance.theme, falling back to graphite-violet with a visible
// warning when the name is unknown.
func NewRoot(svc service.Service, cfg config.Config) Root {
	theme, err := style.ByName(cfg.Appearance.Theme)
	if err != nil {
		theme = style.GraphiteViolet
	}
	r := Root{
		svc:        svc,
		cfg:        cfg,
		theme:      theme,
		dashboard:  New(nil, cfg.Workspace.Root),
		styleTitle: lipgloss.NewStyle().Bold(true).Foreground(theme.Accent),
		styleConfirm: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Border).
			Foreground(theme.Text).
			Background(theme.Panel).
			Padding(0, style.Pad),
		styleMuted: lipgloss.NewStyle().Foreground(theme.Muted),
		styleToast: map[string]lipgloss.Style{
			"":     lipgloss.NewStyle().Foreground(theme.Muted),
			"good": lipgloss.NewStyle().Foreground(theme.Good),
			"warn": lipgloss.NewStyle().Foreground(theme.Warn),
			"bad":  lipgloss.NewStyle().Foreground(theme.Bad),
		},
	}
	if err != nil {
		r.toasts = append(r.toasts, Toast{
			Level: "warn",
			Text:  fmt.Sprintf("unknown theme %q, using graphite-violet", cfg.Appearance.Theme),
		})
	}
	return r
}

// Run starts the terminal interface over the shared service (spec
// 1.4.4: the TUI uses the same service layer as the CLI).
func Run(svc service.Service, cfg config.Config) error {
	_, err := tea.NewProgram(NewRoot(svc, cfg), tea.WithAltScreen()).Run()
	return err
}

// Init loads Current and the project list asynchronously.
func (r Root) Init() tea.Cmd {
	return tea.Batch(r.loadProjects(), r.loadCurrent())
}

func (r Root) loadProjects() tea.Cmd {
	return func() tea.Msg {
		ps, err := r.svc.List(service.Filter{})
		return projectsMsg{ps: ps, err: err}
	}
}

func (r Root) loadCurrent() tea.Cmd {
	return func() tea.Msg {
		p, ok := r.svc.Current()
		return currentMsg{p: p, ok: ok}
	}
}

// Update keeps shared state current, routes to the modal stack first
// and otherwise forwards to the active screen.
func (r Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = x.Width, x.Height
		return r.forward(x)
	case SelectScreenMsg:
		r.screen = x.Screen
		return r, nil
	case projectsMsg:
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "could not list projects: " + x.err.Error()})
		}
		r.dashboard.Projects = x.ps
		r.dashboard.applyFilter()
		return r, nil
	case currentMsg:
		r.current, r.hasCurrent = x.p, x.ok
		return r, nil
	case confirmMsg:
		r.confirms = append(r.confirms, x.c)
		return r, nil
	case toastMsg:
		r.toasts = append(r.toasts, x.t)
		return r, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return popToastMsg{} })
	case popToastMsg:
		if len(r.toasts) > 0 {
			r.toasts = r.toasts[1:]
		}
		return r, nil
	case tea.KeyMsg:
		if n := len(r.confirms); n > 0 {
			return r.updateConfirm(x, r.confirms[n-1])
		}
		return r.forward(x)
	default:
		return r.forward(x)
	}
}

// forward sends msg to the active screen's sub-model.
func (r Root) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	if r.screen != ScreenDashboard {
		return r, nil
	}
	nm, cmd := r.dashboard.Update(msg)
	r.dashboard = nm.(Model)
	return r, cmd
}

// updateConfirm handles the top modal; only y accepts, everything else
// closes it without acting (default No).
func (r Root) updateConfirm(msg tea.KeyMsg, c confirm) (tea.Model, tea.Cmd) {
	pop := func() { r.confirms = r.confirms[:len(r.confirms)-1] }
	switch msg.String() {
	case "y", "Y":
		pop()
		if c.OnYes != nil {
			return r, c.OnYes()
		}
		return r, nil
	case "n", "N", "esc", "enter", "q", "ctrl+c":
		pop()
		return r, nil
	}
	return r, nil // swallow every other key while a modal is open
}

func (r Root) pushToast(t Toast) (tea.Model, tea.Cmd) {
	r.toasts = append(r.toasts, t)
	return r, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return popToastMsg{} })
}

// Confirm queues a yes/no modal over the current screen.
func Confirm(title string, onYes func() tea.Cmd) tea.Cmd {
	return func() tea.Msg { return confirmMsg{c: confirm{Title: title, OnYes: onYes}} }
}

// ShowToast queues a transient status line that clears itself.
func ShowToast(t Toast) tea.Cmd {
	return func() tea.Msg { return toastMsg{t: t} }
}

// View renders the active screen, the toast lines and, when present,
// the top modal centred over everything (spec: esc cancels it).
func (r Root) View() string {
	var body string
	switch r.screen {
	case ScreenDashboard:
		body = r.dashboard.View()
	default:
		name := screenNames[r.screen]
		body = r.styleTitle.Render(strings.ToUpper(name))
	}
	for _, t := range r.toasts {
		body += "\n" + r.toastView(t)
	}
	if n := len(r.confirms); n > 0 {
		box := r.confirmView(r.confirms[n-1])
		if r.width > 0 && r.height > 0 {
			return lipgloss.Place(r.width, r.height, lipgloss.Center, lipgloss.Center, box)
		}
		return box
	}
	return body
}

func (r Root) toastView(t Toast) string {
	st, ok := r.styleToast[t.Level]
	if !ok {
		st = r.styleToast[""]
	}
	return st.Render(" " + t.Text)
}

func (r Root) confirmView(c confirm) string {
	body := r.styleTitle.Render(c.Title) + "\n" +
		r.styleMuted.Render("y yes   n/esc/enter no (default)")
	return r.styleConfirm.Render(body)
}
