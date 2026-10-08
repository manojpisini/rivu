package tui

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
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
	"github.com/manojpisini/rivu/internal/doctor"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/style"
)

const allFlow = "all"

// searchDebounce is how long typing must pause before the filter runs
// (P3.36); long enough to coalesce bursts, short enough to feel live.
const searchDebounce = 80 * time.Millisecond

// searchTickMsg applies the debounced filter; gen stale ticks are ignored.
type searchTickMsg struct{ gen int }

func searchDebounceCmd(gen int) tea.Cmd {
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg { return searchTickMsg{gen: gen} })
}

var flowOrder = []string{allFlow, "source", "active", "maintenance", "research", "delta"}

type Model struct {
	Projects       []registry.Project
	Visible        []registry.Project
	Cursor         int
	FlowCursor     int
	FocusSidebar   bool
	Query          string
	Picked         map[string]bool // multi-select set, keyed by project ID (P4.08)
	Untriaged      bool            // smart filter: source stage past SLA (P4.12)
	Width          int
	Height         int
	WorkspaceRoot  string
	RootExists     bool
	Status         string
	Current        registry.Project
	HasCurrent     bool
	DetailFull     bool
	FixKey         string // attention bucket to pre-highlight in detail (P4.18)
	search         textinput.Model
	help           help.Model
	svc            service.Service
	cfg            config.Config
	scanning       bool
	scanCancel     context.CancelFunc
	scanCount      *atomic.Int32
	scanDirs       int
	spin           spinner.Model
	flowPick       bool
	flowPickCursor int
	actionMenu     bool
	actionCursor   int
	helpOpen       bool
	searchGen      int                // debounces the filter while typing (P3.36)
	conflEdit      *membershipOverlay // Detail `c` membership overlay (P5.04)
}

