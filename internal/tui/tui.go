package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/style"
)

const allFlow = "all"

var flowOrder = []string{allFlow, "source", "active", "maintenance", "research", "delta"}

type Model struct {
	Projects      []registry.Project
	Visible       []registry.Project
	Cursor        int
	FlowCursor    int
	FocusSidebar  bool
	Query         string
	Width         int
	Height        int
	WorkspaceRoot string
	Status        string
	Current       registry.Project
	HasCurrent    bool
	DetailFull    bool
	search        textinput.Model
	help          help.Model
	svc           service.Service
	cfg           config.Config
	scanning      bool
	scanCancel    context.CancelFunc
	scanCount     *atomic.Int32
	scanDirs      int
	spin          spinner.Model
}

func New(ps []registry.Project, workspaceRoot string) Model {
	m := Model{Projects: ps, WorkspaceRoot: workspaceRoot, FocusSidebar: true, help: help.New()}
	m.search = textinput.New()
	m.search.Prompt = "SEARCH: "
	m.search.Width = 32
	m.search.PromptStyle = selectedStyle
	m.search.TextStyle = valueStyle
	m.spin = spinner.New()
	m.spin.Style = mutedStyle
	m.applyFilter()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.KeyMsg:
		// A focused textinput owns every key except esc/enter (its own
		// confirm/cancel) and ctrl+c, which must always quit (spec 3.9).
		if m.search.Focused() {
			switch x.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.Query = ""
				m.search.SetValue("")
				m.search.Blur()
				m.applyFilter()
			case "enter":
				m.search.Blur()
			default:
				in, cmd := m.search.Update(msg)
				m.search = in
				m.Query = m.search.Value()
				m.applyFilter()
				return m, cmd
			}
			return m, nil
		}

		// Full-screen detail: esc/q/d go back, ctrl+c still quits.
		if m.DetailFull {
			switch x.String() {
			case "esc", "q", "d":
				m.DetailFull = false
				return m, nil
			}
		}

		// While a scan runs, esc cancels it (spec P3.15); the search
		// input above keeps esc for clearing its own query.
		if m.scanning && x.String() == "esc" {
			if m.scanCancel != nil {
				m.scanCancel()
			}
			m.Status = "Cancelling scan…"
			return m, nil
		}

		switch {
		case key.Matches(x, keys.Quit):
			return m, tea.Quit
		case key.Matches(x, keys.Detail):
			if _, ok := m.selectedProject(); ok {
				m.DetailFull = true
			} else {
				m.Status = "Select a project to view its details"
			}
		case key.Matches(x, keys.StageDigit):
			if s := x.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '5' {
				m.setStage(int(s[0] - '0'))
			}
		case key.Matches(x, keys.SwitchPanel):
			// While the sidebar has focus, left/right walk the stages;
			// anywhere else they switch panels.
			switch {
			case m.FocusSidebar && key.Matches(x, keys.StagePrev):
				m.stepStage(-1)
			case m.FocusSidebar && key.Matches(x, keys.StageNext):
				m.stepStage(1)
			default:
				m.FocusSidebar = !m.FocusSidebar
			}
		case key.Matches(x, keys.Search):
			m.FocusSidebar = false
			m.search.SetValue(m.Query)
			m.search.Focus()
		case key.Matches(x, keys.Clear):
			m.Query = ""
			m.search.SetValue("")
			m.applyFilter()
		case key.Matches(x, keys.Up):
			if m.FocusSidebar {
				if m.FlowCursor > 0 {
					m.FlowCursor--
					m.Cursor = 0
					m.applyFilter()
				}
			} else if m.Cursor > 0 {
				m.Cursor--
			}
		case key.Matches(x, keys.Down):
			if m.FocusSidebar {
				if m.FlowCursor < len(flowOrder)-1 {
					m.FlowCursor++
					m.Cursor = 0
					m.applyFilter()
				}
			} else if m.Cursor < len(m.Visible)-1 {
				m.Cursor++
			}
		case key.Matches(x, keys.Top):
			m.Cursor = 0
		case key.Matches(x, keys.Bottom):
			if len(m.Visible) > 0 {
				m.Cursor = len(m.Visible) - 1
			}
		case key.Matches(x, keys.Refresh):
			if m.scanning {
				m.Status = "Scan already running - esc cancels it"
				return m, nil
			}
			if m.svc == nil {
				m.Status = "Scan unavailable: no service in this session"
				return m, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			m.scanning = true
			m.scanCancel = cancel
			m.scanCount = &atomic.Int32{}
			m.scanDirs = 0
			m.Status = ""
			count := m.scanCount
			return m, tea.Batch(m.spin.Tick, scanCmd(m.svc, ctx, func(dirs int) {
				count.Store(int32(dirs))
			}))
		case key.Matches(x, keys.Open):
			p, ok := m.selectedProject()
			if !ok {
				m.Status = "Select a project to open"
				return m, nil
			}
			if m.svc == nil {
				m.Status = "Open unavailable: no service in this session"
				return m, nil
			}
			argv, tty, err := openTarget(m.svc, p.Slug, m.cfg.Editors.GUI)
			if err != nil {
				return m, ShowToast(Toast{Level: "bad", Text: "could not open " + p.Slug + ": " + err.Error()})
			}
			if !tty {
				// Terminal editor: suspend the TUI and hand over the TTY.
				return m, tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg {
					return editorDoneMsg{slug: p.Slug, err: err}
				})
			}
			// GUI editor: Start() in the background, TUI keeps running.
			return m, startOpenCmd(p.Slug, argv)
		case key.Matches(x, keys.Doctor):
			if p, ok := m.selectedProject(); ok {
				m.Status = fmt.Sprintf("Health check: rivu doctor %s", p.Slug)
			}
		case key.Matches(x, keys.Map):
			if p, ok := m.selectedProject(); ok {
				m.Status = fmt.Sprintf("Agent map: rivu agent sync %s", p.Slug)
			}
		}
	case spinner.TickMsg:
		if !m.scanning {
			return m, nil
		}
		s, next := m.spin.Update(msg)
		m.spin = s
		if m.scanCount != nil {
			m.scanDirs = int(m.scanCount.Load())
		}
		return m, next
	case scanDoneMsg:
		return m.applyScanDone(x)
	case editorDoneMsg:
		if x.err != nil {
			return m, ShowToast(Toast{Level: "bad", Text: "editor for " + x.slug + " failed: " + x.err.Error()})
		}
		// Refresh on return: the editor may have touched files or git.
		return m, func() tea.Msg {
			ps, err := m.svc.List(service.Filter{})
			return projectsMsg{ps: ps, err: err}
		}
	case tea.WindowSizeMsg:
		m.Width = x.Width
		m.Height = x.Height
		m.help.Width = x.Width
		if x.Width < 100 && m.FocusSidebar {
			m.FocusSidebar = false // sidebar pane is hidden below 100 cols
		}
	}
	return m, nil
}

