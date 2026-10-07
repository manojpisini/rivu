package tui

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/doctor"
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
// default answer is No (esc, enter, n and q all decline). Lines carries
// the Plan body the modal renders; up/down scroll it.
type confirm struct {
	Title  string
	Lines  []string
	OnYes  func() tea.Cmd
	scroll int
}

// confirmVisible is how many Plan lines fit in the modal viewport.
const confirmVisible = 12

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
	src          sourceWizard
	pendingJump  string // slug to select once the post-apply list lands (P4.06)
	current      registry.Project
	hasCurrent   bool
	toasts       []Toast
	confirms     []confirm
	errs         []string
	errExpand    bool
	doctorRes    []doctor.Report
	doctorQ      string
	styleTitle   lipgloss.Style
	styleConfirm lipgloss.Style
	styleMuted   lipgloss.Style
	styleErr     lipgloss.Style
	styleErrBox  lipgloss.Style
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
		styleErr:   lipgloss.NewStyle().Foreground(theme.Bad).Bold(true),
		styleErrBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Bad).
			Foreground(theme.Text).
			Background(theme.Panel).
			Padding(0, style.Pad),
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
	r.dashboard.svc = svc
	r.dashboard.cfg = cfg
	// One stat at startup (P3.24); View never touches the filesystem.
	if _, err := os.Stat(cfg.Workspace.Root); err != nil {
		r.dashboard.RootExists = false
	}
	return r
}

// Run starts the terminal interface over the shared service (spec
// 1.4.4: the TUI uses the same service layer as the CLI).
//
// Bubble Tea's default panic handling restores the terminal first, then
// re-panics; we catch that here, write the crash report (stack included)
// to the rotating JSON log and return a normal error so the CLI exits 1
// with a pointer to the log instead of dying raw (P3.26).
func Run(svc service.Service, cfg config.Config) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			logCrash(rec, &err)
		}
	}()
	_, err = tea.NewProgram(NewRoot(svc, cfg), tea.WithAltScreen()).Run()
	return err
}

