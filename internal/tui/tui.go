package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		case key.Matches(x, keys.SwitchPanel):
			m.FocusSidebar = !m.FocusSidebar
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
	}
	return m, nil
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

	header := m.header()
	footer := m.footer()
	bodyHeight := max(10, m.Height-lipgloss.Height(header)-lipgloss.Height(footer)-1)

	sidebarWidth := clamp(m.Width/5, 20, 28)
	rightWidth := clamp(m.Width/3, 34, 52)
	centerWidth := m.Width - sidebarWidth - rightWidth - 4
	if centerWidth < 36 {
		rightWidth = 0
		centerWidth = m.Width - sidebarWidth - 2
	}

	sidebar := m.sidebar(sidebarWidth, bodyHeight)
	projects := m.projectPanel(centerWidth, bodyHeight)
	row := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", projects)
	if rightWidth > 0 {
		details := m.detailsPanel(rightWidth, bodyHeight)
		row = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", projects, " ", details)
	}

	return bgStyle.Width(m.Width).Render(header + "\n" + row + "\n" + footer)
}

func (m Model) header() string {
	counts, avg := m.metrics()
	brand := titleStyle.Render("RIVU") + " " + subtitleStyle.Render("PROJECT EXPLORER")
	root := mutedStyle.Render(shorten(m.WorkspaceRoot, max(24, m.Width-42)))
	first := lipgloss.JoinHorizontal(lipgloss.Top, brand, strings.Repeat(" ", max(1, m.Width-lipgloss.Width(brand)-lipgloss.Width(root))), root)
	cards := fmt.Sprintf("%s %d   %s %d   %s %d   %s %d   %s %d   %s %d   %s %d/100",
		badgeStyle.Render("ALL"), len(m.Projects),
		badgeStyle.Render("SOURCE"), counts["source"],
		badgeStyle.Render("ACTIVE"), counts["active"],
		badgeStyle.Render("MAINT"), counts["maintenance"],
		badgeStyle.Render("RESEARCH"), counts["research"],
		badgeStyle.Render("DELTA"), counts["delta"],
		badgeStyle.Render("AVG HEALTH"), avg,
	)
	return first + "\n" + cards
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

func (m Model) projectPanel(width, height int) string {
	var b strings.Builder
	flow := strings.ToUpper(flowOrder[m.FlowCursor])
	query := ""
	if m.Searching {
		query = "  " + selectedStyle.Render("SEARCH: "+m.Query+"█")
	} else if m.Query != "" {
		query = "  " + mutedStyle.Render("filter: "+m.Query)
	}
	b.WriteString(titleStyle.Render(flow+" PROJECTS") + query + "\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("%-3s %-24s %-12s %6s", "", "NAME", "STACK", "HEALTH")) + "\n")
	b.WriteString(mutedStyle.Render(strings.Repeat("─", max(1, width-4))) + "\n")

	available := max(1, height-6)
	start := 0
	if m.Cursor >= available {
		start = m.Cursor - available + 1
	}
	end := min(len(m.Visible), start+available)
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
		nameWidth := max(10, width-27)
		line := fmt.Sprintf("%s%-*s %-12s %3d/100", marker, nameWidth, shorten(p.Name, nameWidth), shorten(lang, 12), p.HealthScore)
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
	return mutedStyle.Render(shorten(m.help.View(keys), max(1, m.Width)))
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