// applyScanDone folds an async scan outcome into the model: reloads the
// list on success and always reports what happened in the status line.
func (m Model) applyScanDone(x scanDoneMsg) (Model, tea.Cmd) {
	m.scanning = false
	if m.scanCancel != nil {
		m.scanCancel()
		m.scanCancel = nil
	}
	switch {
	case x.cancelled:
		m.Status = fmt.Sprintf("Scan cancelled after %d dirs", m.scanDirs)
	case x.err != nil:
		m.Status = "Scan failed: " + x.err.Error()
	default:
		m.Projects = x.ps
		m.applyFilter()
		m.Status = fmt.Sprintf("Scan done: %d projects", len(x.ps))
		if n := len(x.warnings); n > 0 {
			m.Status += fmt.Sprintf(" (%d warnings)", n)
		}
	}
	return m, nil
}

// scanDoneMsg is the async scan outcome; cancelled distinguishes an
// esc-initiated stop from a failure.
type scanDoneMsg struct {
	ps        []registry.Project
	warnings  []string
	err       error
	cancelled bool
}

// scanCmd runs the service scan off the UI thread and reloads the list
// from the registry (truth, spec 4.4.5).
func scanCmd(svc service.Service, ctx context.Context, progress func(dirs int)) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.ScanContext(ctx, progress)
		if err != nil {
			return scanDoneMsg{err: err, cancelled: errors.Is(err, context.Canceled)}
		}
		ps, listErr := svc.List(service.Filter{})
		if listErr != nil {
			return scanDoneMsg{err: listErr, warnings: res.Warnings}
		}
		return scanDoneMsg{ps: ps, warnings: res.Warnings}
	}
}