// logCrash records a recovered TUI panic to the default slog logger and
// replaces the returned error; kept separate so tests can drive it.
func logCrash(rec any, errp *error) {
	slog.Error("tui panic", "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
	*errp = fmt.Errorf("tui crashed: %v (stack trace in the Rivu log)", rec)
}

// windowTitle is "Rivu — <Current>" (AGENTS 8), plain "Rivu" until a
// Current exists (P3.28).
func windowTitle(name string, has bool) string {
	if !has || name == "" {
		return "Rivu"
	}
	return "Rivu — " + name
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
		nr, cmd := r.forward(x)
		return nr, tea.Batch(cmd, tea.SetWindowTitle(windowTitle(r.dashboard.Current.Name, r.hasCurrent)))
	case SelectScreenMsg:
		r.screen = x.Screen
		if x.Screen == ScreenSource {
			// Every entry starts a fresh wizard (P4.01).
			r.src = newSourceWizard()
			r.dashboard.Status = ""
		}
		return r, nil
	case projectsMsg:
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "could not list projects: " + x.err.Error()})
		}
		r.dashboard.Projects = x.ps
		r.dashboard.applyFilter()
		if r.pendingJump != "" {
			r.dashboard.selectSlug(r.pendingJump)
			r.pendingJump = ""
		}
		return r, nil
	case currentMsg:
		r.current, r.hasCurrent = x.p, x.ok
		r.dashboard.Current, r.dashboard.HasCurrent = x.p, x.ok
		return r, tea.SetWindowTitle(windowTitle(x.p.Name, x.ok))
	case scanDoneMsg:
		// Intercepted here so a scan finishing while another screen is
		// open is not dropped by forward().
		r.dashboard, _ = r.dashboard.applyScanDone(x)
		return r, nil
	case editorDoneMsg:
		// A GUI editor can finish while another screen is active.
		nm, cmd := r.dashboard.Update(x)
		r.dashboard = nm.(Model)
		return r, cmd
	case mapDoneMsg:
		nm, cmd := r.dashboard.Update(x)
		r.dashboard = nm.(Model)
		return r, cmd
	case flowPlanMsg:
		nm, cmd := r.dashboard.Update(x)
		r.dashboard = nm.(Model)
		return r, cmd
	case flowApplyMsg:
		nm, cmd := r.dashboard.Update(x)
		r.dashboard = nm.(Model)
		return r, cmd
	case copyDoneMsg:
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "copy path failed: " + x.err.Error()})
		}
		return r.pushToast(Toast{Level: "good", Text: "path copied to the clipboard (OSC52)"})
	case revealDoneMsg:
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "reveal failed: " + x.err.Error()})
		}
		return r.pushToast(Toast{Level: "good", Text: "folder revealed: " + x.path})
	case sourceDryMsg:
		// stale when the wizard was cancelled while the preview ran
		if r.screen != ScreenSource || !r.src.loading {
			return r, nil
		}
		r.src.loading = false
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "could not build the plan: " + x.err.Error()})
		}
		p := x.res.Plan
		r.src.plan = &p
		return r, nil
	case sourceApplyMsg:
		// The apply already ran — its outcome is reported even if the
		// wizard was left mid-flight (never silently drop a write).
		r.src = sourceWizard{}
		if x.err != nil {
			r.screen = ScreenDashboard
			return r.pushToast(Toast{Level: "bad", Text: "could not source the project: " + x.err.Error()})
		}
		r.screen = ScreenDashboard
		r.dashboard.Status = ""
		r.pendingJump = x.res.Project.Slug
		toast := ShowToast(Toast{Level: "good", Text: fmt.Sprintf("Sourced %s at %s", x.res.Project.Name, x.res.Project.Path)})
		return r, tea.Batch(r.loadProjects(), toast)
	case doctorDoneMsg:
		r.dashboard.Status = ""
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "health checks failed: " + x.err.Error()})
		}
		r.doctorRes, r.doctorQ = x.reports, x.q
		r.screen = ScreenHealth
		return r, nil
	case confirmMsg:
		r.confirms = append(r.confirms, x.c)
		return r, nil
	case toastMsg:
		return r.pushToast(x.t)
	case popToastMsg:
		if len(r.toasts) > 0 {
			r.toasts = r.toasts[1:]
		}
		return r, nil
	case tea.KeyMsg:
		if n := len(r.confirms); n > 0 {
			return r.updateConfirm(x, r.confirms[n-1])
		}
		if r.errExpand {
			switch x.String() {
			case "e", "esc":
				r.errExpand = false
			case "x":
				r.errs = nil
				r.errExpand = false
			case "ctrl+c":
				return r, tea.Quit
			}
			return r, nil // the overlay swallows every other key
		}
		// "e" expands sticky errors, but never steals a typed search.
		if x.String() == "e" && len(r.errs) > 0 && !r.dashboard.search.Focused() {
			r.errExpand = true
			return r, nil
		}
		if r.screen == ScreenSource {
			return r.updateSource(x)
		}
		if r.screen != ScreenDashboard {
			// Sub-screens: esc/q go back, ctrl+c quits, everything else
			// belongs to the screen once it grows its own keymap.
			switch x.String() {
			case "esc", "q":
				r.screen = ScreenDashboard
				return r, nil
			case "ctrl+c":
				return r, tea.Quit
			}
			return r, nil
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
// closes it without acting (default No). up/down scroll the Plan body.
func (r Root) updateConfirm(msg tea.KeyMsg, c confirm) (tea.Model, tea.Cmd) {
	n := len(r.confirms) - 1
	pop := func() { r.confirms = r.confirms[:n] }
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
	case "up":
		if r.confirms[n].scroll > 0 {
			r.confirms[n].scroll--
		}
		return r, nil
	case "down":
		maxScroll := max(0, len(r.confirms[n].Lines)-confirmVisible)
		if r.confirms[n].scroll < maxScroll {
			r.confirms[n].scroll++
		}
		return r, nil
	}
	return r, nil // swallow every other key while a modal is open
}