func New(ps []registry.Project, workspaceRoot string) Model {
	m := Model{Projects: ps, WorkspaceRoot: workspaceRoot, RootExists: true, FocusSidebar: true, help: help.New()}
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
				m.applyFilter() // confirm shows the results immediately
			default:
				in, cmd := m.search.Update(msg)
				m.search = in
				m.Query = m.search.Value()
				// Filter after a ~80ms typing pause instead of on every
				// keystroke (P3.36): a stale tick is ignored by gen.
				m.searchGen++
				return m, tea.Batch(cmd, searchDebounceCmd(m.searchGen))
			}
			return m, nil
		}

		// The help overlay owns the keyboard until ?/esc.
		if m.helpOpen {
			switch x.String() {
			case "?", "esc":
				m.helpOpen = false
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}

		// The stage picker owns the keyboard until enter/esc.
		if m.flowPick {
			switch {
			case key.Matches(x, keys.Up):
				if m.flowPickCursor > 0 {
					m.flowPickCursor--
				}
			case key.Matches(x, keys.Down):
				if m.flowPickCursor < len(flowOrder)-2 {
					m.flowPickCursor++
				}
			case key.Matches(x, keys.SearchDone):
				return m.flowPickConfirm()
			case x.String() == "esc":
				m.flowPick = false
				m.Status = ""
			}
			return m, nil
		}

		// The actions menu owns the keyboard until enter/esc (spec 3.9, P4.11).
		if m.actionMenu {
			switch {
			case key.Matches(x, keys.Up):
				if m.actionCursor > 0 {
					m.actionCursor--
				}
			case key.Matches(x, keys.Down):
				if m.actionCursor < len(actionItems)-1 {
					m.actionCursor++
				}
			case key.Matches(x, keys.SearchDone):
				m.actionMenu = false
				return m.Update(actionKeyMsg(actionItems[m.actionCursor].key))
			case x.String() == "esc", x.String() == "q":
				m.actionMenu = false
				m.Status = ""
			}
			return m, nil
		}

		// The membership overlay owns the keyboard until enter/esc (P5.04).
		if m.conflEdit != nil {
			cmd := m.conflEdit.key(x)
			if m.conflEdit.closed {
				m.conflEdit = nil
			}
			return m, cmd
		}

		// Full-screen detail: esc/q/d go back, ctrl+c still quits.
		if m.DetailFull {
			switch x.String() {
			case "esc", "q", "d":
				m.DetailFull = false
				m.FixKey = ""
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
				m.FixKey = ""
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
		case key.Matches(x, keys.Pick):
			p, ok := m.selectedProject()
			if !ok {
				m.Status = "Nothing to pick"
				return m, nil
			}
			if m.Picked == nil {
				m.Picked = map[string]bool{}
			}
			if m.Picked[p.ID] {
				delete(m.Picked, p.ID)
			} else {
				m.Picked[p.ID] = true
			}
			if n := len(m.Picked); n > 0 {
				m.Status = fmt.Sprintf("%d picked — f to flow · space to un-pick", n)
			} else {
				m.Status = ""
			}
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
		case key.Matches(x, keys.Bottom):
			if len(m.Visible) > 0 {
				m.Cursor = len(m.Visible) - 1
			}
		case key.Matches(x, keys.PageDown):
			m.moveCursor(m.listRows())
		case key.Matches(x, keys.PageUp):
			m.moveCursor(-m.listRows())
		case key.Matches(x, keys.HalfDown):
			m.moveCursor(m.listRows() / 2)
		case key.Matches(x, keys.HalfUp):
			m.moveCursor(-m.listRows() / 2)
		case key.Matches(x, keys.Home):
			m.Cursor = 0
		case key.Matches(x, keys.End):
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
			cmd, err := openCmd(m.svc, p.Slug, "", m.cfg.Editors.GUI)
			if err != nil {
				return m, ShowToast(Toast{Level: "bad", Text: "could not open " + p.Slug + ": " + err.Error()})
			}
			return m, cmd
		case key.Matches(x, keys.Doctor):
			if m.svc == nil {
				m.Status = "Health checks unavailable: no service in this session"
				return m, nil
			}
			q := ""
			if p, ok := m.selectedProject(); ok {
				q = p.Slug // nothing selected (empty list) checks all
			}
			m.Status = "Running health checks…"
			return m, doctorCmd(m.svc, q)
		case key.Matches(x, keys.Master):
			if m.svc == nil {
				m.Status = "Master dashboard unavailable: no service in this session"
				return m, nil
			}
			m.Status = "Loading master dashboard…"
			return m, dashCmd(m.svc)
		case key.Matches(x, keys.Stats):
			if m.svc == nil {
				m.Status = "Stats unavailable: no service in this session"
				return m, nil
			}
			m.Status = "Loading stats…"
			return m, statsCmd(m.svc)
		case key.Matches(x, keys.Confluence):
			if m.svc == nil {
				m.Status = "Confluences unavailable: no service in this session"
				return m, nil
			}
			if m.DetailFull {
				// Spec 3.4: [c] on Detail edits this project's membership.
				p, ok := m.selectedProject()
				if !ok {
					m.Status = "Select a project to see its confluences"
					return m, nil
				}
				return m, conflEditForProjectCmd(m.svc, p.Slug)
			}
			m.Status = "Loading confluences…"
			return m, confluencesCmd(m.svc)
		case key.Matches(x, keys.Map):
			if m.svc == nil {
				m.Status = "Agent map unavailable: no service in this session"
				return m, nil
			}
			p, ok := m.selectedProject()
			if !ok {
				m.Status = "Select a project to open its map report"
				return m, nil
			}
			m.Status = "Loading map report for " + p.Name + "…"
			return m, mapPreviewCmd(m.svc, p.Slug)
		case key.Matches(x, keys.Flow):
			if m.svc == nil {
				m.Status = "Flow unavailable: no service in this session"
				return m, nil
			}
			if _, ok := m.selectedProject(); !ok {
				m.Status = "Select a project to flow"
				return m, nil
			}
			m.flowPick = true
			m.flowPickCursor = 0
			m.Status = ""
			return m, nil
		case key.Matches(x, keys.Delta):
			if m.svc == nil {
				m.Status = "Delta unavailable: no service in this session"
				return m, nil
			}
			p, ok := m.selectedProject()
			if !ok {
				m.Status = "Select a project to delta"
				return m, nil
			}
			if p.FlowStage == "delta" {
				m.Status = p.Name + " is already at Delta — nothing to do"
				return m, nil
			}
			m.Status = "Preparing delta for " + p.Name + "…"
			return m, deltaPlanCmd(m.svc, p.Slug)
		case key.Matches(x, keys.Actions):
			if _, ok := m.selectedProject(); !ok {
				m.Status = "Select a project to see its actions"
				return m, nil
			}
			m.actionMenu = true
			m.actionCursor = 0
			m.Status = ""
			return m, nil
		case key.Matches(x, keys.Untriaged):
			m.Untriaged = !m.Untriaged
			m.applyFilter()
			if m.Untriaged {
				sla := m.cfg.Flow.SourceSLADays
				if sla < 1 {
					sla = 14
				}
				m.Status = fmt.Sprintf("Untriaged on — source past %dd · %d shown", sla, len(m.Visible))
			} else {
				m.Status = ""
			}
			return m, nil
		case key.Matches(x, keys.Source):
			return m, SelectScreen(ScreenSource)
		case key.Matches(x, keys.Help):
			m.helpOpen = true
			return m, nil
		case key.Matches(x, keys.Copy):
			if p, ok := m.selectedProject(); ok {
				return m, copyPathCmd(p.Path)
			}
			m.Status = "Select a project to copy its path"
			return m, nil
		case key.Matches(x, keys.Reveal):
			if p, ok := m.selectedProject(); ok {
				return m, revealCmd(p.Path)
			}
			m.Status = "Select a project to reveal its folder"
			return m, nil
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
	case searchTickMsg:
		if x.gen == m.searchGen {
			m.applyFilter()
		}
	case scanDoneMsg:
		return m.applyScanDone(x)
	case editorDoneMsg:
		if x.err != nil {
			return m, ShowToast(Toast{Level: "bad", Text: "editor for " + x.slug + " failed: " + x.err.Error()})
		}
		// Refresh on return: the editor may have touched files or git.
		return m, listCmd(m.svc)
	case mapDoneMsg:
		if x.err != nil {
			return m, ShowToast(Toast{Level: "bad", Text: "agent map failed for " + x.q + ": " + x.err.Error()})
		}
		// The map flags changed - refresh the list and confirm.
		return m, tea.Batch(
			ShowToast(Toast{Level: "good", Text: "agent map built for " + x.q}),
			listCmd(m.svc),
		)
	case flowPlanMsg:
		m.Status = ""
		if x.err != nil {
			if x.delta {
				return m, ShowToast(Toast{Level: "bad", Text: "cannot delta " + x.slug + ": " + x.err.Error()})
			}
			return m, ShowToast(Toast{Level: "bad", Text: "cannot move " + x.slug + ": " + x.err.Error()})
		}
		if x.delta {
			lines := append([]string{"Move to the Delta stage — project files stay untouched"}, flowPlanLines(x.res.Plan)...)
			svc := m.svc
			return m, ConfirmPlan("Delta "+x.slug+"?", lines, func() tea.Cmd {
				return deltaApplyCmd(svc, x.slug)
			})
		}
		title := fmt.Sprintf("Flow %s: %s -> %s?", x.slug, x.res.Plan.FromStage, x.res.Plan.ToStage)
		svc := m.svc
		return m, ConfirmPlan(title, flowPlanLines(x.res.Plan), func() tea.Cmd {
			return flowApplyCmd(svc, x.slug, x.stage)
		})
	case flowApplyMsg:
		m.Status = ""
		if x.err != nil {
			if x.delta {
				return m, ShowToast(Toast{Level: "bad", Text: "delta failed for " + x.slug + ": " + x.err.Error()})
			}
			return m, ShowToast(Toast{Level: "bad", Text: "flow failed for " + x.slug + ": " + x.err.Error()})
		}
		note := x.res.Note
		if x.delta {
			name := x.res.Project.Name
			if name == "" {
				name = x.slug
			}
			note = "Deltaed " + name + " — project files untouched"
		} else if note == "" {
			note = fmt.Sprintf("%s moved to %s", x.slug, x.stage)
		}
		return m, tea.Batch(
			ShowToast(Toast{Level: "good", Text: note}),
			listCmd(m.svc),
		)
	case bulkFlowPlanMsg:
		m.Status = ""
		if x.err != nil {
			return m, ShowToast(Toast{Level: "bad", Text: "cannot move picked projects: " + x.err.Error()})
		}
		if len(x.res.Done) == 0 {
			return m, ShowToast(Toast{Level: "bad", Text: "nothing to move to " + x.stage})
		}
		lines := []string{fmt.Sprintf("Flow %d projects: -> %s", len(x.res.Done), x.stage)}
		for _, d := range x.res.Done {
			lines = append(lines, flowPlanLines(d.Plan)...)
		}
		for _, f := range x.res.Failed {
			lines = append(lines, "skipped "+f.Query+": "+f.Err.Error())
		}
		title := fmt.Sprintf("Flow %d projects to %s?", len(x.res.Done), x.stage)
		svc, slugs, stage := m.svc, x.slugs, x.stage
		// Confirm-by-typing for the riskiest bulk moves (> 5): type the
		// count (spec Part A, P4.09).
		if len(x.res.Done) > 5 {
			return m, ConfirmPlanTyped(title, lines, len(x.res.Done), func() tea.Cmd {
				return bulkFlowApplyCmd(svc, slugs, stage)
			})
		}
		return m, ConfirmPlan(title, lines, func() tea.Cmd {
			return bulkFlowApplyCmd(svc, slugs, stage)
		})
	case bulkFlowApplyMsg:
		m.Status = ""
		total := len(x.res.Done) + len(x.res.Failed)
		if x.err != nil {
			return m, ShowToast(Toast{Level: "bad", Text: "bulk flow failed: " + x.err.Error()})
		}
		// The picked set has served its purpose either way — partial
		// failures are reported, not silently kept for a retry.
		m.Picked = nil
		if len(x.res.Failed) > 0 {
			return m, tea.Batch(
				ShowToast(Toast{Level: "bad", Text: fmt.Sprintf("moved %d of %d to %s — %s", len(x.res.Done), total, x.stage, x.res.Failed[0].Err)}),
				listCmd(m.svc),
			)
		}
		return m, tea.Batch(
			ShowToast(Toast{Level: "good", Text: fmt.Sprintf("moved %d project(s) to %s", len(x.res.Done), x.stage)}),
			listCmd(m.svc),
		)
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

// doctorDoneMsg carries health-check reports to the Root, which owns the
// result screen.
type doctorDoneMsg struct {
	reports []doctor.Report
	q       string
	err     error
}

// doctorCmd runs health checks off the UI thread.
func doctorCmd(svc service.Service, q string) tea.Cmd {
	return func() tea.Msg {
		rs, err := svc.Doctor(q)
		return doctorDoneMsg{reports: rs, q: q, err: err}
	}
}

// mapDoneMsg is the outcome of building one agent map.
type mapDoneMsg struct {
	q   string
	err error
}

// mapCmd builds the agent map for q off the UI thread.
func mapCmd(svc service.Service, q string) tea.Cmd {
	return func() tea.Msg {
		return mapDoneMsg{q: q, err: svc.Map(q)}
	}
}

// mapPreviewMsg carries the Map report payload (P4.15).
type mapPreviewMsg struct {
	q    string
	prev service.MapPreview
	err  error
}

// mapPreviewCmd loads the Map report for q off the UI thread.
func mapPreviewCmd(svc service.Service, q string) tea.Cmd {
	return func() tea.Msg {
		prev, err := svc.MapPreview(q)
		return mapPreviewMsg{q: q, prev: prev, err: err}
	}
}

// dashMsg carries the Master Dashboard snapshot (P4.16).
type dashMsg struct {
	d   service.Dashboard
	err error
}

// dashCmd loads the Master Dashboard off the UI thread.
func dashCmd(svc service.Service) tea.Cmd {
	return func() tea.Msg {
		d, err := svc.Dashboard()
		return dashMsg{d: d, err: err}
	}
}

// statsMsg carries the Stats screen snapshot (P4.19).
type statsMsg struct {
	st    service.Stats
	daily []int
	err   error
}

// statsCmd loads metrics plus the 30-day activity series off-thread.
func statsCmd(svc service.Service) tea.Cmd {
	return func() tea.Msg {
		st, err := svc.Stats(0)
		if err != nil {
			return statsMsg{err: err}
		}
		daily, derr := svc.ActivityDaily(30)
		return statsMsg{st: st, daily: daily, err: derr}
	}
}

// exportDoneMsg reports a finished stats export (P4.20).
type exportDoneMsg struct {
	name string
	err  error
}

// exportCmd writes stats.csv or stats.json into the working directory
// with the same encoders `rivu stats --csv/--json` uses (P4.20).
func exportCmd(format string, st service.Stats) tea.Cmd {
	return func() tea.Msg {
		path := "stats." + format
		f, err := os.Create(path)
		if err != nil {
			return exportDoneMsg{err: err}
		}
		defer f.Close()
		if format == "json" {
			err = service.StatsJSON(f, st)
		} else {
			err = service.StatsCSV(f, st)
		}
		if err != nil {
			return exportDoneMsg{err: err}
		}
		return exportDoneMsg{name: path}
	}
}

// osc52 encodes s for the OSC 52 clipboard escape (P3.29); terminals
// that ignore it simply drop the sequence.
func osc52(s string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(s)) + "\x07"
}

// copyDoneMsg reports the OSC 52 write outcome.
type copyDoneMsg struct{ err error }

// copyPathCmd writes the OSC 52 sequence to stdout off the UI thread.
func copyPathCmd(path string) tea.Cmd {
	return func() tea.Msg {
		_, err := io.WriteString(os.Stdout, osc52(path))
		return copyDoneMsg{err: err}
	}
}

// revealDoneMsg reports the file-manager launch outcome.
type revealDoneMsg struct {
	path string
	err  error
}

// revealCmd opens the OS file manager at path (GUI, no TTY needed —
// the argv is chosen from GOOS, never through a shell).
func revealCmd(path string) tea.Cmd {
	return func() tea.Msg {
		var c *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			c = exec.Command("explorer", path)
		case "darwin":
			c = exec.Command("open", path)
		default:
			c = exec.Command("xdg-open", path)
		}
		if err := c.Start(); err != nil {
			return revealDoneMsg{path: path, err: err}
		}
		go func() { _ = c.Wait() }()
		return revealDoneMsg{path: path}
	}
}

// flowPlanMsg is a dry-run Flow result; it opens the Plan modal.
type flowPlanMsg struct {
	slug  string
	stage string
	delta bool // true when opened by the dedicated A delta action (P4.10)
	res   service.FlowResult
	err   error
}

// flowApplyMsg is the applied Flow result.
type flowApplyMsg struct {
	slug  string
	stage string
	delta bool
	res   service.FlowResult
	err   error
}

// flowPlanCmd asks the service for the dry-run Plan (no changes).
func flowPlanCmd(svc service.Service, slug, stage string) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.Flow(slug, stage, false, true)
		return flowPlanMsg{slug: slug, stage: stage, res: res, err: err}
	}
}