// editorDoneMsg reports the outcome of an open: exec error, if any.
type editorDoneMsg struct {
	slug string
	err  error
}

// guiLaunch mirrors editorlaunch.IsGUI for an argv we already hold
// (re-joining and re-parsing would break paths with spaces): base name
// match against config's [editors].gui list, case-insensitive.
func guiLaunch(argv []string, gui []string) bool {
	if len(argv) == 0 {
		return false
	}
	base := argv[0]
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		base = strings.TrimSuffix(base, ext)
	}
	for _, g := range gui {
		if base == strings.ToLower(g) {
			return true
		}
	}
	return false
}

// openTarget resolves the launch argv for slug and reports whether the
// editor needs the terminal (tty) or can be Start()ed in background.
func openTarget(svc service.Service, slug string, gui []string) (argv []string, tty bool, err error) {
	argv, err = svc.OpenCommand(slug, "")
	if err != nil {
		return nil, false, err
	}
	return argv, !guiLaunch(argv, gui), nil
}

// startOpenCmd launches a GUI editor without blocking the TUI.
func startOpenCmd(slug string, argv []string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Command(argv[0], argv[1:]...).Start()
		return editorDoneMsg{slug: slug, err: err}
	}
}

// setStage jumps the Flow sidebar to flowOrder[i] and refilters.
func (m *Model) setStage(i int) {
	if i < 0 || i >= len(flowOrder) {
		return
	}
	m.FlowCursor = i
	m.Cursor = 0
	m.applyFilter()
}

// stepStage walks the Flow sidebar by delta, wrapping at both ends.
func (m *Model) stepStage(delta int) {
	n := len(flowOrder)
	m.setStage(((m.FlowCursor+delta)%n + n) % n)
}

func (m *Model) applyFilter() {
	flow := flowOrder[m.FlowCursor]
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(m.Query)))
	m.Visible = m.Visible[:0]
	for _, p := range m.Projects {
		if flow != allFlow && p.FlowStage != flow {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			p.Name, p.Slug, p.Path, p.Language, p.FlowStage, strings.Join(p.Stack, " "),
		}, " "))
		if !matchesQuery(p, tokens, haystack) {
			continue
		}
		m.Visible = append(m.Visible, p)
	}
	sort.SliceStable(m.Visible, func(i, j int) bool {
		if m.Visible[i].FlowStage == m.Visible[j].FlowStage {
			return strings.ToLower(m.Visible[i].Name) < strings.ToLower(m.Visible[j].Name)
		}
		return m.Visible[i].FlowStage < m.Visible[j].FlowStage
	})
	if len(m.Visible) == 0 {
		m.Cursor = 0
	} else if m.Cursor >= len(m.Visible) {
		m.Cursor = len(m.Visible) - 1
	}
}

// matchesQuery applies every whitespace token (AND): `field:value`
// restricts the named field (name/slug/path/stack/lang), any other
// token is a substring of the full haystack.
func matchesQuery(p registry.Project, tokens []string, haystack string) bool {
	for _, tok := range tokens {
		if field, value, ok := strings.Cut(tok, ":"); ok {
			if v, ok := searchField(p, field); ok {
				if !strings.Contains(strings.ToLower(v), value) {
					return false
				}
				continue
			}
		}
		if !strings.Contains(haystack, tok) {
			return false
		}
	}
	return true
}