// pushToast queues a transient line: errors are sticky (kept until the
// user clears them with `x` in the `e` overlay — never lost behind a
// redraw), everything else expires after 3 s.
func (r Root) pushToast(t Toast) (tea.Model, tea.Cmd) {
	if t.Level == "bad" {
		r.errs = append(r.errs, t.Text)
		// ponytail: cap at 10 — raise only if errors start vanishing in practice
		if len(r.errs) > 10 {
			r.errs = r.errs[len(r.errs)-10:]
		}
		return r, nil
	}
	r.toasts = append(r.toasts, t)
	return r, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return popToastMsg{} })
}

// Confirm queues a yes/no modal over the current screen.
func Confirm(title string, onYes func() tea.Cmd) tea.Cmd {
	return func() tea.Msg { return confirmMsg{c: confirm{Title: title, OnYes: onYes}} }
}

// ConfirmPlan queues the same modal with a Plan body (spec 3.5): every
// line is shown, scrollable with up/down when taller than the viewport,
// default No.
func ConfirmPlan(title string, lines []string, onYes func() tea.Cmd) tea.Cmd {
	return func() tea.Msg { return confirmMsg{c: confirm{Title: title, Lines: lines, OnYes: onYes}} }
}

// sourcePlanLines flattens a Source plan into the modal's body lines
// (the same Will create/write/run vocabulary as the CLI dry run).
func sourcePlanLines(p service.SourcePlan) []string {
	lines := []string{fmt.Sprintf("Source %s into %s", p.Name, p.Channel)}
	for _, s := range p.Create {
		lines = append(lines, "Will create "+s)
	}
	for _, s := range p.Write {
		lines = append(lines, "Will write "+s)
	}
	for _, s := range p.Run {
		lines = append(lines, "Will run "+s)
	}
	for _, s := range p.Registry {
		lines = append(lines, "Registry: "+s)
	}
	for _, s := range p.Bank {
		lines = append(lines, "Bank: "+s)
	}
	return lines
}

// flowPlanLines flattens a Flow plan the same way.
func flowPlanLines(p service.FlowPlan) []string {
	lines := []string{fmt.Sprintf("Flow %s: %s -> %s", p.Query, p.FromStage, p.ToStage)}
	for _, s := range p.Move {
		lines = append(lines, "Will move "+s)
	}
	for _, s := range p.Registry {
		lines = append(lines, "Registry: "+s)
	}
	for _, s := range p.Bank {
		lines = append(lines, "Bank: "+s)
	}
	return lines
}

// ShowToast queues a transient status line that clears itself.
func ShowToast(t Toast) tea.Cmd {
	return func() tea.Msg { return toastMsg{t: t} }
}

// View renders the active screen, the toast lines and, when present,
// the top modal centred over everything (spec: esc cancels it).
// View is the single rendered frame; ASCII mode (P3.31) swaps every
// non-ASCII glyph for a plain equivalent here, once.
func (r Root) View() string {
	v := r.render()
	if asciiGlyphs {
		return asciiSwap.Replace(v)
	}
	return v
}

func (r Root) render() string {
	var body string
	switch r.screen {
	case ScreenDashboard:
		body = r.dashboard.View()
	case ScreenSource:
		body = r.sourceView()
	case ScreenHealth:
		body = r.doctorView()
	default:
		name := screenNames[r.screen]
		body = r.styleTitle.Render(strings.ToUpper(name))
	}
	for _, t := range r.toasts {
		body += "\n" + r.toastView(t)
	}
	if len(r.errs) > 0 && !r.errExpand {
		body += "\n" + r.errBannerView()
	}
	if n := len(r.confirms); n > 0 {
		box := r.confirmView(r.confirms[n-1])
		if r.width > 0 && r.height > 0 {
			return lipgloss.Place(r.width, r.height, lipgloss.Center, lipgloss.Center, box)
		}
		return box
	}
	if r.errExpand {
		box := r.errExpandView()
		if r.width > 0 && r.height > 0 {
			return lipgloss.Place(r.width, r.height, lipgloss.Center, lipgloss.Center, box)
		}
		return box
	}
	return body
}

