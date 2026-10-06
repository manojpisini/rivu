package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/manojpisini/rivu/internal/registry"
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
	Searching     bool
	Query         string
	Width         int
	Height        int
	WorkspaceRoot string
	Status        string
	Current       registry.Project
	HasCurrent    bool
	help          help.Model
}

func New(ps []registry.Project, workspaceRoot string) Model {
	m := Model{Projects: ps, WorkspaceRoot: workspaceRoot, FocusSidebar: true, help: help.New()}
	m.applyFilter()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.KeyMsg:
		if m.Searching {
			switch x.String() {
			case "esc":
				m.Searching = false
				m.Query = ""
				m.applyFilter()
			case "enter":
				m.Searching = false
			case "backspace":
				if len(m.Query) > 0 {
					_, n := utf8.DecodeLastRuneInString(m.Query)
					m.Query = m.Query[:len(m.Query)-n]
					m.applyFilter()
				}
			default:
				if len(x.Runes) > 0 {
					m.Query += string(x.Runes)
					m.applyFilter()
				}
			}
			return m, nil
		}

		switch {
		case key.Matches(x, keys.Quit):
			return m, tea.Quit
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
			m.Searching = true
			m.FocusSidebar = false
		case key.Matches(x, keys.Clear):
			m.Query = ""
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
			m.Status = "Run `rivu scan` to refresh the registry"
		case key.Matches(x, keys.Open):
			if p, ok := m.selectedProject(); ok {
				m.Status = fmt.Sprintf("Open with: rivu open %s", p.Slug)
			}
		case key.Matches(x, keys.Doctor):
			if p, ok := m.selectedProject(); ok {
				m.Status = fmt.Sprintf("Health check: rivu doctor %s", p.Slug)
			}
		case key.Matches(x, keys.Map):
			if p, ok := m.selectedProject(); ok {
				m.Status = fmt.Sprintf("Agent map: rivu agent sync %s", p.Slug)
			}
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
	query := strings.ToLower(strings.TrimSpace(m.Query))
	m.Visible = m.Visible[:0]
	for _, p := range m.Projects {
		if flow != allFlow && p.FlowStage != flow {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{p.Name, p.Slug, p.Path, p.Language, p.FlowStage}, " "))
		if query != "" && !strings.Contains(haystack, query) {
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

	header := m.header()
	footer := m.footer()
	bodyHeight := max(10, m.Height-lipgloss.Height(header)-lipgloss.Height(footer)-1)

	var row string
	switch {
	case m.Width < 100: // single pane: project list only
		row = m.projectPanel(m.Width-2, bodyHeight)
	case m.Width < 140: // two panes: list + detail
		rightWidth := clamp(m.Width/3, 30, 44)
		centerWidth := m.Width - rightWidth - 3
		row = lipgloss.JoinHorizontal(lipgloss.Top,
			m.projectPanel(centerWidth, bodyHeight), " ",
			m.detailsPanel(rightWidth, bodyHeight))
	default: // three panes: sidebar + list + detail
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
	if m.Searching {
		query = "  " + selectedStyle.Render("SEARCH: "+m.Query+"█")
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

func (m Model) detailsPanel(width, height int) string {
	p, ok := m.selectedProject()
	if !ok {
		return panelStyle.Width(width - 2).Height(height - 2).Render(titleStyle.Render("PROJECT DETAILS") + "\n\n" + mutedStyle.Render("Select a project to inspect it."))
	}
	status := func(v bool) string {
		if v {
			return "✓ ready"
		}
		return "○ missing"
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("PROJECT DETAILS") + "\n\n")
	b.WriteString(selectedStyle.Render(p.Name) + "\n")
	b.WriteString(mutedStyle.Render(p.Slug) + "\n\n")
	b.WriteString(labelValue("Flow", p.FlowStage) + "\n")
	b.WriteString(labelValue("Channel", p.Channel) + "\n")
	b.WriteString(labelValue("Language", fallback(p.Language, "Unknown")) + "\n")
	b.WriteString(labelValue("Health", fmt.Sprintf("%d/100", p.HealthScore)) + "\n\n")
	b.WriteString(titleStyle.Render("CAPABILITIES") + "\n")
	b.WriteString(labelValue("Git", status(p.HasGit)) + "\n")
	b.WriteString(labelValue("Bank", status(p.HasBank)) + "\n")
	b.WriteString(labelValue("Agent Map", status(p.HasMap)) + "\n\n")
	b.WriteString(titleStyle.Render("LOCATION") + "\n")
	b.WriteString(mutedStyle.Render(wrapPath(p.Path, width-4)) + "\n\n")
	b.WriteString(titleStyle.Render("COMMANDS") + "\n")
	b.WriteString(mutedStyle.Render("o/enter  open\nd        doctor\nm        rebuild map"))
	return panelStyle.Width(width - 2).Height(height - 2).Render(b.String())
}

func (m Model) footer() string {
	if m.Status != "" {
		return mutedStyle.Render(shorten(m.Status, max(1, m.Width)))
	}
	return mutedStyle.Render(shorten(m.help.ShortHelpView(m.footerHints()), max(1, m.Width)))
}

// footerHints picks the context key set; every binding comes from the
// single keyMap table so hints and handlers cannot drift.
func (m Model) footerHints() []key.Binding {
	if m.Searching {
		return []key.Binding{keys.SearchDone, keys.SearchCancel, keys.Quit}
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