// searchField resolves a `field:value` prefix; ok is false for
// unknown fields, which then fall back to plain-text matching.
func searchField(p registry.Project, field string) (string, bool) {
	switch field {
	case "name":
		return p.Name, true
	case "slug":
		return p.Slug, true
	case "path":
		return p.Path, true
	case "stack":
		return strings.Join(p.Stack, " "), true
	case "lang":
		return p.Language, true
	}
	return "", false
}

func (m Model) selectedProject() (registry.Project, bool) {
	if len(m.Visible) == 0 || m.Cursor < 0 || m.Cursor >= len(m.Visible) {
		return registry.Project{}, false
	}
	return m.Visible[m.Cursor], true
}

var theme = style.GraphiteViolet

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	subtitleStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Border).Foreground(theme.Text).Background(theme.Panel).Padding(0, style.Pad)
	focusStyle    = panelStyle.Copy().BorderForeground(theme.Accent)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	mutedStyle    = lipgloss.NewStyle().Foreground(theme.Muted)
	valueStyle    = lipgloss.NewStyle().Foreground(theme.Text)
	badgeStyle    = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	bgStyle       = lipgloss.NewStyle().Background(theme.Bg)
)

func (m Model) View() string {
	if m.Width == 0 {
		return "Loading Rivu…"
	}
	if m.Width < 60 || m.Height < 15 {
		return tooSmall(m.Width, m.Height)
	}
	if m.DetailFull {
		if _, ok := m.selectedProject(); ok {
			return m.detailFullView()
		}
	}

	header := m.header()
	footer := m.footer()
	bodyHeight := max(10, m.Height-lipgloss.Height(header)-lipgloss.Height(footer)-1)

	var row string
	switch {
	case m.Width < 100: // single pane: project list only
		row = m.projectPanel(m.Width-2, bodyHeight)
	case m.Width < 120: // two panes: sidebar + list (spec 3.2)
		sidebarWidth := clamp(m.Width/5, 20, 28)
		row = lipgloss.JoinHorizontal(lipgloss.Top,
			m.sidebar(sidebarWidth, bodyHeight), " ",
			m.projectPanel(m.Width-sidebarWidth-3, bodyHeight))
	default: // side detail at >=120 (P3.12): sidebar + list + detail
		sidebarWidth := clamp(m.Width/5, 20, 28)
		rightWidth := clamp(m.Width/3, 34, 52)
		centerWidth := m.Width - sidebarWidth - rightWidth - 4
		row = lipgloss.JoinHorizontal(lipgloss.Top,
			m.sidebar(sidebarWidth, bodyHeight), " ",
			m.projectPanel(centerWidth, bodyHeight), " ",
			m.detailsPanel(rightWidth, bodyHeight))
	}

	return bgStyle.Width(m.Width).Render(header + "\n" + row + "\n" + footer)
}

// tooSmall is the <60x15 rescue screen (AGENTS 8 layout rules).
func tooSmall(w, h int) string {
	msg := fmt.Sprintf("Terminal too small — need at least 60×15 (got %d×%d). Resize and Rivu adapts.", w, h)
	box := lipgloss.NewStyle().Foreground(theme.Warn).Render(msg)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) header() string {
	counts, avg := m.metrics()
	brand := titleStyle.Render("RIVU") + " " + subtitleStyle.Render("PROJECT EXPLORER")
	root := mutedStyle.Render(shorten(m.WorkspaceRoot, max(24, m.Width-42)))
	first := lipgloss.JoinHorizontal(lipgloss.Top, brand, strings.Repeat(" ", max(1, m.Width-lipgloss.Width(brand)-lipgloss.Width(root))), root)
	current := "—"
	if m.HasCurrent {
		current = shorten(m.Current.Name, 18)
	}
	cards := fmt.Sprintf("%s %d   %s %d   %s %d   %s %d   %s %d   %s %d   %s %d/100   %s %s   %s %s",
		badgeStyle.Render("ALL"), len(m.Projects),
		badgeStyle.Render("SOURCE"), counts["source"],
		badgeStyle.Render("ACTIVE"), counts["active"],
		badgeStyle.Render("MAINT"), counts["maintenance"],
		badgeStyle.Render("RESEARCH"), counts["research"],
		badgeStyle.Render("DELTA"), counts["delta"],
		badgeStyle.Render("AVG HEALTH"), avg,
		badgeStyle.Render("CURRENT"), current,
		badgeStyle.Render("LAST SCAN"), ago(m.lastScan()),
	)
	return first + "\n" + cards
}