// errBannerView is the one-line sticky error notice; full text lives in
// the `e` overlay. Width 0 means "not measured yet" — no truncation.
func (r Root) errBannerView() string {
	first := r.errs[0]
	if r.width > 0 {
		first = shorten(first, max(1, r.width-14))
	}
	extra := ""
	if n := len(r.errs); n > 1 {
		extra = fmt.Sprintf(" (+%d more)", n-1)
	}
	return r.styleErr.Render(fmt.Sprintf(" %s%s  [e expand]", first, extra))
}

// errExpandView lists every sticky error in full, wrapped to width.
func (r Root) errExpandView() string {
	var b strings.Builder
	b.WriteString(r.styleTitle.Render(fmt.Sprintf("ERRORS (%d)", len(r.errs))))
	for _, e := range r.errs {
		text := e
		if r.width > 0 {
			text = ansi.Wrap(e, max(20, r.width-8), "")
		}
		b.WriteString("\n\n" + text)
	}
	b.WriteString("\n\n" + r.styleMuted.Render("x clear all · esc close"))
	return r.styleErrBox.Render(b.String())
}

// doctorView renders health-check reports: every project's score, then
// only the failing checks (the rest come from `rivu doctor`).
func (r Root) doctorView() string {
	var b strings.Builder
	title := "HEALTH — all projects"
	if r.doctorQ != "" {
		title = "HEALTH — " + r.doctorQ
	}
	b.WriteString(r.styleTitle.Render(title))
	if len(r.doctorRes) == 0 {
		b.WriteString("\n\n" + r.styleMuted.Render("No projects to check yet — run `rivu source <path>` or press r to rescan."))
		b.WriteString("\n" + r.styleMuted.Render("esc back"))
		return b.String()
	}
	for _, rep := range r.doctorRes {
		b.WriteString("\n\n" + rep.Project.Name + "  " + healthBadge(rep.Score))
		failed := false
		for _, c := range rep.Checks {
			if c.OK {
				continue
			}
			failed = true
			b.WriteString("\n" + r.styleErr.Render(" ! "+c.Name) + "  " + c.Detail)
		}
		if !failed {
			b.WriteString("\n" + r.styleMuted.Render(" all checks pass"))
		}
	}
	b.WriteString("\n\n" + r.styleMuted.Render("esc back · rivu doctor for full output"))
	return b.String()
}

func (r Root) toastView(t Toast) string {
	st, ok := r.styleToast[t.Level]
	if !ok {
		st = r.styleToast[""]
	}
	return st.Render(" " + t.Text)
}

func (r Root) confirmView(c confirm) string {
	var b strings.Builder
	b.WriteString(r.styleTitle.Render(c.Title))
	if len(c.Lines) > 0 {
		start := min(c.scroll, max(0, len(c.Lines)-confirmVisible))
		end := min(len(c.Lines), start+confirmVisible)
		for _, ln := range c.Lines[start:end] {
			if r.width > 0 {
				ln = shorten(ln, max(1, r.width-6))
			}
			b.WriteString("\n" + ln)
		}
		if len(c.Lines) > confirmVisible {
			b.WriteString("\n" + r.styleMuted.Render(fmt.Sprintf("%d-%d of %d", start+1, end, len(c.Lines))))
		}
	}
	hint := "y yes   n/esc/enter no (default)"
	if len(c.Lines) > confirmVisible {
		hint += "   ↑↓ scroll"
	}
	b.WriteString("\n" + r.styleMuted.Render(hint))
	return r.styleConfirm.Render(b.String())
}