// flowApplyCmd performs the confirmed move (Plan -> Apply, spec 1.4.3).
func flowApplyCmd(svc service.Service, slug, stage string) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.Flow(slug, stage, false, false)
		return flowApplyMsg{slug: slug, stage: stage, res: res, err: err}
	}
}

// deltaPlanCmd is flowPlanCmd through the dedicated delta path: same
// service call, delta wording in the modal (spec 1.2.5, P4.10).
func deltaPlanCmd(svc service.Service, slug string) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.Flow(slug, "delta", false, true)
		return flowPlanMsg{slug: slug, stage: "delta", delta: true, res: res, err: err}
	}
}

// deltaApplyCmd is flowApplyCmd with the dedicated delta wording.
func deltaApplyCmd(svc service.Service, slug string) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.Flow(slug, "delta", false, false)
		return flowApplyMsg{slug: slug, stage: "delta", delta: true, res: res, err: err}
	}
}

// bulkFlowPlanMsg is the dry-run Plan for a multi-select flow (P4.08).
type bulkFlowPlanMsg struct {
	slugs []string
	stage string
	res   service.BulkFlowResult
	err   error
}

// bulkFlowApplyMsg is the applied bulk flow.
type bulkFlowApplyMsg struct {
	slugs []string
	stage string
	res   service.BulkFlowResult
	err   error
}

