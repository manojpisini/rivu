package tui

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strconv"
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
// the Plan body the modal renders; up/down scroll it. Require > 0
// switches to confirm-by-typing: the user must type that exact count
// (spec Part A: bulk flow of > 5 projects).
type confirm struct {
	Title   string
	Lines   []string
	OnYes   func() tea.Cmd
	Require int
	typed   string
	scroll  int
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
	mapPrev      service.MapPreview // Map report payload (P4.15)
	mapQ         string
	mapScroll    int
	masterRes    *service.Dashboard // Master Dashboard snapshot (P4.16)
	masterCursor int                // attention queue cursor (P4.18)
	statsRes     *service.Stats     // Stats screen snapshot (P4.19)
	statsDaily   []int              // 30-day activity counts
	statsCompact bool               // hide language/flow sections (P4.20)
	statsExport  bool               // export format picker open (P4.20)
	statsExportC int                // 0 = csv, 1 = json
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
			r.src = newSourceWizard(r.cfg)
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
		if r.screen == ScreenMap {
			// Rebuild from inside the report: toast like the dashboard
			// path, then refresh the preview and HasMap flags together.
			r.dashboard.Status = ""
			if x.err != nil {
				return r.pushToast(Toast{Level: "bad", Text: "agent map failed for " + x.q + ": " + x.err.Error()})
			}
			return r, tea.Batch(
				ShowToast(Toast{Level: "good", Text: "agent map built for " + x.q}),
				mapPreviewCmd(r.svc, x.q),
				r.loadProjects(),
			)
		}
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
	case bulkFlowPlanMsg:
		nm, cmd := r.dashboard.Update(x)
		r.dashboard = nm.(Model)
		return r, cmd
	case bulkFlowApplyMsg:
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
		w := r.src
		r.src = sourceWizard{}
		if x.err != nil {
			r.screen = ScreenDashboard
			return r.pushToast(Toast{Level: "bad", Text: "could not source the project: " + x.err.Error()})
		}
		r.screen = ScreenDashboard
		r.dashboard.Status = ""
		r.pendingJump = x.res.Project.Slug
		toast := ShowToast(Toast{Level: "good", Text: fmt.Sprintf("Sourced %s at %s", x.res.Project.Name, x.res.Project.Path)})
		cmds := []tea.Cmd{r.loadProjects(), toast}
		if w.openEditor {
			// Automation "Open editor after" (P4.23): the wizard's Editor
			// row feeds the launch; a bad editor string warns without
			// undoing the successful source.
			oc, err := openCmd(r.svc, x.res.Project.Slug, w.editor, r.cfg.Editors.GUI)
			if err != nil {
				cmds = append(cmds, ShowToast(Toast{Level: "warn", Text: "could not open " + x.res.Project.Slug + ": " + err.Error()}))
			} else {
				cmds = append(cmds, oc)
			}
		}
		return r, tea.Batch(cmds...)
	case doctorDoneMsg:
		r.dashboard.Status = ""
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "health checks failed: " + x.err.Error()})
		}
		r.doctorRes, r.doctorQ = x.reports, x.q
		r.screen = ScreenHealth
		return r, nil
	case mapPreviewMsg:
		r.dashboard.Status = ""
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "could not open the map report: " + x.err.Error()})
		}
		r.mapPrev, r.mapQ = x.prev, x.q
		r.mapScroll = 0
		r.screen = ScreenMap
		return r, nil
	case dashMsg:
		r.dashboard.Status = ""
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "could not load the master dashboard: " + x.err.Error()})
		}
		r.masterRes = &x.d
		r.masterCursor = 0
		r.screen = ScreenMasterDashboard
		return r, nil
	case statsMsg:
		r.dashboard.Status = ""
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "could not load stats: " + x.err.Error()})
		}
		r.statsRes = &x.st
		r.statsDaily = x.daily
		r.screen = ScreenStats
		return r, nil
	case exportDoneMsg:
		if x.err != nil {
			return r.pushToast(Toast{Level: "bad", Text: "export failed: " + x.err.Error()})
		}
		return r.pushToast(Toast{Level: "", Text: "exported " + x.name})
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
		// Errors win over the Stats screen's export key so the banner's
		// [e expand] hint is never a lie; clear them, then e exports.
		if x.String() == "e" && len(r.errs) > 0 && !r.dashboard.search.Focused() {
			r.errExpand = true
			return r, nil
		}
		if r.screen == ScreenStats {
			return r.statsKeys(x)
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
			if r.screen == ScreenMap {
				return r.mapKeys(x)
			}
			if r.screen == ScreenMasterDashboard {
				return r.masterKeys(x)
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
// A Require modal only accepts the typed count.
func (r Root) updateConfirm(msg tea.KeyMsg, c confirm) (tea.Model, tea.Cmd) {
	n := len(r.confirms) - 1
	pop := func() { r.confirms = r.confirms[:n] }
	if c.Require > 0 {
		target := strconv.Itoa(c.Require)
		switch msg.String() {
		case "esc", "enter", "n", "q", "ctrl+c":
			pop()
			return r, nil
		case "backspace":
			if c.typed != "" {
				r.confirms[n].typed = c.typed[:len(c.typed)-1]
			}
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
		if s := msg.String(); len(s) == 1 && s[0] >= '0' && s[0] <= '9' {
			r.confirms[n].typed += s
			if !strings.HasPrefix(target, r.confirms[n].typed) {
				r.confirms[n].typed = s // wrong digit starts over
			}
			if r.confirms[n].typed == target {
				pop()
				if c.OnYes != nil {
					return r, c.OnYes()
				}
			}
		}
		return r, nil // swallow every other key: y alone never applies
	}
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

// ConfirmPlanTyped is the confirm-by-typing variant: the modal only
// accepts when the user types exactly require (the count of affected
// projects); y alone never applies.
func ConfirmPlanTyped(title string, lines []string, require int, onYes func() tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		return confirmMsg{c: confirm{Title: title, Lines: lines, OnYes: onYes, Require: require}}
	}
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
	case ScreenMap:
		body = r.mapReportView()
	case ScreenMasterDashboard:
		body = r.masterView()
	case ScreenStats:
		body = r.statsView()
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

// doctorRemedy gives a failing check a concrete next step in the
// Health screen (P4.13); "" means the check needs no action.
func doctorRemedy(c doctor.Check, slug string) string {
	switch c.Name {
	case "README":
		return "create README.md in the project folder"
	case "Git":
		return "run `git init` in the project folder"
	case "Bank":
		return "run `rivu doctor " + slug + "` to audit"
	case "Map":
		return "run `rivu agent sync " + slug + "`"
	case "Tests":
		return "add a tests/ folder"
	case "CI":
		return "add a workflow under .github/workflows"
	case "License":
		return "add a LICENSE file"
	case "Git exclusivity":
		return "verify history (.git + bridge marker + git_init_owner=rivu)"
	}
	return ""
}

// doctorView renders health-check reports (spec P4.13): score bar,
// every check as a finding (CLI parity with `rivu doctor`), and a fix
// line under each failing check.
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
		b.WriteString("\n\n" + rep.Project.Name + "  " + healthBadge(rep.Score) + "  " + scoreBar(rep.Score) + "  " + strconv.Itoa(rep.Score) + "/100")
		if len(rep.Trend) >= 2 {
			b.WriteString("  " + r.styleMuted.Render("trend ") + sparkline(rep.Trend))
		}
		for _, c := range rep.Checks {
			line := r.styleMuted.Render(" ✓ ") + c.Name + "  " + c.Detail
			if !c.OK {
				line = r.styleErr.Render(" ! ") + c.Name + "  " + c.Detail
			}
			if r.width > 0 {
				line = shorten(line, max(1, r.width-2))
			}
			b.WriteString("\n" + line)
			if !c.OK {
				if fix := doctorRemedy(c, rep.Project.Slug); fix != "" {
					b.WriteString("\n" + r.styleMuted.Render("   fix: "+fix))
				}
			}
		}
	}
	b.WriteString("\n\n" + r.styleMuted.Render("esc back · rivu doctor for full output"))
	return b.String()
}

// mapKeys gives the Map report its own keys (P4.15): scroll the
// preview, "a" rebuilds the map and reloads the report.
func (r Root) mapKeys(x tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch x.String() {
	case "a":
		if r.svc == nil {
			return r.pushToast(Toast{Level: "bad", Text: "Agent map unavailable: no service in this session"})
		}
		return r, tea.Batch(mapCmd(r.svc, r.mapQ), mapPreviewCmd(r.svc, r.mapQ))
	case "up", "k":
		r.mapScroll = max(0, r.mapScroll-1)
	case "down", "j":
		r.mapScroll++
	case "pgup":
		r.mapScroll = max(0, r.mapScroll-10)
	case "pgdown":
		r.mapScroll += 10
	case "g":
		r.mapScroll = 0
	case "G":
		r.mapScroll = 1 << 30 // clamped against the content at render
	}
	return r, nil
}

// masterKeys drives the attention queue (P4.18): up/down pick a bucket,
// enter opens that project's Detail with the fix pre-highlighted.
func (r Root) masterKeys(x tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := 0
	if r.masterRes != nil {
		n = len(r.masterRes.Attention)
	}
	switch x.String() {
	case "up", "k":
		r.masterCursor = max(0, r.masterCursor-1)
	case "down", "j":
		r.masterCursor = min(max(n-1, 0), r.masterCursor+1)
	case "enter":
		if n == 0 {
			return r, nil
		}
		at := r.masterRes.Attention[min(r.masterCursor, n-1)]
		if at.Sample == "" {
			return r.pushToast(Toast{Level: "", Text: "no project in that bucket yet"})
		}
		// Drop the filters so the sample is always visible, then jump
		// into its Detail with the fix line lit (spec 3.3).
		r.dashboard.Query = ""
		r.dashboard.Untriaged = false
		r.dashboard.FlowCursor = 0
		r.dashboard.applyFilter()
		r.dashboard.selectSlug(at.Sample)
		r.dashboard.FixKey = at.Key
		r.dashboard.Status = ""
		r.screen = ScreenDashboard
		return r, nil
	}
	return r, nil
}

// statsKeys drives the Stats screen (P4.20): t toggles the language
// and flow breakdowns, e opens the csv/json export picker (spec 3.6).
func (r Root) statsKeys(x tea.KeyMsg) (tea.Model, tea.Cmd) {
	if r.statsExport {
		switch x.String() {
		case "esc", "q":
			r.statsExport = false
		case "enter":
			r.statsExport = false
			if r.statsRes == nil {
				return r.pushToast(Toast{Level: "bad", Text: "nothing to export yet"})
			}
			format := "csv"
			if r.statsExportC == 1 {
				format = "json"
			}
			return r, exportCmd(format, *r.statsRes)
		case "left", "right", "h", "l", "tab", "shift+tab":
			r.statsExportC = 1 - r.statsExportC
		case "ctrl+c":
			return r, tea.Quit
		}
		return r, nil // the picker swallows every other key
	}
	switch x.String() {
	case "esc", "q":
		r.screen = ScreenDashboard
	case "ctrl+c":
		return r, tea.Quit
	case "t":
		r.statsCompact = !r.statsCompact
	case "e":
		r.statsExport = true
		r.statsExportC = 0
	}
	return r, nil
}

// mapLines is the scrollable body of the Map report: both on-disk
// files plus the diff a rebuild would apply (P4.15).
func (r Root) mapLines() []string {
	var lines []string
	lines = append(lines, "PROJECT_MAP.md")
	if r.mapPrev.MapBody == "" {
		lines = append(lines, "(missing - press a to rebuild)")
	} else {
		lines = append(lines, fileLines(r.mapPrev.MapBody)...)
	}
	if r.mapPrev.MapStale && len(r.mapPrev.Diff) > 0 {
		lines = append(lines, "", "DIFF vs disk (a rebuilds):")
		lines = append(lines, r.mapPrev.Diff...)
	}
	lines = append(lines, "", "AGENTS.md (create-if-missing, never overwritten)")
	if r.mapPrev.AgentsBody == "" {
		lines = append(lines, "(missing - press a to create)")
	} else {
		lines = append(lines, fileLines(r.mapPrev.AgentsBody)...)
	}
	return lines
}

// mapReportView is the Map report screen (spec screen 8): a preview of
// PROJECT_MAP.md/AGENTS.md with a diff against disk, scrollable.
func (r Root) mapReportView() string {
	var b strings.Builder
	line := "MAP - " + r.mapPrev.Project.Name + "  "
	if r.mapPrev.MapStale {
		line += r.styleErr.Render("map: stale")
	} else {
		line += r.styleMuted.Render("map: ok")
	}
	line += "  "
	if r.mapPrev.AgentsMissing {
		line += r.styleErr.Render("agents: missing")
	} else {
		line += r.styleMuted.Render("agents: present")
	}
	b.WriteString(r.styleTitle.Render(line))

	lines := r.mapLines()
	rows := max(5, r.height-6)
	if r.height == 0 {
		rows = 20
	}
	start := min(max(r.mapScroll, 0), max(0, len(lines)-rows))
	for _, ln := range lines[start:min(start+rows, len(lines))] {
		if r.width > 0 {
			ln = shorten(ln, max(1, r.width-4))
		}
		b.WriteString("\n" + ln)
	}
	b.WriteString("\n\n" + r.styleMuted.Render(fmt.Sprintf(
		"line %d/%d - up/down scroll - a rebuild - esc back", start+1, len(lines))))
	return b.String()
}

// fileLines splits file content into lines without a trailing blank.
func fileLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// Fixed column widths for the Master Dashboard, built once (AGENTS 8).
var (
	masterLabel = lipgloss.NewStyle().Width(24)
	masterEvent = lipgloss.NewStyle().Width(12)
)

// masterView is the Master Dashboard (spec 3.3): portfolio numbers,
// language bars and recent activity (P4.16).
func (r Root) masterView() string {
	var b strings.Builder
	if r.masterRes == nil {
		b.WriteString(r.styleTitle.Render("MASTER DASHBOARD"))
		b.WriteString("\n\n" + r.styleMuted.Render("Loading…"))
		b.WriteString("\n" + r.styleMuted.Render("esc back"))
		return b.String()
	}
	d := *r.masterRes
	b.WriteString(r.styleTitle.Render("MASTER DASHBOARD"))
	b.WriteString("\n" + r.styleMuted.Render(fmt.Sprintf("Root: %s  Roots: %d  Last scan: %s  Health: %d/100",
		d.Root, d.Roots, ago(d.LastScan), d.Stats.AvgHealth)))

	b.WriteString("\n\nPortfolio")
	b.WriteString("\n  " + masterLabel.Render("Total projects") + strconv.Itoa(d.Stats.Total))
	for _, c := range d.Stats.ByFlow {
		label := c.Name
		if c.Name == "source" {
			label = "source (untriaged)"
		}
		b.WriteString("\n  " + masterLabel.Render(label) + strconv.Itoa(c.Count))
	}

	b.WriteString("\n\nNeeds attention")
	if len(d.Attention) == 0 {
		b.WriteString("\n  " + r.styleMuted.Render("nothing needs attention"))
	}
	for i, at := range d.Attention {
		mark := "  ! "
		if i == r.masterCursor {
			mark = r.styleErr.Render("> ! ")
		}
		b.WriteString("\n" + mark + attentionLabel(at, r.cfg.Flow.StaleThresholdDays))
	}

	b.WriteString("\n\nBy language")
	if len(d.Stats.ByLanguage) == 0 {
		b.WriteString("\n  " + r.styleMuted.Render("no projects yet"))
	}
	for _, c := range d.Stats.ByLanguage {
		b.WriteString("\n  " + masterLabel.Render(c.Name) + masterBar(c.Count, d.Stats.Total) + "  " + strconv.Itoa(c.Count))
	}

	b.WriteString("\n\nRecent activity")
	if len(d.Recent) == 0 {
		b.WriteString("\n  " + r.styleMuted.Render("no activity recorded yet"))
	}
	for _, ev := range d.Recent {
		b.WriteString("\n  " + masterEvent.Render(ago(ev.OccurredAt)) + ev.Event + "  " + ev.Name)
	}
	foot := "esc back · g/m master"
	if len(d.Attention) > 0 {
		foot = "up/down queue · enter fix · " + foot
	}
	b.WriteString("\n\n" + r.styleMuted.Render(foot))

	out := b.String()
	if r.width > 0 {
		ls := strings.Split(out, "\n")
		for i, ln := range ls {
			ls[i] = shorten(ln, max(1, r.width-2))
		}
		out = strings.Join(ls, "\n")
	}
	return out
}

// masterBar is the in-cell language bar (spec 3.3), capped at 8 cells
// so one outlier cannot stretch the row. █ maps to # in ascii mode.
func masterBar(n, total int) string {
	if total <= 0 || n <= 0 {
		return ""
	}
	return strings.Repeat("█", max(1, min(8, n*8/total)))
}

// statsView is the Stats screen (spec 3.6): totals, breakdowns and the
// 30-day activity sparkline (P4.19). Sections stack instead of the
// spec's three columns so every terminal width shows all of them.
func (r Root) statsView() string {
	var b strings.Builder
	if r.statsRes == nil {
		b.WriteString(r.styleTitle.Render("STATS"))
		b.WriteString("\n\n" + r.styleMuted.Render("Loading…"))
		b.WriteString("\n" + r.styleMuted.Render("esc back"))
		return b.String()
	}
	st := *r.statsRes
	rng := "All time"
	if st.Range != "all" {
		rng = st.Range
	}
	b.WriteString(r.styleTitle.Render("STATS"))
	b.WriteString("\n" + r.styleMuted.Render("Range: "+rng))

	b.WriteString("\n\n  " + masterLabel.Render("Projects total") + strconv.Itoa(st.Total))
	b.WriteString("\n  " + masterLabel.Render("Avg health") + strconv.Itoa(st.AvgHealth) + "/100")
	b.WriteString("\n  " + masterLabel.Render("Median health") + strconv.Itoa(st.MedianHealth) + "/100")
	b.WriteString("\n  " + masterLabel.Render(fmt.Sprintf("Stale (%dd+)", r.cfg.Flow.StaleThresholdDays)) + strconv.Itoa(st.Stale))
	b.WriteString("\n  " + masterLabel.Render("Missing folder") + strconv.Itoa(st.MissingFolder))
	b.WriteString("\n  " + masterLabel.Render("Missing map") + strconv.Itoa(st.MissingMap))
	b.WriteString("\n  " + masterLabel.Render("Missing README") + strconv.Itoa(st.MissingReadme))

	writeCounts := func(header string, cs []service.Count) {
		b.WriteString("\n\n" + header)
		if len(cs) == 0 {
			b.WriteString("\n  " + r.styleMuted.Render("none"))
			return
		}
		for _, c := range cs {
			b.WriteString("\n  " + masterLabel.Render(c.Name) + masterBar(c.Count, st.Total) + "  " + strconv.Itoa(c.Count))
		}
	}
	if !r.statsCompact {
		writeCounts("By language", st.ByLanguage)
		writeCounts("By flow", st.ByFlow)
	}
	writeCounts("By health", st.ByHealth)

	b.WriteString("\n\nActivity (last 30 days)")
	if sp := activitySpark(r.statsDaily); sp != "" {
		b.WriteString("\n  " + sp)
	} else {
		b.WriteString("\n  " + r.styleMuted.Render("no events yet"))
	}
	if r.statsExport {
		b.WriteString("\n\nExport: ")
		csv, js := "csv", "json"
		if r.statsExportC == 0 {
			csv = "[" + csv + "]"
		} else {
			js = "[" + js + "]"
		}
		b.WriteString(csv + " " + js)
		b.WriteString("\n" + r.styleMuted.Render("left/right choose · enter save to ./stats.* · esc cancel"))
	} else {
		b.WriteString("\n\n" + r.styleMuted.Render("t toggle · e export · esc back"))
	}

	out := b.String()
	if r.width > 0 {
		ls := strings.Split(out, "\n")
		for i, ln := range ls {
			ls[i] = shorten(ln, max(1, r.width-2))
		}
		out = strings.Join(ls, "\n")
	}
	return out
}

// attentionLabel words one triage bucket the way `rivu dashboard`
// does, so the screen and the CLI never disagree (P4.17).
func attentionLabel(at service.Attention, staleDays int) string {
	noun := "projects"
	if at.Count == 1 {
		noun = "project"
	}
	switch at.Key {
	case "mismatch_missing":
		return fmt.Sprintf("%d %s registered but missing from disk", at.Count, noun)
	case "unregistered":
		return fmt.Sprintf("%d %s on disk not yet registered", at.Count, noun)
	case "missing_git":
		return fmt.Sprintf("%d %s missing git", at.Count, noun)
	case "missing_bank":
		return fmt.Sprintf("%d %s missing Bank", at.Count, noun)
	case "missing_map":
		return fmt.Sprintf("%d %s missing Map", at.Count, noun)
	case "missing_readme":
		return fmt.Sprintf("%d %s missing README", at.Count, noun)
	case "stale":
		return fmt.Sprintf("%d stale %s (%dd+)", at.Count, noun, staleDays)
	}
	return fmt.Sprintf("%d %s %s", at.Count, noun, at.Key)
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
	if c.Require > 0 {
		hint = fmt.Sprintf("type %d to confirm   esc no (default)", c.Require)
		if c.typed != "" {
			hint += "   entered: " + c.typed
		}
	}
	if len(c.Lines) > confirmVisible {
		hint += "   ↑↓ scroll"
	}
	b.WriteString("\n" + r.styleMuted.Render(hint))
	return r.styleConfirm.Render(b.String())
}