// lastScan is the most recent scan time across loaded projects.
func (m Model) lastScan() time.Time {
	var last time.Time
	for _, p := range m.Projects {
		if p.LastScannedAt.After(last) {
			last = p.LastScannedAt
		}
	}
	return last
}

// ago renders a compact relative time for the header; a zero
// timestamp means the scan never happened (mirrors the CLI dashboard).
func ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func (m Model) sidebar(width, height int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("FLOW") + "\n\n")
	counts, _ := m.metrics()
	for i, flow := range flowOrder {
		label := strings.ToUpper(flow)
		count := len(m.Projects)
		if flow != allFlow {
			count = counts[flow]
		}
		line := fmt.Sprintf("%-13s %3d", label, count)
		if i == m.FlowCursor {
			line = selectedStyle.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + titleStyle.Render("WORKSPACE") + "\n")
	b.WriteString(mutedStyle.Render(shorten(m.WorkspaceRoot, width-4)) + "\n")
	b.WriteString("\n" + titleStyle.Render("QUICK ACTIONS") + "\n")
	b.WriteString(mutedStyle.Render("r  rescan hint\n/  search\no  open hint\nd  doctor hint\nm  map hint"))
	style := panelStyle
	if m.FocusSidebar {
		style = focusStyle
	}
	return style.Width(width - 2).Height(height - 2).Render(b.String())
}

// tableCols lists the optional project-table columns that fit at the
// current width; when space runs short the shrink order is path, then
// language, then channel (P3.09).
type tableCols struct{ path, lang, channel bool }

func columnsFor(inner int) tableCols {
	switch {
	case inner >= 70:
		return tableCols{path: true, lang: true, channel: true}
	case inner >= 55:
		return tableCols{lang: true, channel: true}
	case inner >= 44:
		return tableCols{channel: true}
	default:
		return tableCols{}
	}
}