func bulkFlowPlanCmd(svc service.Service, slugs []string, stage string) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.FlowBulk(slugs, stage, false, true)
		return bulkFlowPlanMsg{slugs: slugs, stage: stage, res: res, err: err}
	}
}

func bulkFlowApplyCmd(svc service.Service, slugs []string, stage string) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.FlowBulk(slugs, stage, false, false)
		return bulkFlowApplyMsg{slugs: slugs, stage: stage, res: res, err: err}
	}
}

// flowPickConfirm closes the picker and either explains why no move can
// happen or asks the service for the dry-run Plan. With a multi-select
// set (P4.08) the picked projects win over the cursor row.
func (m Model) flowPickConfirm() (tea.Model, tea.Cmd) {
	stage := flowOrder[m.flowPickCursor+1]
	m.flowPick = false
	if len(m.Picked) > 0 {
		var qs []string
		for _, p := range m.Projects {
			if m.Picked[p.ID] && p.FlowStage != stage {
				qs = append(qs, p.Slug)
			}
		}
		if len(qs) == 0 {
			m.Status = "All picked projects are already in " + stage
			return m, nil
		}
		m.Status = fmt.Sprintf("Preparing %d moves to %s…", len(qs), stage)
		return m, bulkFlowPlanCmd(m.svc, qs, stage)
	}
	p, ok := m.selectedProject()
	if !ok {
		m.Status = "Select a project to flow"
		return m, nil
	}
	if p.FlowStage == stage {
		m.Status = p.Name + " is already in " + stage
		return m, nil
	}
	m.Status = "Preparing move to " + stage + "…"
	return m, flowPlanCmd(m.svc, p.Slug, stage)
}

// helpView renders the full keymap through bubbles/help (ShowAll), in
// the standard panel, centred; ? or esc closes it.
func (m Model) helpView() string {
	m.help.ShowAll = true
	if m.help.Width == 0 {
		m.help.Width = m.Width
	}
	box := panelStyle.Render(titleStyle.Render("KEYS") + "\n\n" + m.help.View(keys) +
		"\n\n" + mutedStyle.Render("? or esc to close"))
	if m.Width > 0 && m.Height > 0 {
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// flowPickerView centres the stage list: up/down choose, enter builds
// the Plan, esc cancels. Current stage is marked, not hidden.
func (m Model) flowPickerView() string {
	var b strings.Builder
	title := "FLOW — choose a stage"
	if p, ok := m.selectedProject(); ok {
		title = "FLOW — move " + p.Name
	}
	b.WriteString(titleStyle.Render(title))
	for i, s := range flowOrder[1:] {
		line := "  " + s
		if i == m.flowPickCursor {
			line = selectedStyle.Render("> " + s)
		}
		if p, ok := m.selectedProject(); ok && p.FlowStage == s {
			line += mutedStyle.Render("  (current)")
		}
		b.WriteString("\n" + line)
	}
	b.WriteString("\n\n" + mutedStyle.Render("↑↓ choose stage · enter plan · esc cancel"))
	box := panelStyle.Render(b.String())
	if m.Width > 0 && m.Height > 0 {
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// actionItems is the x actions menu (spec 3.9): every action that
// operates on the selection. Choosing a row dispatches back through
// the same key path the shortcut uses, so the menu cannot drift.
var actionItems = []struct{ key, label string }{
	{"enter", "open in preferred editor"},
	{"d", "detail"},
	{"h", "doctor"},
	{"a", "build agent map"},
	{"f", "flow to another stage"},
	{"A", "delta (archive)"},
	{"y", "copy path"},
	{"o", "reveal folder"},
	{"?", "help"},
}

// actionKeyMsg turns a menu key back into the KeyMsg the switch expects.
func actionKeyMsg(k string) tea.Msg {
	if k == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// actionMenuView centres the action list like the stage picker.
func (m Model) actionMenuView() string {
	var b strings.Builder
	title := "ACTIONS"
	if p, ok := m.selectedProject(); ok {
		title = "ACTIONS — " + p.Name
	}
	b.WriteString(titleStyle.Render(title))
	for i, a := range actionItems {
		line := "  " + a.key + "  " + a.label
		if i == m.actionCursor {
			line = selectedStyle.Render("> " + a.key + "  " + a.label)
		}
		b.WriteString("\n" + line)
	}
	b.WriteString("\n\n" + mutedStyle.Render("↑↓ choose · enter run · esc cancel"))
	box := panelStyle.Render(b.String())
	if m.Width > 0 && m.Height > 0 {
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// sparkline draws scores as ▁▂▃▄▅▆▇ blocks (P4.14). Fewer than two
// points cannot show a trend, so it renders nothing.
func sparkline(scores []int) string {
	if len(scores) < 2 {
		return ""
	}
	const blocks = "▁▂▃▄▅▆▇█"
	widths := []rune(blocks)
	var b strings.Builder
	for _, s := range scores {
		s = max(0, min(100, s))
		b.WriteString(string(widths[s*(len(widths)-1)/100]))
	}
	return b.String()
}

// activitySpark scales counts against their own maximum (unlike
// sparkline, which is fixed to 0-100 health scores) for the Stats
// screen (P4.19). No events at all renders nothing.
func activitySpark(counts []int) string {
	if len(counts) == 0 {
		return ""
	}
	top := 0
	for _, c := range counts {
		top = max(top, c)
	}
	if top == 0 {
		return ""
	}
	const levels = "▁▂▃▄▅▆▇█"
	widths := []rune(levels)
	var b strings.Builder
	for _, c := range counts {
		b.WriteString(string(widths[c*(len(widths)-1)/top]))
	}
	return b.String()
}

// listCmd re-queries the registry; Root applies projectsMsg.
func listCmd(svc service.Service) tea.Cmd {
	return func() tea.Msg {
		ps, err := svc.List(service.Filter{})
		return projectsMsg{ps: ps, err: err}
	}
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

// openTarget resolves the launch argv for slug (editor "" = the
// configured default) and reports whether the editor needs the
// terminal (tty) or can be Start()ed in background.
func openTarget(svc service.Service, slug, editor string, gui []string) (argv []string, tty bool, err error) {
	argv, err = svc.OpenCommand(slug, editor)
	if err != nil {
		return nil, false, err
	}
	return argv, !guiLaunch(argv, gui), nil
}

// openCmd resolves slug in editor and returns the command that launches
// it: terminal editors take the TTY via ExecProcess, GUI editors
// Start() in the background (spec 1.x / B-05).
func openCmd(svc service.Service, slug, editor string, gui []string) (tea.Cmd, error) {
	argv, tty, err := openTarget(svc, slug, editor, gui)
	if err != nil {
		return nil, err
	}
	if !tty {
		return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg {
			return editorDoneMsg{slug: slug, err: err}
		}), nil
	}
	return startOpenCmd(slug, argv), nil
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
	// -1 asks applyFilter not to carry the old selection over: a stage
	// switch browses the new stage from the top (P3.27).
	m.Cursor = -1
	m.applyFilter()
}

// stepStage walks the Flow sidebar by delta, wrapping at both ends.
func (m *Model) stepStage(delta int) {
	n := len(flowOrder)
	m.setStage(((m.FlowCursor+delta)%n + n) % n)
}

// applyFilter rebuilds Visible from the current stage and query. The
// selection follows the project by ID (P3.27): refresh or search that
// reorders, shrinks or drops rows keeps the same project selected when
// it survives, else falls back to the first row.
func (m *Model) applyFilter() {
	var keep string
	if m.Cursor >= 0 && m.Cursor < len(m.Visible) {
		keep = m.Visible[m.Cursor].ID
	}
	flow := flowOrder[m.FlowCursor]
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(m.Query)))
	sla := m.cfg.Flow.SourceSLADays
	if sla < 1 {
		sla = 14 // config default (config.go Flow defaults)
	}
	m.Visible = m.Visible[:0]
	for _, p := range m.Projects {
		if flow != allFlow && p.FlowStage != flow {
			continue
		}
		if m.Untriaged && !isUntriaged(p, sla, time.Now()) {
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
	m.Cursor = 0
	if keep != "" {
		for i := range m.Visible {
			if m.Visible[i].ID == keep {
				m.Cursor = i
				break
			}
		}
	}
}

// isUntriaged reports whether p is still in Source past the SLA
// (spec 3.9 smart filter u): source stage, created more than slaDays
// ago. A missing created_at is not enough evidence, so it counts as
// triaged.
func isUntriaged(p registry.Project, slaDays int, now time.Time) bool {
	if p.FlowStage != "source" || p.CreatedAt.IsZero() {
		return false
	}
	return now.Sub(p.CreatedAt) > time.Duration(slaDays)*24*time.Hour
}

// selectSlug moves the cursor to slug and opens its full-screen detail
// (the post-Source jump, P4.06). A slug the current filter hides is
// skipped — the toast still reports where the project landed.
func (m *Model) selectSlug(slug string) {
	for i, p := range m.Visible {
		if p.Slug == slug {
			m.Cursor = i
			m.DetailFull = true
			return
		}
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

// P3.31 ASCII fallback: when RIVU_ASCII=1 (or the locale says the
// terminal cannot show UTF-8) every rendered non-ASCII glyph is swapped
// for a width-1 plain equivalent at the single Root.View exit point —
// one replacer instead of threading a flag through every builder.
// Width-1 both sides keep lipgloss's layout maths honest.
func useASCIIGlyphs(getenv func(string) string) bool {
	if v := getenv("RIVU_ASCII"); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	locale := strings.ToLower(cmp.Or(getenv("LC_ALL"), getenv("LC_CTYPE"), getenv("LANG")))
	return locale != "" && !strings.Contains(locale, "utf")
}

var asciiGlyphs = useASCIIGlyphs(os.Getenv)

var asciiSwap = strings.NewReplacer(
	"—", "-", "·", ".", "…", ".", "›", ">", "▸", ">",
	"↑", "^", "↓", "v", "─", "-", "│", "|",
	"╭", "+", "╮", "+", "╰", "+", "╯", "+",
	"✓", "+", "×", "x", "•", ".",
	"█", "#", "░", ".",
	"▁", "_", "▂", "_", "▃", "_", "▄", "_", "▅", "_", "▆", "_", "▇", "_",
)

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
	if m.conflEdit != nil {
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, m.conflEdit.view(m.Width))
	}
	if m.helpOpen {
		return m.helpView()
	}
	if m.DetailFull {
		if _, ok := m.selectedProject(); ok {
			return m.detailFullView()
		}
	}
	if m.flowPick {
		return m.flowPickerView()
	}
	if m.actionMenu {
		return m.actionMenuView()
	}

	// The badge row wraps on narrow terminals, so budget the body from
	// the wrapped height — bubbletea cuts the TOP lines of a frame that
	// is taller than the terminal (standard_renderer.go), which would
	// hide the header entirely.
	header := bgStyle.Width(m.Width).Render(m.header())
	footer := m.footer()
	bodyHeight := max(10, m.Height-lipgloss.Height(header)-lipgloss.Height(footer)-1)

	var row string
	switch {
	case m.Width < 100: // single pane: project list only
		row = m.projectPanel(m.Width-2, bodyHeight)
	case m.Width < 120: // two panes: sidebar + list (spec 3.2)
		sidebarWidth := clamp(m.Width/5, 24, 30)
		row = lipgloss.JoinHorizontal(lipgloss.Top,
			m.sidebar(sidebarWidth, bodyHeight), " ",
			m.projectPanel(m.Width-sidebarWidth-3, bodyHeight))
	default: // side detail at >=120 (P3.12): sidebar + list + detail
		sidebarWidth := clamp(m.Width/5, 24, 30)
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
	b.WriteString(mutedStyle.Render("r  rescan\n/  search\nd  detail\nh  doctor\na  map\nf  flow\no  reveal\ny  copy path"))
	style := panelStyle
	if m.FocusSidebar {
		style = focusStyle
	}
	out := style.Width(width - 2).Height(max(0, height-2)).Render(b.String())
	// lipgloss only PADS to Height and never truncates, and a wrapped
	// line still outgrows the budget: cap the rendered pane to height,
	// keeping a proper bottom border (bubbletea cuts the frame's TOP
	// lines otherwise and the header vanishes).
	if height >= 2 {
		if lines := strings.Split(out, "\n"); len(lines) > height {
			empty := strings.Split(style.Width(width-2).Render(""), "\n")
			out = strings.Join(append(lines[:height-1], empty[len(empty)-1]), "\n")
		}
	}
	return out
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

// listRows mirrors projectPanel's virtualisation: how many table rows
// fit at the current window size (P3.30 page motions use the same
// number the renderer shows).
func (m Model) listRows() int {
	header := bgStyle.Width(m.Width).Render(m.header())
	body := max(10, m.Height-lipgloss.Height(header)-lipgloss.Height(m.footer())-1)
	return max(1, body-6)
}

// moveCursor steps the list by delta rows, clamped to the visible set;
// page and half-page motions drive the list regardless of sidebar
// focus (the sidebar only has six stages — a page is meaningless there).
func (m *Model) moveCursor(delta int) {
	if len(m.Visible) == 0 {
		m.Cursor = 0
		return
	}
	m.Cursor = clamp(m.Cursor+delta, 0, len(m.Visible)-1)
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
		mark := " "
		if m.Picked[p.ID] {
			mark = "✓"
		}
		marker := mark + " "
		if i == m.Cursor {
			marker = "›" + mark
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
		// Empty states teach the next action (P3.23); first run gets a
		// fuller onboarding card (P3.24).
		switch {
		case m.Query != "":
			b.WriteString("\n" + mutedStyle.Render("No projects match your search — press esc to clear it.") + "\n")
		case !m.RootExists:
			b.WriteString("\n" + m.onboardMissingRoot())
		case len(m.Projects) == 0:
			b.WriteString("\n" + m.onboardWelcome())
		default:
			b.WriteString("\n" + mutedStyle.Render("Nothing in this stage — press 1-5 to switch stage, or r to rescan.") + "\n")
		}
	}
	style := panelStyle
	if !m.FocusSidebar {
		style = focusStyle
	}
	return style.Width(width - 2).Height(height - 2).Render(b.String())
}

// onboardWelcome is the first-run card when the workspace simply has
// nothing registered yet (P3.24): three concrete next steps.
func (m Model) onboardWelcome() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("WELCOME") + "\n")
	b.WriteString(mutedStyle.Render("Rivu maps your project filesystem.") + "\n\n")
	b.WriteString(selectedStyle.Render("1") + "  rivu source <path>   add your first project\n")
	b.WriteString(selectedStyle.Render("2") + "  r                    rescan the workspace\n")
	b.WriteString(selectedStyle.Render("3") + "  / search  ·  ?       find things, see all keys\n")
	return b.String()
}

// onboardMissingRoot explains that the configured root does not exist;
// nothing can be discovered until it is created (P3.24).
func (m Model) onboardMissingRoot() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("WELCOME") + "\n")
	b.WriteString(mutedStyle.Render("Workspace root not found:") + "\n")
	b.WriteString(valueStyle.Render("  "+m.WorkspaceRoot) + "\n\n")
	b.WriteString(mutedStyle.Render("Create the folder (or set workspace.root), then press r.") + "\n")
	return b.String()
}

// detailsPanel is the side detail pane (>=120 cols).
func (m Model) detailsPanel(width, height int) string {
	p, ok := m.selectedProject()
	if !ok {
		return panelStyle.Width(width - 2).Height(height - 2).Render(titleStyle.Render("PROJECT DETAILS") + "\n\n" + mutedStyle.Render("Select a project (up/down to move, enter to open)."))
	}
	body := titleStyle.Render("PROJECT DETAILS") + "\n\n" + m.detailBody(p, width-4)
	return panelStyle.Width(width - 2).Height(height - 2).Render(body)
}

// detailFullView is the full-screen detail route (spec 3.4), used when
// the terminal is narrower than the 120-col side layout or on `d`.
func (m Model) detailFullView() string {
	p, _ := m.selectedProject()
	body := m.detailBody(p, min(m.Width-8, 76))
	actions := mutedStyle.Render("[enter] open  [h] doctor  [a] map report  [r] rescan  [d/esc] back")
	actions += "\n" + mutedStyle.Render("[c] confluences")
	box := panelStyle.Render(body + "\n" + actions)
	return bgStyle.Width(m.Width).Render(lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box))
}

// fixHint words the pre-highlighted fix for an attention bucket (P4.18);
// buckets without a TUI action say what to do by hand.
func fixHint(key string) string {
	switch key {
	case "missing_map", "missing_bank":
		return "press a to open the map report, then a again to rebuild Bank + Map"
	case "mismatch_missing", "unregistered", "stale":
		return "press r to rescan the workspace"
	case "missing_git":
		return "run git init in the project folder, then rescan (r)"
	case "missing_readme":
		return "add a README.md in the project folder"
	}
	return ""
}

// detailBody renders the spec 3.4 fields plus health checks with
// remedies; used by both the side pane and the full-screen view.
func (m Model) detailBody(p registry.Project, width int) string {
	var b strings.Builder
	b.WriteString(selectedStyle.Render(p.Name) + "\n")
	b.WriteString(mutedStyle.Render(p.Slug) + "\n")
	if m.FixKey != "" {
		b.WriteString("\n" + selectedStyle.Render("FIX") + "  " + valueStyle.Render(fixHint(m.FixKey)) + "\n")
	}
	b.WriteString("\n")
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