// padCell truncates s to w display cells (tail side) and right-pads.
func padCell(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

// padStartCell keeps the tail of s (paths) and right-pads to w.
func padStartCell(s string, w int) string {
	if w <= 0 {
		return ""
	}
	for lipgloss.Width(s) > w-1 && s != "" {
		_, n := utf8.DecodeRuneInString(s)
		s = s[n:]
	}
	s = "…" + s
	return padCell(s, w)
}

func (m Model) projectPanel(width, height int) string {
	var b strings.Builder
	inner := width - 4

	// Virtualised window: only the rows around the cursor render.
	available := max(1, height-6)
	start := 0
	if m.Cursor >= available {
		start = m.Cursor - available + 1
	}
	end := min(len(m.Visible), start+available)

	flow := strings.ToUpper(flowOrder[m.FlowCursor])
	query := ""
	if m.search.Focused() {
		query = "  " + m.search.View()
	} else if m.Query != "" {
		query = "  " + mutedStyle.Render("filter: "+m.Query)
	}
	// Right side of the title: n/N position plus up/down indicators.
	right := ""
	if start > 0 {
		right += mutedStyle.Render(fmt.Sprintf("↑%d ", start))
	}
	pos := "0/0"
	if len(m.Visible) > 0 {
		pos = fmt.Sprintf("%d/%d", min(m.Cursor, len(m.Visible)-1)+1, len(m.Visible))
	}
	right += mutedStyle.Render(pos)
	if end < len(m.Visible) {
		right += mutedStyle.Render(fmt.Sprintf(" ↓%d", len(m.Visible)-end))
	}
	gap := max(1, inner-lipgloss.Width(flow+" PROJECTS"+query)-lipgloss.Width(right))
	b.WriteString(titleStyle.Render(flow+" PROJECTS") + query + strings.Repeat(" ", gap) + right + "\n")

	cols := columnsFor(inner)
	const (
		markerW = 2
		healthW = 8
		langW   = 12
		chanW   = 10
	)
	fixed := markerW + healthW
	if cols.lang {
		fixed += langW + 1
	}
	if cols.channel {
		fixed += chanW + 1
	}
	remainder := max(0, inner-fixed)
	nameW := max(14, remainder*2/3)
	pathW := 0
	if cols.path {
		pathW = remainder - nameW - 2
		if pathW < 10 {
			cols.path = false
			nameW = remainder
		}
	} else {
		nameW = remainder
	}

	header := padCell("", markerW) + padCell("NAME", nameW)
	if cols.path {
		header += " " + padCell("PATH", pathW)
	}
	if cols.lang {
		header += " " + padCell("LANGUAGE", langW)
	}
	if cols.channel {
		header += " " + padCell("CHANNEL", chanW)
	}
	header += strings.Repeat(" ", healthW-len("HEALTH")) + "HEALTH"
	b.WriteString(mutedStyle.Render(header) + "\n")
	b.WriteString(mutedStyle.Render(strings.Repeat("─", max(1, inner))) + "\n")

	for i := start; i < end; i++ {
		p := m.Visible[i]
		marker := "  "
		if i == m.Cursor {
			marker = "› "
		}
		lang := p.Language
		if lang == "" {
			lang = "Unknown"
		}
		line := marker + padCell(p.Name, nameW)
		if cols.path {
			line += " " + padStartCell(p.Path, pathW)
		}
		if cols.lang {
			line += " " + padCell(lang, langW)
		}
		if cols.channel {
			line += " " + padCell(p.Channel, chanW)
		}
		line += fmt.Sprintf(" %3d/100", p.HealthScore)
		if i == m.Cursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if len(m.Visible) == 0 {
		b.WriteString("\n" + mutedStyle.Render("No projects match this view.") + "\n")
	}
	style := panelStyle
	if !m.FocusSidebar {
		style = focusStyle
	}
	return style.Width(width - 2).Height(height - 2).Render(b.String())
}

// detailsPanel is the side detail pane (>=120 cols).
func (m Model) detailsPanel(width, height int) string {
	p, ok := m.selectedProject()
	if !ok {
		return panelStyle.Width(width - 2).Height(height - 2).Render(titleStyle.Render("PROJECT DETAILS") + "\n\n" + mutedStyle.Render("Select a project to inspect it."))
	}
	body := titleStyle.Render("PROJECT DETAILS") + "\n\n" + m.detailBody(p, width-4)
	return panelStyle.Width(width - 2).Height(height - 2).Render(body)
}

// detailFullView is the full-screen detail route (spec 3.4), used when
// the terminal is narrower than the 120-col side layout or on `d`.
func (m Model) detailFullView() string {
	p, _ := m.selectedProject()
	body := m.detailBody(p, min(m.Width-8, 76))
	actions := mutedStyle.Render("[enter] open  [h] doctor  [m] build map  [r] rescan  [d/esc] back")
	box := panelStyle.Render(body + "\n" + actions)
	return bgStyle.Width(m.Width).Render(lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box))
}

// detailBody renders the spec 3.4 fields plus health checks with
// remedies; used by both the side pane and the full-screen view.
func (m Model) detailBody(p registry.Project, width int) string {
	var b strings.Builder
	b.WriteString(selectedStyle.Render(p.Name) + "\n")
	b.WriteString(mutedStyle.Render(p.Slug) + "\n\n")
	b.WriteString(labelValue("Channel", fallback(p.Channel, "—")) + "\n")
	b.WriteString(labelValue("Flow", stageBadge(p.FlowStage)) + "\n")
	b.WriteString(labelValue("Stack", fallback(strings.Join(p.Stack, " › "), "—")) + "\n")
	b.WriteString(labelValue("Git", gitBadge(p.HasGit)) + "\n")
	b.WriteString(labelValue("Bank", bankBadge(p.HasBank)) + "\n")
	b.WriteString(labelValue("Map", mapBadge(p.HasMap)) + "\n")
	b.WriteString(labelValue("Health", healthBadge(p.HealthScore)) + "\n\n")
	b.WriteString(titleStyle.Render("HEALTH CHECKS") + "\n")
	for _, line := range detailChecks(p) {
		b.WriteString(shorten(line, width) + "\n")
	}
	return b.String()
}

// detailChecks lists ✓/! lines with remedies; only stored registry
// flags are used, so no I/O happens here (the real audit is
// `rivu doctor`, P3.19).
func detailChecks(p registry.Project) []string {
	var out []string
	check := func(ok bool, good, bad string) {
		if ok {
			out = append(out, badgeGood.Render("✓ "+good))
		} else {
			out = append(out, badgeWarn.Render("! "+bad))
		}
	}
	check(p.HasGit, "Git initialized", "Git missing — run `git init` in the project folder")
	check(p.HasBank, "Bank present", "Bank missing — run `rivu doctor "+p.Slug+"` to audit")
	check(p.HasMap, "Map built", "Map missing — run `rivu agent sync "+p.Slug+"`")
	check(p.HealthScore >= 70, fmt.Sprintf("Health %d/100", p.HealthScore),
		fmt.Sprintf("Health %d/100 is low — run `rivu doctor %s`", p.HealthScore, p.Slug))
	return out
}

func (m Model) footer() string {
	if m.scanning {
		line := m.spin.View() + fmt.Sprintf(" scanning… %d dirs", m.scanDirs)
		if m.Status != "" {
			line += " - " + m.Status
		}
		return mutedStyle.Render(shorten(line, max(1, m.Width)))
	}
	if m.Status != "" {
		return mutedStyle.Render(shorten(m.Status, max(1, m.Width)))
	}
	return mutedStyle.Render(shorten(m.help.ShortHelpView(m.footerHints()), max(1, m.Width)))
}

// footerHints picks the context key set; every binding comes from the
// single keyMap table so hints and handlers cannot drift. While a
// textinput is focused the global bindings do not apply, so they are
// not offered (P3.14).
func (m Model) footerHints() []key.Binding {
	if m.search.Focused() {
		return []key.Binding{keys.SearchDone, keys.SearchCancel}
	}
	return keys.ShortHelp()
}

func (m Model) metrics() (map[string]int, int) {
	counts := map[string]int{}
	totalHealth := 0
	for _, p := range m.Projects {
		counts[p.FlowStage]++
		totalHealth += p.HealthScore
	}
	avg := 0
	if len(m.Projects) > 0 {
		avg = totalHealth / len(m.Projects)
	}
	return counts, avg
}

func labelValue(label, value string) string {
	return mutedStyle.Render(fmt.Sprintf("%-10s", label)) + valueStyle.Render(value)
}

func shorten(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

func wrapPath(s string, width int) string {
	if width < 8 {
		return shorten(s, width)
	}
	var lines []string
	for len([]rune(s)) > width {
		r := []rune(s)
		cut := width
		for i := width; i > width/2; i-- {
			if r[i-1] == '\\' || r[i-1] == '/' {
				cut = i
				break
			}
		}
		lines = append(lines, string(r[:cut]))
		s = string(r[cut:])
	}
	if s != "" {
		lines = append(lines, s)
	}
	return strings.Join(lines, "\n")
}

func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func clamp(v, low, high int) int {
	return min(max(v, low), high)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
